package processors

import (
	"context"
	"crypto/sha256"
	"math/big"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-horizon/internal/db2/history"
)

type assetContractStatValue struct {
	contractID    xdr.ContractId
	activeBalance *big.Int
	activeHolders int32
}

func (v assetContractStatValue) ConvertToHistoryObject() history.ContractAssetStatRow {
	return history.ContractAssetStatRow{
		ContractID: v.contractID[:],
		Stat: history.ContractStat{
			ActiveBalance: v.activeBalance.String(),
			ActiveHolders: v.activeHolders,
		},
	}
}

type contractAssetBalancesQ interface {
	GetContractAssetBalances(ctx context.Context, keys []xdr.Hash) ([]history.ContractAssetBalance, error)
	DeleteContractAssetBalancesExpiringAt(ctx context.Context, ledger uint32) ([]history.ContractAssetBalance, error)
}

// ContractAssetStatSet represents a collection of asset stats for
// contract asset holders
type ContractAssetStatSet struct {
	createdAssetContracts    []xdr.Asset
	contractAssetStats       map[xdr.ContractId]assetContractStatValue
	createdBalances          []history.ContractAssetBalance
	removedBalances          []xdr.Hash
	updatedBalances          map[xdr.Hash]*big.Int
	removedExpirationEntries map[xdr.Hash]uint32
	createdExpirationEntries map[xdr.Hash]uint32
	updatedExpirationEntries map[xdr.Hash][2]uint32
	networkPassphrase        string
	assetStatsQ              contractAssetBalancesQ
	currentLedger            uint32
}

// NewContractAssetStatSet constructs a new ContractAssetStatSet instance
func NewContractAssetStatSet(
	assetStatsQ contractAssetBalancesQ,
	networkPassphrase string,
	removedExpirationEntries map[xdr.Hash]uint32,
	createdExpirationEntries map[xdr.Hash]uint32,
	updatedExpirationEntries map[xdr.Hash][2]uint32,
	currentLedger uint32,
) *ContractAssetStatSet {
	return &ContractAssetStatSet{
		createdAssetContracts:    []xdr.Asset{},
		contractAssetStats:       map[xdr.ContractId]assetContractStatValue{},
		networkPassphrase:        networkPassphrase,
		assetStatsQ:              assetStatsQ,
		removedExpirationEntries: removedExpirationEntries,
		createdExpirationEntries: createdExpirationEntries,
		updatedExpirationEntries: updatedExpirationEntries,
		currentLedger:            currentLedger,
		updatedBalances:          map[xdr.Hash]*big.Int{},
	}
}

// AddContractData updates the set to account for how a given contract data entry has changed.
// change must be a xdr.LedgerEntryTypeContractData type.
func (s *ContractAssetStatSet) AddContractData(change ingest.Change) error {
	// skip ingestion of contract asset balances if we find an asset contract metadata entry
	// because a ledger entry cannot be both an asset contract metadata entry and a
	// contract asset balance.
	if found, err := s.ingestAssetContractMetadata(change); err != nil {
		return err
	} else if found {
		return nil
	}
	return s.ingestContractAssetBalance(change)
}

func (s *ContractAssetStatSet) GetCreatedAssetContracts() ([]history.AssetContract, error) {
	var rows []history.AssetContract
	for _, asset := range s.createdAssetContracts {
		contractID, err := asset.ContractID(s.networkPassphrase)
		if err != nil {
			return nil, err
		}
		row := history.AssetContract{
			ContractID: contractID[:],
		}
		if err = asset.Extract(&row.AssetType, &row.AssetCode, &row.AssetIssuer); err != nil {
			return nil, errors.Wrap(err, "could not extract asset info from asset")
		}

		ledgerKey := sac.AssetToContractDataLedgerKey(contractID)
		bin, err := ledgerKey.MarshalBinary()
		if err != nil {
			return nil, errors.Wrap(err, "could not marshal key")
		}
		keyHash := sha256.Sum256(bin)
		row.KeyHash = keyHash[:]
		var ok bool
		row.ExpirationLedger, ok = s.createdExpirationEntries[keyHash]
		if !ok {
			return nil, errors.Errorf("could not find expiration ledger entry for asset contract %d", contractID)
		}
		rows = append(rows, row)
	}

	return rows, nil
}

