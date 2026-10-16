package processors

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-horizon/internal/db2/history"
)

// runContractDataLedger ingests changes for a single ledger the way live
// ingestion does, with one AssetStatsProcessor per ledger, and reports the rows
// the processor asked the db to upsert into contract_asset_balances, the key
// hashes it asked the db to remove, plus the contract asset stats it wrote.
// storedRows is what contract_asset_balances already holds for the keys the
// ledger changes.
func runContractDataLedger(
	t *testing.T, ledger uint32, storedRows []history.ContractAssetBalance, changes ...ingest.Change,
) (upserted []history.ContractAssetBalance, removed []xdr.Hash, statInserts []history.ContractAssetStatRow) {
	ctx := context.Background()
	q := &history.MockQAssetStats{}

	q.On("UpsertContractAssetBalances", ctx, mock.Anything).Run(func(args mock.Arguments) {
		upserted = append(upserted, args.Get(1).([]history.ContractAssetBalance)...)
	}).Return(nil).Maybe()
	q.On("RemoveContractAssetBalances", ctx, mock.Anything).Run(func(args mock.Arguments) {
		removed = append(removed, args.Get(1).([]xdr.Hash)...)
	}).Return(nil).Maybe()

	q.On("GetContractAssetBalances", ctx, mock.Anything).Return(storedRows, nil).Maybe()

	q.On("InsertAssetContracts", ctx, mock.Anything).Return(nil).Maybe()
	q.On("UpdateContractAssetBalanceExpirations", ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	q.On("UpdateAssetContractExpirations", ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	q.On("DeleteContractAssetBalancesExpiredBefore", ctx, ledger).
		Return([]history.ContractAssetBalance{}, nil).Maybe()
	q.On("DeleteAssetContractsExpiredBefore", ctx, ledger).Return(int64(0), nil).Maybe()
	q.On("GetContractAssetStat", ctx, mock.Anything).
		Return(history.ContractAssetStatRow{}, sql.ErrNoRows).Maybe()
	q.On("InsertContractAssetStat", ctx, mock.Anything).Run(func(args mock.Arguments) {
		statInserts = append(statInserts, args.Get(1).(history.ContractAssetStatRow))
	}).Return(int64(1), nil).Maybe()

	p := NewAssetStatsProcessor(q, "passphrase", false, ledger)
	for _, change := range changes {
		assert.NoError(t, p.ProcessChange(ctx, change))
	}
	assert.NoError(t, p.Commit(ctx))

	return upserted, removed, statInserts
}

func contractDataChange(pre, post *xdr.LedgerEntryData) ingest.Change {
	change := ingest.Change{Type: xdr.LedgerEntryTypeContractData}
	if pre != nil {
		change.Pre = &xdr.LedgerEntry{Data: *pre}
	}
	if post != nil {
		change.Post = &xdr.LedgerEntry{Data: *post}
	}
	return change
}

func ttlEntry(keyHash xdr.Hash, liveUntil uint32) *xdr.LedgerEntry {
	return &xdr.LedgerEntry{Data: xdr.LedgerEntryData{
		Type: xdr.LedgerEntryTypeTtl,
		Ttl: &xdr.TtlEntry{
			KeyHash:            keyHash,
			LiveUntilLedgerSeq: xdr.Uint32(liveUntil),
		},
	}}
}

func ttlCreate(keyHash xdr.Hash, liveUntil uint32) ingest.Change {
	return ingest.Change{Type: xdr.LedgerEntryTypeTtl, Post: ttlEntry(keyHash, liveUntil)}
}

func ttlUpdate(keyHash xdr.Hash, from, to uint32) ingest.Change {
	return ingest.Change{Type: xdr.LedgerEntryTypeTtl, Pre: ttlEntry(keyHash, from), Post: ttlEntry(keyHash, to)}
}

func ttlRemove(keyHash xdr.Hash, liveUntil uint32) ingest.Change {
	return ingest.Change{Type: xdr.LedgerEntryTypeTtl, Pre: ttlEntry(keyHash, liveUntil)}
}

// rewrittenBalance returns a balance entry whose value was rewritten, at the same
// ledger key, into something which is not a Stellar Asset Contract balance.
func rewrittenBalance(t *testing.T, balance xdr.LedgerEntryData) xdr.LedgerEntryData {
	rewritten := *balance.ContractData
	val := xdr.Uint32(1)
	rewritten.Val = xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &val}
	rewrittenData := xdr.LedgerEntryData{
		Type:         xdr.LedgerEntryTypeContractData,
		ContractData: &rewritten,
	}
	_, _, ok := sac.ContractBalanceFromContractData(xdr.LedgerEntry{Data: rewrittenData}, "passphrase")
	assert.False(t, ok, "rewritten entry must no longer resemble a balance")
	return rewrittenData
}

// storedBalance returns the row of a balance of 1000 which expires at ledger 5000.
func storedBalance(keyHash xdr.Hash, assetContractID [32]byte) history.ContractAssetBalance {
	return history.ContractAssetBalance{
		KeyHash:          keyHash[:],
		ContractID:       assetContractID[:],
		Amount:           "1000",
		ExpirationLedger: 5000,
	}
}

// TestContractDataRemovalFilteredByLedgerKey covers removal of a contract data
// entry whose value was rewritten, at the same ledger key, into something which
// no longer resembles a Stellar Asset Contract balance. contract_asset_balances
// is keyed on the ledger key, so a row's lifetime follows the key rather than
// the value, and removals are filtered on that basis.
func TestContractDataRemovalFilteredByLedgerKey(t *testing.T) {
	assetContractID := [32]byte{0xAA}
	holderID := [32]byte{0xBB}

	balance := sac.BalanceToContractData(assetContractID, holderID, 1000)
	keyHash := getKeyHashForBalance(t, assetContractID, holderID)
	rewrittenData := rewrittenBalance(t, balance)
	stored := storedBalance(keyHash, assetContractID)

	upserted, _, _ := runContractDataLedger(t, 100, nil,
		contractDataChange(nil, &balance),
		ttlCreate(keyHash, 5000),
	)
	assert.Equal(t, []history.ContractAssetBalance{stored}, upserted)

	_, removed, _ := runContractDataLedger(t, 101, []history.ContractAssetBalance{stored},
		contractDataChange(&rewrittenData, nil),
	)
	assert.Equal(t, []xdr.Hash{keyHash}, removed)
}

// TestContractDataUpdateFollowsValue covers updates which rewrite the value of a
// balance entry into something which is not a balance and back. The row follows
// the latest value of the entry: it is removed while the value is not a balance,
// and stored again when the value turns back into a balance in a ledger which also
// changes its ttl, since the expiration ledger was removed along with the row.
func TestContractDataUpdateFollowsValue(t *testing.T) {
	assetContractID := [32]byte{0xAA}
	holderID := [32]byte{0xBB}

	balance := sac.BalanceToContractData(assetContractID, holderID, 1000)
	keyHash := getKeyHashForBalance(t, assetContractID, holderID)
	rewrittenData := rewrittenBalance(t, balance)
	updatedBalance := sac.BalanceToContractData(assetContractID, holderID, 5000)

	upserted, removed, statInserts := runContractDataLedger(t, 101,
		[]history.ContractAssetBalance{storedBalance(keyHash, assetContractID)},
		contractDataChange(&balance, &rewrittenData),
	)
	assert.Empty(t, upserted)
	assert.Equal(t, []xdr.Hash{keyHash}, removed)
	assert.Equal(t, []history.ContractAssetStatRow{
		{
			ContractID: assetContractID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-1000",
				ActiveHolders: -1,
			},
		},
	}, statInserts)

	upserted, removed, statInserts = runContractDataLedger(t, 102, nil,
		contractDataChange(&rewrittenData, &updatedBalance),
		ttlUpdate(keyHash, 5000, 6000),
	)
	assert.Empty(t, removed)
	assert.Equal(t, []history.ContractAssetBalance{
		{
			KeyHash:          keyHash[:],
			ContractID:       assetContractID[:],
			Amount:           "5000",
			ExpirationLedger: 6000,
		},
	}, upserted)
	assert.Equal(t, []history.ContractAssetStatRow{
		{
			ContractID: assetContractID[:],
			Stat: history.ContractStat{
				ActiveBalance: "5000",
				ActiveHolders: 1,
			},
		},
	}, statInserts)
}

