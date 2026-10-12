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

// contractStats holds the stats of contract balances, or the changes to them, by contract.
type contractStats map[xdr.ContractId]assetContractStatValue

// add adds holders and amount to the stats of a contract.
func (c contractStats) add(contractID xdr.ContractId, holders int32, amount *big.Int) {
	stat, ok := c[contractID]
	if !ok {
		stat = assetContractStatValue{contractID: contractID, activeBalance: big.NewInt(0)}
	}
	stat.activeHolders += holders
	stat.activeBalance.Add(stat.activeBalance, amount)
	if stat.activeHolders == 0 && stat.activeBalance.Sign() == 0 {
		delete(c, contractID)
	} else {
		c[contractID] = stat
	}
}

// removeRow removes a contract balance row stored in the db from the stats.
func (c contractStats) removeRow(row history.ContractAssetBalance) error {
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
	c.add(contractID, -1, amount.Neg(amount))
	return nil
}

func (c contractStats) rows() []history.ContractAssetStatRow {
	var rows []history.ContractAssetStatRow
	for _, stat := range c {
		rows = append(rows, stat.ConvertToHistoryObject())
	}
	return rows
}

type contractAssetBalancesQ interface {
	GetContractAssetBalances(ctx context.Context, keys []xdr.Hash) ([]history.ContractAssetBalance, error)
	DeleteContractAssetBalancesExpiredBefore(ctx context.Context, ledger uint32) ([]history.ContractAssetBalance, error)
}

// contractBalance is the latest value of a contract balance entry. amount is nil if the
// entry was removed or if its value is not a Stellar Asset Contract balance.
type contractBalance struct {
	contractID xdr.ContractId
	amount     *big.Int
}

// ContractAssetStatSet represents a collection of asset stats for
// contract asset holders
type ContractAssetStatSet struct {
	createdAssetContracts []xdr.Asset
	// contractAssetStats holds the change to the stats of each contract.
	contractAssetStats contractStats
	// balances holds the latest value of the changed contract balance entries by key
	// hash, and balanceKeys holds their key hashes in the order they were added.
	balances                 map[xdr.Hash]contractBalance
	balanceKeys              []xdr.Hash
	removedExpirationEntries map[xdr.Hash]uint32
	createdExpirationEntries map[xdr.Hash]uint32
	updatedExpirationEntries map[xdr.Hash][2]uint32
	networkPassphrase        string
	assetStatsQ              contractAssetBalancesQ
	currentLedger            uint32
}

// NewContractAssetStatSet constructs a new ContractAssetStatSet instance. The ttl
// changes of a ledger are only read when the rows of the set are built, so they can be
// added before or after its contract data changes.
func NewContractAssetStatSet(
	assetStatsQ contractAssetBalancesQ,
	networkPassphrase string,
	currentLedger uint32,
) *ContractAssetStatSet {
	return &ContractAssetStatSet{
		createdAssetContracts:    []xdr.Asset{},
		contractAssetStats:       contractStats{},
		balances:                 map[xdr.Hash]contractBalance{},
		removedExpirationEntries: map[xdr.Hash]uint32{},
		createdExpirationEntries: map[xdr.Hash]uint32{},
		updatedExpirationEntries: map[xdr.Hash][2]uint32{},
		networkPassphrase:        networkPassphrase,
		assetStatsQ:              assetStatsQ,
		currentLedger:            currentLedger,
	}
}

// AddContractData updates the set to account for how a given contract data entry has changed.
// change must be a xdr.LedgerEntryTypeContractData type.
func (s *ContractAssetStatSet) AddContractData(change ingest.Change) error {
	// a ledger entry cannot be both an asset contract instance and a contract balance
	if s.addAssetContract(change) {
		return nil
	}
	return s.addContractBalance(change)
}

// ContractAssetStatSet requires that the ttl changes it ingests are already compacted
// because the TTL change semantics in CAP-63 (see
// https://github.com/stellar/stellar-protocol/blob/master/core/cap-0063.md#ttl-ledger-change-semantics )
// are encapsulated in the ChangeCompactor
func (s *ContractAssetStatSet) checkTTLChangeIsCompacted(change ingest.Change) error {
	var keyHash xdr.Hash
	if change.Pre != nil {
		keyHash = change.Pre.Data.MustTtl().KeyHash
	} else {
		keyHash = change.Post.Data.MustTtl().KeyHash
	}
	if _, ok := s.removedExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	if _, ok := s.createdExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	if _, ok := s.updatedExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	return nil
}

