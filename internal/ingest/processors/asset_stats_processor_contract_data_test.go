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
// ingestion does, with one AssetStatsProcessor per ledger, and reports the key
// hashes the processor asked the db to insert into and remove from
// contract_asset_balances, plus the contract asset stats it wrote. storedRows is
// what contract_asset_balances already holds for the keys the ledger removes.
func runContractDataLedger(
	t *testing.T, ledger uint32, storedRows []history.ContractAssetBalance, changes ...ingest.Change,
) (inserted []xdr.Hash, removed []xdr.Hash, statInserts []history.ContractAssetStatRow) {
	ctx := context.Background()
	q := &history.MockQAssetStats{}

	q.On("InsertContractAssetBalances", ctx, mock.Anything).Run(func(args mock.Arguments) {
		for _, row := range args.Get(1).([]history.ContractAssetBalance) {
			var keyHash xdr.Hash
			copy(keyHash[:], row.KeyHash)
			inserted = append(inserted, keyHash)
		}
	}).Return(nil).Maybe()
	q.On("RemoveContractAssetBalances", ctx, mock.Anything).Run(func(args mock.Arguments) {
		removed = append(removed, args.Get(1).([]xdr.Hash)...)
	}).Return(nil).Maybe()

	q.On("GetContractAssetBalances", ctx, mock.Anything).Return(storedRows, nil).Maybe()

	q.On("InsertAssetContracts", ctx, mock.Anything).Return(nil).Maybe()
	q.On("UpdateContractAssetBalanceAmounts", ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	q.On("UpdateContractAssetBalanceExpirations", ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	q.On("UpdateAssetContractExpirations", ctx, mock.Anything, mock.Anything).Return(nil).Maybe()
	q.On("DeleteContractAssetBalancesExpiringAt", ctx, mock.Anything).
		Return([]history.ContractAssetBalance{}, nil).Maybe()
	q.On("DeleteAssetContractsExpiringAt", ctx, mock.Anything).Return(int64(0), nil).Maybe()
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

	return inserted, removed, statInserts
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

func ttlCreate(keyHash xdr.Hash, liveUntil uint32) ingest.Change {
	return ingest.Change{
		Type: xdr.LedgerEntryTypeTtl,
		Post: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeTtl,
			Ttl: &xdr.TtlEntry{
				KeyHash:            keyHash,
				LiveUntilLedgerSeq: xdr.Uint32(liveUntil),
			},
		}},
	}
}

func ttlRemove(keyHash xdr.Hash, liveUntil uint32) ingest.Change {
	return ingest.Change{
		Type: xdr.LedgerEntryTypeTtl,
		Pre: &xdr.LedgerEntry{Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeTtl,
			Ttl: &xdr.TtlEntry{
				KeyHash:            keyHash,
				LiveUntilLedgerSeq: xdr.Uint32(liveUntil),
			},
		}},
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

	// value rewritten, ledger key unchanged
	rewritten := *balance.ContractData
	val := xdr.Uint32(1)
	rewritten.Val = xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &val}
	rewrittenData := xdr.LedgerEntryData{
		Type:         xdr.LedgerEntryTypeContractData,
		ContractData: &rewritten,
	}

	_, _, ok := sac.ContractBalanceFromContractData(
		xdr.LedgerEntry{Data: rewrittenData}, "passphrase",
	)
	assert.False(t, ok, "rewritten entry must no longer resemble a balance")

	inserted, _, _ := runContractDataLedger(t, 100, nil,
		contractDataChange(nil, &balance),
		ttlCreate(keyHash, 5000),
	)
	assert.Equal(t, []xdr.Hash{keyHash}, inserted)

	_, removed, _ := runContractDataLedger(t, 101, nil, contractDataChange(&rewrittenData, nil))
	assert.Equal(t, []xdr.Hash{keyHash}, removed)
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

	// value rewritten, ledger key unchanged
	rewritten := *balance.ContractData
	val := xdr.Uint32(1)
	rewritten.Val = xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &val}
	rewrittenData := xdr.LedgerEntryData{
		Type:         xdr.LedgerEntryTypeContractData,
		ContractData: &rewritten,
	}

	storedRows := []history.ContractAssetBalance{
		{
			KeyHash:          keyHash[:],
			ContractID:       assetContractID[:],
			Amount:           "1000",
			ExpirationLedger: 5000,
		},
	}

	_, removed, statInserts := runContractDataLedger(t, 101, storedRows,
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
// does resemble a balance but which was never ingested as one, because it only
// came to resemble a balance after it was created. With no stored row there is no
// stat to adjust, whatever the removed entry's value looks like.
func TestContractDataRemovalWithoutStoredRowLeavesStats(t *testing.T) {
	assetContractID := [32]byte{0xAA}
	holderID := [32]byte{0xBB}

	balance := sac.BalanceToContractData(assetContractID, holderID, 100)
	keyHash := getKeyHashForBalance(t, assetContractID, holderID)

	_, removed, statInserts := runContractDataLedger(t, 102, nil,
		contractDataChange(&balance, nil),
		ttlRemove(keyHash, 5000),
	)

	assert.Equal(t, []xdr.Hash{keyHash}, removed)
	assert.Empty(t, statInserts)
}