// TestContractDataRemovalOfUnrelatedKeyIgnored covers the other side of that
// filter: a removal whose ledger key could never have identified a row in
// contract_asset_balances is still skipped.
func TestContractDataRemovalOfUnrelatedKeyIgnored(t *testing.T) {
	contractID := xdr.ContractId{0xAA}
	key := xdr.ScSymbol("other")
	data := xdr.LedgerEntryData{
		Type: xdr.LedgerEntryTypeContractData,
		ContractData: &xdr.ContractDataEntry{
			Contract: xdr.ScAddress{
				Type:       xdr.ScAddressTypeScAddressTypeContract,
				ContractId: &contractID,
			},
			Key:        xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &key},
			Durability: xdr.ContractDataDurabilityPersistent,
			Val:        xdr.ScVal{Type: xdr.ScValTypeScvVoid},
		},
	}

	_, removed, _ := runContractDataLedger(t, 100, nil, contractDataChange(&data, nil))
	assert.Empty(t, removed)
}

// TestContractDataRemovalUsesStoredAmount covers the stat delta for a removed
// balance whose value was rewritten before it was removed. The amount comes from
// the stored row, since the value on the removed entry no longer describes what
// was counted.
func TestContractDataRemovalUsesStoredAmount(t *testing.T) {
	assetContractID := [32]byte{0xAA}
	holderID := [32]byte{0xBB}

	balance := sac.BalanceToContractData(assetContractID, holderID, 1000)
	keyHash := getKeyHashForBalance(t, assetContractID, holderID)
	rewrittenData := rewrittenBalance(t, balance)

	_, removed, statInserts := runContractDataLedger(t, 101,
		[]history.ContractAssetBalance{storedBalance(keyHash, assetContractID)},
		contractDataChange(&rewrittenData, nil),
		ttlRemove(keyHash, 5000),
	)

	assert.Equal(t, []xdr.Hash{keyHash}, removed)
	assert.Equal(t, []history.ContractAssetStatRow{
		{
			ContractID: assetContractID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-1000",
				ActiveHolders: -1,
			},
		},
	}, statInserts)
}

// TestContractDataRemovalWithoutStoredRowLeavesStats covers a removal whose entry
// does resemble a balance but which was never ingested as one. With no stored row
// there is nothing to remove and no stat to adjust, whatever the removed entry's
// value looks like.
func TestContractDataRemovalWithoutStoredRowLeavesStats(t *testing.T) {
	assetContractID := [32]byte{0xAA}
	holderID := [32]byte{0xBB}

	balance := sac.BalanceToContractData(assetContractID, holderID, 100)
	keyHash := getKeyHashForBalance(t, assetContractID, holderID)

	_, removed, statInserts := runContractDataLedger(t, 102, nil,
		contractDataChange(&balance, nil),
		ttlRemove(keyHash, 5000),
	)

	assert.Empty(t, removed)
	assert.Empty(t, statInserts)
}
