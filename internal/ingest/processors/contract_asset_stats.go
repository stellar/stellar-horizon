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
	DeleteContractAssetBalancesExpiredBefore(ctx context.Context, ledger uint32) ([]history.ContractAssetBalance, error)
}

// contractBalanceChange is the latest value of a ledger entry whose key has the form
// of a Stellar Asset Contract balance key. post is nil if the entry was removed.
type contractBalanceChange struct {
	contractID xdr.ContractId
	post       *xdr.LedgerEntry
}

// ContractAssetStatSet represents a collection of asset stats for
// contract asset holders
type ContractAssetStatSet struct {
	createdAssetContracts []xdr.Asset
	// contractAssetStats holds the change to the stats of each contract.
	contractAssetStats map[xdr.ContractId]assetContractStatValue
	// balanceChanges holds the changed contract balance entries by key hash, and
	// balanceKeys holds their key hashes in the order they were added.
	balanceChanges           map[xdr.Hash]contractBalanceChange
	balanceKeys              []xdr.Hash
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
		balanceChanges:           map[xdr.Hash]contractBalanceChange{},
		networkPassphrase:        networkPassphrase,
		assetStatsQ:              assetStatsQ,
		removedExpirationEntries: removedExpirationEntries,
		createdExpirationEntries: createdExpirationEntries,
		updatedExpirationEntries: updatedExpirationEntries,
		currentLedger:            currentLedger,
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
	return s.addContractBalanceChange(change)
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

// GetCreatedBalances returns the rows of the contract balances added to the set,
// assuming none of them is stored yet. This is the case when ingesting state from a
// history archive, where every change creates a ledger entry.
func (s *ContractAssetStatSet) GetCreatedBalances() []history.ContractAssetBalance {
	rows, _ := s.createdBalances()
	return rows
}

// GetContractStats returns the stats of the balances returned by GetCreatedBalances,
// grouped by contract.
func (s *ContractAssetStatSet) GetContractStats() []history.ContractAssetStatRow {
	_, stats := s.createdBalances()
	var contractStats []history.ContractAssetStatRow
	for _, contractStat := range stats {
		contractStats = append(contractStats, contractStat.ConvertToHistoryObject())
	}
	return contractStats
}

func (s *ContractAssetStatSet) createdBalances() ([]history.ContractAssetBalance, map[xdr.ContractId]assetContractStatValue) {
	var rows []history.ContractAssetBalance
	stats := map[xdr.ContractId]assetContractStatValue{}
	for _, keyHash := range s.balanceKeys {
		change := s.balanceChanges[keyHash]
		row, amount, ok := s.balanceRow(keyHash, change, nil)
		if !ok {
			continue
		}
		rows = append(rows, row)

		stat, ok := stats[change.contractID]
		if !ok {
			stat = assetContractStatValue{
				contractID:    change.contractID,
				activeBalance: big.NewInt(0),
			}
		}
		stat.activeHolders++
		stat.activeBalance.Add(stat.activeBalance, amount)
		stats[change.contractID] = stat
	}
	return rows, stats
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
	return getLedgerKeyHash(lk)
}

func getLedgerKeyHash(ledgerKey xdr.LedgerKey) (xdr.Hash, error) {
	bin, err := ledgerKey.MarshalBinary()
	if err != nil {
		return xdr.Hash{}, errors.Wrap(err, "could not marshal key")
	}
	return sha256.Sum256(bin), nil
}

// addContractBalanceChange records the latest value of a ledger entry whose key has
// the form of a Stellar Asset Contract balance key. Entries are selected by their key,
// which never changes, rather than by their value, which any contract can set to
// anything for its own entries. A value which is not a balance removes the row of the
// key, the same way a removed entry does.
func (s *ContractAssetStatSet) addContractBalanceChange(change ingest.Change) error {
	entry := change.Post
	if entry == nil {
		entry = change.Pre
	}
	if entry == nil {
		return errors.Errorf("unexpected change Pre: %v Post: %v", change.Pre, change.Post)
	}
	ledgerKey, err := entry.LedgerKey()
	if err != nil {
		return errors.Wrap(err, "could not extract ledger key")
	}
	if !sac.ValidContractBalanceLedgerKey(ledgerKey) {
		return nil
	}
	pContractID := entry.Data.MustContractData().Contract.ContractId
	if pContractID == nil {
		return nil
	}
	keyHash, err := getLedgerKeyHash(ledgerKey)
	if err != nil {
		return err
	}
	if _, ok := s.balanceChanges[keyHash]; !ok {
		s.balanceKeys = append(s.balanceKeys, keyHash)
	}
	s.balanceChanges[keyHash] = contractBalanceChange{contractID: *pContractID, post: change.Post}
	return nil
}

// balanceRow returns the row to store for the latest value of a contract balance entry
// along with its amount, or false if the entry should have no row: when it was removed,
// when its value is not a Stellar Asset Contract balance, or when it is not active in
// the current ledger. stored is the row in the db, if any, and is only used for the
// expiration ledger of entries whose ttl did not change in this ledger.
func (s *ContractAssetStatSet) balanceRow(
	keyHash xdr.Hash,
	change contractBalanceChange,
	stored *history.ContractAssetBalance,
) (history.ContractAssetBalance, *big.Int, bool) {
	if change.post == nil {
		return history.ContractAssetBalance{}, nil, false
	}
	_, amount, ok := sac.ContractBalanceFromContractData(*change.post, s.networkPassphrase)
	if !ok {
		return history.ContractAssetBalance{}, nil, false
	}
	expirationLedger, ok := s.expirationLedger(keyHash, stored)
	if !ok || expirationLedger < s.currentLedger {
		return history.ContractAssetBalance{}, nil, false
	}
	contractID := change.contractID
	return history.ContractAssetBalance{
		KeyHash:          keyHash[:],
		ContractID:       contractID[:],
		Amount:           amount.String(),
		ExpirationLedger: expirationLedger,
	}, amount, true
}

// expirationLedger returns the expiration ledger of a contract balance entry. A ttl
// change in this ledger takes precedence over the stored row.
func (s *ContractAssetStatSet) expirationLedger(keyHash xdr.Hash, stored *history.ContractAssetBalance) (uint32, bool) {
	if expirationLedger, ok := s.createdExpirationEntries[keyHash]; ok {
		return expirationLedger, true
	}
	if update, ok := s.updatedExpirationEntries[keyHash]; ok {
		return update[1], true
	}
	if stored != nil {
		return stored.ExpirationLedger, true
	}
	return 0, false
}

// ingestExpiredBalances deletes the rows of contract balances which are no longer
// active in the current ledger and removes them from the contract stats.
func (s *ContractAssetStatSet) ingestExpiredBalances(ctx context.Context) error {
	rows, err := s.assetStatsQ.DeleteContractAssetBalancesExpiredBefore(ctx, s.currentLedger)
	if err != nil {
		return errors.Wrap(err, "Error deleting expired contract asset balances")
	}

	for _, row := range rows {
		if err = s.removeFromStats(row); err != nil {
			return err
		}
	}
	return nil
}

// reconcileBalances compares the row which the latest value of every changed contract
// balance entry calls for with the row stored in the db. It returns the rows to upsert
// and the keys of the rows to remove, and adjusts the contract stats by the difference
// between the stored and the new rows.
//
// It must run after ingestExpiredBalances: an entry can be restored in the ledger right
// after it expired, and its expired row must be removed from the stats only once.
func (s *ContractAssetStatSet) reconcileBalances(ctx context.Context) ([]history.ContractAssetBalance, []xdr.Hash, error) {
	if len(s.balanceKeys) == 0 {
		return nil, nil, nil
	}

	rows, err := s.assetStatsQ.GetContractAssetBalances(ctx, s.balanceKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Error fetching contract asset balances")
	}
	stored := make(map[xdr.Hash]history.ContractAssetBalance, len(rows))
	for _, row := range rows {
		var keyHash xdr.Hash
		copy(keyHash[:], row.KeyHash)
		stored[keyHash] = row
	}

	var upserts []history.ContractAssetBalance
	var removals []xdr.Hash
	for _, keyHash := range s.balanceKeys {
		change := s.balanceChanges[keyHash]
		storedRow, hasStored := stored[keyHash]
		var storedPtr *history.ContractAssetBalance
		if hasStored {
			storedPtr = &storedRow
		}
		row, amount, hasRow := s.balanceRow(keyHash, change, storedPtr)

		if hasStored && hasRow &&
			row.Amount == storedRow.Amount &&
			row.ExpirationLedger == storedRow.ExpirationLedger {
			continue
		}
		if hasStored {
			if err = s.removeFromStats(storedRow); err != nil {
				return nil, nil, err
			}
			if !hasRow {
				removals = append(removals, keyHash)
			}
		}
		if hasRow {
			s.addToStats(change.contractID, amount)
			upserts = append(upserts, row)
		}
	}
	return upserts, removals, nil
}

func (s *ContractAssetStatSet) addToStats(contractID xdr.ContractId, amount *big.Int) {
	stat := s.getContractAssetStat(contractID)
	stat.activeHolders++
	stat.activeBalance.Add(stat.activeBalance, amount)
	s.maybeAddContractAssetStat(contractID, stat)
}

// removeFromStats removes a contract balance row stored in the db from the contract stats.
func (s *ContractAssetStatSet) removeFromStats(row history.ContractAssetBalance) error {
	amount, ok := new(big.Int).SetString(row.Amount, 10)
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
	stat.activeBalance.Sub(stat.activeBalance, amount)
	s.maybeAddContractAssetStat(contractID, stat)
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
