package processors

import (
	"context"
	"crypto/sha256"
	"math/big"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/ingest/sac"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
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
		// We always include the key hash in s.removedBalances even
		// if the ledger entry is not a valid balance ledger entry.
		// It's possible that a contract is able to forge a created
		// balance ledger entry which matches the Stellar Asset Contract
		// and later on the ledger entry is updated to an invalid state.
		// In such a scenario we still want to remove the balance ledger
		// entry from our db when the entry is removed from the ledger.
		s.removedBalances = append(s.removedBalances, keyHash)

		_, preAmt, ok := sac.ContractBalanceFromContractData(*change.Pre, s.networkPassphrase)
		if !ok {
			return nil
		}

		expirationLedger, ok := s.removedExpirationEntries[keyHash]
		if !ok || expirationLedger < s.currentLedger {
			return nil
		}

		stat := s.getContractAssetStat(*pContractID)
		stat.activeHolders--
		stat.activeBalance = new(big.Int).Sub(stat.activeBalance, preAmt)
		s.maybeAddContractAssetStat(*pContractID, stat)
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