func (s *ContractAssetStatSet) GetContractStats() []history.ContractAssetStatRow {
	var contractStats []history.ContractAssetStatRow
	for _, contractStat := range s.contractAssetStats {
		contractStats = append(contractStats, contractStat.ConvertToHistoryObject())
	}
	return contractStats
}

func (s *ContractAssetStatSet) GetCreatedBalances() []history.ContractAssetBalance {
	return s.createdBalances
}

func (s *ContractAssetStatSet) ingestAssetContractMetadata(change ingest.Change) (bool, error) {
	if change.Pre != nil || change.Post == nil {
		return false, nil
	}
	asset, found := sac.AssetFromContractData(*change.Post, s.networkPassphrase)
	if !found {
		return false, nil
	}
	keyHash, err := getKeyHash(*change.Post)
	if err != nil {
		return false, err
	}
	expirationLedger, ok := s.createdExpirationEntries[keyHash]
	if !ok || expirationLedger < s.currentLedger {
		return false, nil
	}
	if pContactID := change.Post.Data.MustContractData().Contract.ContractId; pContactID != nil {
		s.createdAssetContracts = append(s.createdAssetContracts, asset)
		return true, nil
	}
	return false, nil
}

func getKeyHash(ledgerEntry xdr.LedgerEntry) (xdr.Hash, error) {
	lk, err := ledgerEntry.LedgerKey()
	if err != nil {
		return xdr.Hash{}, errors.Wrap(err, "could not extract ledger key")
	}
	bin, err := lk.MarshalBinary()
	if err != nil {
		return xdr.Hash{}, errors.Wrap(err, "could not marshal key")
	}
	return sha256.Sum256(bin), nil
}

func (s *ContractAssetStatSet) ingestContractAssetBalance(change ingest.Change) error {
	switch {
	case change.Pre == nil && change.Post != nil: // created or restored
		pContractID := change.Post.Data.MustContractData().Contract.ContractId
		if pContractID == nil {
			return nil
		}

		_, postAmt, postOk := sac.ContractBalanceFromContractData(*change.Post, s.networkPassphrase)
		// we only ingest created ledger entries if we determine that they resemble the shape of
		// a Stellar Asset Contract balance ledger entry
		if !postOk {
			return nil
		}

		keyHash, err := getKeyHash(*change.Post)
		if err != nil {
			return err
		}
		expirationLedger, ok := s.createdExpirationEntries[keyHash]
		if !ok || expirationLedger < s.currentLedger {
			return nil
		}
		s.createdBalances = append(s.createdBalances, history.ContractAssetBalance{
			KeyHash:          keyHash[:],
			ContractID:       (*pContractID)[:],
			Amount:           postAmt.String(),
			ExpirationLedger: expirationLedger,
		})

		stat := s.getContractAssetStat(*pContractID)
		stat.activeHolders++
		stat.activeBalance.Add(stat.activeBalance, postAmt)
		s.maybeAddContractAssetStat(*pContractID, stat)
	case change.Pre != nil && change.Post == nil: // removed
		pContractID := change.Pre.Data.MustContractData().Contract.ContractId
		if pContractID == nil {
			return nil
		}

		keyHash, err := getKeyHash(*change.Pre)
		if err != nil {
			return err
		}
		// The key hash is recorded whether or not the entry still resembles a
		// balance, since rows are keyed on the ledger key. The stat delta is
		// applied by ingestRemovedBalances from the stored row, because the
		// value here is not a reliable record of what was counted.
		s.removedBalances = append(s.removedBalances, keyHash)
	case change.Pre != nil && change.Post != nil: // updated
		pContractID := change.Pre.Data.MustContractData().Contract.ContractId
		if pContractID == nil {
			return nil
		}

		holder, amt, ok := sac.ContractBalanceFromContractData(*change.Pre, s.networkPassphrase)
		if !ok {
			return nil
		}

		// if the updated ledger entry is not in the expected format then this
		// cannot be emitted by the stellar asset contract, so ignore it
		postHolder, postAmt, postOk := sac.ContractBalanceFromContractData(*change.Post, s.networkPassphrase)
		if !postOk || postHolder != holder {
			return nil
		}

		amtDelta := new(big.Int).Sub(postAmt, amt)
		if amtDelta.Cmp(big.NewInt(0)) == 0 {
			return nil
		}

		keyHash, err := getKeyHash(*change.Post)
		if err != nil {
			return err
		}

		s.updatedBalances[keyHash] = postAmt
		stat := s.getContractAssetStat(*pContractID)
		stat.activeBalance.Add(stat.activeBalance, amtDelta)
		s.maybeAddContractAssetStat(*pContractID, stat)
	default:
		return errors.Errorf("unexpected change Pre: %v Post: %v", change.Pre, change.Post)
	}
	return nil
}