// AddTTL updates the set to account for how a given ttl entry has changed.
// change must be a xdr.LedgerEntryTypeTtl type.
func (s *ContractAssetStatSet) AddTTL(change ingest.Change) error {
	if err := s.checkTTLChangeIsCompacted(change); err != nil {
		return err
	}

	switch {
	case change.Pre == nil && change.Post != nil: // created
		post := change.Post.Data.MustTtl()
		s.createdExpirationEntries[post.KeyHash] = uint32(post.LiveUntilLedgerSeq)
	case change.Pre != nil && change.Post == nil: // removed
		pre := change.Pre.Data.MustTtl()
		s.removedExpirationEntries[pre.KeyHash] = uint32(pre.LiveUntilLedgerSeq)
	case change.Pre != nil && change.Post != nil: // updated
		pre := change.Pre.Data.MustTtl()
		post := change.Post.Data.MustTtl()
		// it's unclear if core could emit a ledger entry change where the
		// expiration ledger remains the same
		if pre.LiveUntilLedgerSeq == post.LiveUntilLedgerSeq {
			return nil
		}

		// if a TTL was created, restored, or updated at ledger N
		// the LiveUntilLedgerSeq must be >= current ledger
		if uint32(post.LiveUntilLedgerSeq) < s.currentLedger {
			return errors.Errorf(
				"post TTL %v is less than current ledger %v",
				post.LiveUntilLedgerSeq,
				s.currentLedger,
			)
		}

		// The previous expiration ledger must always be greater than or equal to the current ledger
		// because if the previous expiration ledger is less than the current ledger then it implies
		// the ledger entry was archived. However, an archived ledger entry cannot be updated without
		// first being restored.
		// Alternatively, the TTL can be updated without being restored if it is a temporary ledger
		// entry. However, in that case, we can ignore the ledger entry entirely because SAC
		// ledger entries are always kept in persistent storage.
		if uint32(pre.LiveUntilLedgerSeq) < s.currentLedger {
			return nil
		}
		s.updatedExpirationEntries[pre.KeyHash] = [2]uint32{
			uint32(pre.LiveUntilLedgerSeq),
			uint32(post.LiveUntilLedgerSeq),
		}
	default:
		return errors.Errorf("unexpected change Pre: %v Post: %v", change.Pre, change.Post)
	}

	return nil
}

// addAssetContract records the asset of a created asset contract instance, and reports
// whether the change created one. Asset contract instances are recognized by their
// value, which names the asset that their contract id is derived from.
func (s *ContractAssetStatSet) addAssetContract(change ingest.Change) bool {
	if change.Pre != nil || change.Post == nil {
		return false
	}
	asset, found := sac.AssetFromContractData(*change.Post, s.networkPassphrase)
	if found {
		s.createdAssetContracts = append(s.createdAssetContracts, asset)
	}
	return found
}

// GetCreatedAssetContracts returns the rows of the created asset contract instances
// which are active in the current ledger.
func (s *ContractAssetStatSet) GetCreatedAssetContracts() ([]history.AssetContract, error) {
	var rows []history.AssetContract
	for _, asset := range s.createdAssetContracts {
		contractID, err := asset.ContractID(s.networkPassphrase)
		if err != nil {
			return nil, err
		}
		keyHash, err := getLedgerKeyHash(sac.AssetToContractDataLedgerKey(contractID))
		if err != nil {
			return nil, err
		}
		expirationLedger, ok := s.createdExpirationEntries[keyHash]
		if !ok || expirationLedger < s.currentLedger {
			continue
		}
		row := history.AssetContract{
			KeyHash:          keyHash[:],
			ContractID:       contractID[:],
			ExpirationLedger: expirationLedger,
		}
		if err = asset.Extract(&row.AssetType, &row.AssetCode, &row.AssetIssuer); err != nil {
			return nil, errors.Wrap(err, "could not extract asset info from asset")
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// GetCreatedBalancesAndStats returns the rows of the contract balances added to the set,
// assuming none of them is stored yet, along with their stats grouped by contract. This
// is the case when ingesting state from a history archive, where every change creates a
// ledger entry.
func (s *ContractAssetStatSet) GetCreatedBalancesAndStats() ([]history.ContractAssetBalance, []history.ContractAssetStatRow) {
	var rows []history.ContractAssetBalance
	stats := contractStats{}
	for _, keyHash := range s.balanceKeys {
		balance := s.balances[keyHash]
		if row, ok := s.balanceRow(keyHash, balance, nil); ok {
			rows = append(rows, row)
			stats.add(balance.contractID, 1, balance.amount)
		}
	}
	return rows, stats.rows()
}

func getLedgerKeyHash(ledgerKey xdr.LedgerKey) (xdr.Hash, error) {
	bin, err := ledgerKey.MarshalBinary()
	if err != nil {
		return xdr.Hash{}, errors.Wrap(err, "could not marshal key")
	}
	return sha256.Sum256(bin), nil
}

// addContractBalance records the latest value of a ledger entry whose key has the form
// of a Stellar Asset Contract balance key. Entries are selected by their key, which
// never changes, rather than by their value, which any contract can set to anything for
// its own entries. A value which is not a balance removes the row of the key, the same
// way a removed entry does.
func (s *ContractAssetStatSet) addContractBalance(change ingest.Change) error {
	if change.Pre == nil && change.Post == nil {
		return errors.Errorf("unexpected change Pre: %v Post: %v", change.Pre, change.Post)
	}
	ledgerKey, err := change.LedgerKey()
	if err != nil {
		return errors.Wrap(err, "could not extract ledger key")
	}
	if !sac.ValidContractBalanceLedgerKey(ledgerKey) {
		return nil
	}
	contractID := ledgerKey.ContractData.Contract.ContractId
	if contractID == nil {
		return nil
	}
	keyHash, err := getLedgerKeyHash(ledgerKey)
	if err != nil {
		return err
	}
	balance := contractBalance{contractID: *contractID}
	if change.Post != nil {
		if _, amount, valid := sac.ContractBalanceFromContractData(*change.Post, s.networkPassphrase); valid {
			balance.amount = amount
		}
	}
	if _, seen := s.balances[keyHash]; !seen {
		s.balanceKeys = append(s.balanceKeys, keyHash)
	}
	s.balances[keyHash] = balance
	return nil
}

// balanceRow returns the row to store for the latest value of a contract balance entry,
// or false if the entry should have no row: when it was removed, when its value is not
// a Stellar Asset Contract balance, or when it is not active in the current ledger.
// stored is the row in the db, if any.
func (s *ContractAssetStatSet) balanceRow(
	keyHash xdr.Hash,
	balance contractBalance,
	stored *history.ContractAssetBalance,
) (history.ContractAssetBalance, bool) {
	if balance.amount == nil {
		return history.ContractAssetBalance{}, false
	}
	expirationLedger, ok := s.expirationLedger(keyHash, stored)
	if !ok || expirationLedger < s.currentLedger {
		return history.ContractAssetBalance{}, false
	}
	return history.ContractAssetBalance{
		KeyHash:          keyHash[:],
		ContractID:       balance.contractID[:],
		Amount:           balance.amount.String(),
		ExpirationLedger: expirationLedger,
	}, true
}

// expirationLedger returns the expiration ledger of a contract balance entry. A ttl
// change in this ledger takes precedence over the stored row.
//
// Neither is known when a value which is not a balance removed the row of an entry, and
// a later ledger turns the value back into a balance without changing its ttl. Stellar
// Asset Contracts only write balances, so from protocol 23, where restoring an entry
// also emits the entry and its ttl, this only happens to the entries of other
// contracts, whose balances are not served. Such an entry is left without a row.
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
		if err = s.contractAssetStats.removeRow(row); err != nil {
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
	stored := make(map[xdr.Hash]*history.ContractAssetBalance, len(rows))
	for i := range rows {
		var keyHash xdr.Hash
		copy(keyHash[:], rows[i].KeyHash)
		stored[keyHash] = &rows[i]
	}

	var upserts []history.ContractAssetBalance
	var removals []xdr.Hash
	for _, keyHash := range s.balanceKeys {
		balance := s.balances[keyHash]
		storedRow := stored[keyHash]
		row, hasRow := s.balanceRow(keyHash, balance, storedRow)
		if storedRow != nil {
			if hasRow && row.Amount == storedRow.Amount && row.ExpirationLedger == storedRow.ExpirationLedger {
				continue
			}
			if err = s.contractAssetStats.removeRow(*storedRow); err != nil {
				return nil, nil, err
			}
			if !hasRow {
				removals = append(removals, keyHash)
			}
		}
		if hasRow {
			s.contractAssetStats.add(balance.contractID, 1, balance.amount)
			upserts = append(upserts, row)
		}
	}
	return upserts, removals, nil
}