// ingestRemovedBalances applies the stat delta for balances whose ledger entry
// was removed, taking the amount from the stored row rather than from the removed
// entry. A removed key with no stored row was either never ingested as a balance
// or was already accounted for when its row was reaped on expiry, and needs no
// adjustment either way.
//
// This must run before RemoveContractAssetBalances, which deletes the rows it
// reads.
func (s *ContractAssetStatSet) ingestRemovedBalances(ctx context.Context) error {
	if len(s.removedBalances) == 0 {
		return nil
	}

	rows, err := s.assetStatsQ.GetContractAssetBalances(ctx, s.removedBalances)
	if err != nil {
		return errors.Wrap(err, "Error fetching removed contract asset balances")
	}

	for _, row := range rows {
		amt, ok := new(big.Int).SetString(row.Amount, 10)
		if !ok {
			return errors.Errorf(
				"contract balance %v has invalid amount: %v",
				row.KeyHash,
				row.Amount,
			)
		}

		var contractID xdr.ContractId
		copy(contractID[:], row.ContractID)
		stat := s.getContractAssetStat(contractID)
		stat.activeHolders--
		stat.activeBalance.Sub(stat.activeBalance, amt)
		s.maybeAddContractAssetStat(contractID, stat)
	}

	return nil
}

func (s *ContractAssetStatSet) ingestExpiredBalances(ctx context.Context) error {
	rows, err := s.assetStatsQ.DeleteContractAssetBalancesExpiringAt(ctx, s.currentLedger-1)
	if err != nil {
		return errors.Wrap(err, "Error fetching contract asset balances")
	}

	for _, row := range rows {
		var keyHash xdr.Hash
		copy(keyHash[:], row.KeyHash)

		if _, ok := s.updatedExpirationEntries[keyHash]; ok {
			// the expiration of this contract balance was bumped, so we can
			// skip this contract balance since it is still active
			continue
		}

		var contractID xdr.ContractId
		copy(contractID[:], row.ContractID)
		stat := s.getContractAssetStat(contractID)
		amt, ok := new(big.Int).SetString(row.Amount, 10)
		if !ok {
			return errors.Errorf(
				"contract balance %v has invalid amount: %v",
				row.KeyHash,
				row.Amount,
			)
		}

		stat.activeHolders--
		stat.activeBalance.Sub(stat.activeBalance, amt)
		s.maybeAddContractAssetStat(contractID, stat)
	}

	return nil
}

func (s *ContractAssetStatSet) maybeAddContractAssetStat(contractID xdr.ContractId, stat assetContractStatValue) {
	if stat.activeHolders == 0 &&
		stat.activeBalance.Cmp(big.NewInt(0)) == 0 {
		delete(s.contractAssetStats, contractID)
	} else {
		s.contractAssetStats[contractID] = stat
	}
}

func (s *ContractAssetStatSet) getContractAssetStat(contractID xdr.ContractId) assetContractStatValue {
	stat, ok := s.contractAssetStats[contractID]
	if !ok {
		stat = assetContractStatValue{
			contractID:    contractID,
			activeBalance: big.NewInt(0),
			activeHolders: 0,
		}
	}
	return stat
}
