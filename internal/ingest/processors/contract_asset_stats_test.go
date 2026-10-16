package processors

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2/history"
)

func getKeyHashForBalance(t *testing.T, assetContractId, holderID [32]byte) xdr.Hash {
	keyHash, err := getLedgerKeyHash(sac.ContractBalanceLedgerKey(assetContractId, holderID))
	assert.NoError(t, err)
	return keyHash
}

func getKeyHash(ledgerEntry xdr.LedgerEntry) (xdr.Hash, error) {
	ledgerKey, err := ledgerEntry.LedgerKey()
	if err != nil {
		return xdr.Hash{}, err
	}
	return getLedgerKeyHash(ledgerKey)
}

func TestAddContractData(t *testing.T) {
	xlmID, err := xdr.MustNewNativeAsset().ContractID("passphrase")
	assert.NoError(t, err)
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)
	etherIssuer := keypair.MustRandom().Address()
	etherAsset := xdr.MustNewCreditAsset("ETHER", etherIssuer)
	etherID, err := etherAsset.ContractID("passphrase")
	assert.NoError(t, err)
	uniAsset := xdr.MustNewCreditAsset("UNI", etherIssuer)
	uniID, err := uniAsset.ContractID("passphrase")
	assert.NoError(t, err)

	set := NewContractAssetStatSet(
		&history.MockQAssetStats{},
		"passphrase",
		150,
	)

	xlmContractData, err := sac.AssetToContractData(true, "", "", xlmID)
	assert.NoError(t, err)
	xlmAssetKeyHash, err := getKeyHash(xdr.LedgerEntry{
		Data: xlmContractData,
	})
	assert.NoError(t, err)
	set.createdExpirationEntries[xlmAssetKeyHash] = 160
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: xlmContractData,
		},
	})
	assert.NoError(t, err)

	uniContractData, err := sac.AssetToContractData(false, "UNI", etherIssuer, uniID)
	assert.NoError(t, err)
	uniAssetKeyHash, err := getKeyHash(xdr.LedgerEntry{
		Data: uniContractData,
	})
	assert.NoError(t, err)
	set.createdExpirationEntries[uniAssetKeyHash] = 140
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: uniContractData,
		},
	})
	assert.NoError(t, err)

	xlmBalanceKeyHash := getKeyHashForBalance(t, xlmID, [32]byte{})
	assert.NoError(t, err)
	set.createdExpirationEntries[xlmBalanceKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(xlmID, [32]byte{}, 100),
		},
	})
	assert.NoError(t, err)

	uniBalanceKeyHash := getKeyHashForBalance(t, uniID, [32]byte{})
	set.createdExpirationEntries[uniBalanceKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(uniID, [32]byte{}, 0),
		},
	})
	assert.NoError(t, err)

	usdcContractData, err := sac.AssetToContractData(false, "USDC", usdcIssuer, usdcID)
	assert.NoError(t, err)
	usdcAssetKeyHash, err := getKeyHash(xdr.LedgerEntry{
		Data: usdcContractData,
	})
	assert.NoError(t, err)
	set.createdExpirationEntries[usdcAssetKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: usdcContractData,
		},
	})
	assert.NoError(t, err)

	etherContractData, err := sac.AssetToContractData(false, "ETHER", etherIssuer, etherID)
	assert.NoError(t, err)
	etherAssetKeyHash, err := getKeyHash(xdr.LedgerEntry{
		Data: etherContractData,
	})
	assert.NoError(t, err)
	set.createdExpirationEntries[etherAssetKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: etherContractData,
		},
	})
	assert.NoError(t, err)

	etherBalanceKeyHash := getKeyHashForBalance(t, etherID, [32]byte{})
	set.createdExpirationEntries[etherBalanceKeyHash] = 100
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{}, 50),
		},
	})
	assert.NoError(t, err)

	otherEtherBalanceKeyHash := getKeyHashForBalance(t, etherID, [32]byte{1})
	set.createdExpirationEntries[otherEtherBalanceKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{1}, 150),
		},
	})
	assert.NoError(t, err)

	// negative balances will be ignored
	negativeBalanceKeyHash := getKeyHashForBalance(t, etherID, [32]byte{3})
	set.createdExpirationEntries[negativeBalanceKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceInt128ToContractData(etherID, [32]byte{3}, xdr.Int128Parts{Hi: -1, Lo: 0}),
		},
	})
	assert.NoError(t, err)

	btcAsset := xdr.MustNewCreditAsset("BTC", etherIssuer)
	btcID, err := btcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(btcID, [32]byte{2}, 300),
		},
	})
	assert.NoError(t, err)

	createdAssetContracts, err := set.GetCreatedAssetContracts()
	assert.NoError(t, err)
	assert.Equal(t, []history.AssetContract{
		{
			KeyHash:          usdcAssetKeyHash[:],
			ContractID:       usdcID[:],
			AssetType:        xdr.AssetTypeAssetTypeCreditAlphanum4,
			AssetCode:        "USDC",
			AssetIssuer:      usdcIssuer,
			ExpirationLedger: 150,
		},
		{
			KeyHash:          etherAssetKeyHash[:],
			ContractID:       etherID[:],
			AssetType:        xdr.AssetTypeAssetTypeCreditAlphanum12,
			AssetCode:        "ETHER",
			AssetIssuer:      etherIssuer,
			ExpirationLedger: 150,
		},
	}, createdAssetContracts)
	createdBalances, statRows := set.GetCreatedBalancesAndStats()
	assert.Equal(t, []history.ContractAssetBalance{
		{
			KeyHash:          uniBalanceKeyHash[:],
			ContractID:       uniID[:],
			Amount:           "0",
			ExpirationLedger: 150,
		},
		{
			KeyHash:          otherEtherBalanceKeyHash[:],
			ContractID:       etherID[:],
			Amount:           "150",
			ExpirationLedger: 150,
		},
	}, createdBalances)
	assert.ElementsMatch(t, statRows, []history.ContractAssetStatRow{
		{
			ContractID: uniID[:],
			Stat: history.ContractStat{
				ActiveBalance: "0",
				ActiveHolders: 1,
			},
		},
		{
			ContractID: etherID[:],
			Stat: history.ContractStat{
				ActiveBalance: "150",
				ActiveHolders: 1,
			},
		},
	})
}

func TestGetCreatedAssetContractsSkipsMissingExpiration(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	set := NewContractAssetStatSet(
		&history.MockQAssetStats{},
		"passphrase",
		150,
	)

	usdcContractData, err := sac.AssetToContractData(false, "USDC", usdcIssuer, usdcID)
	assert.NoError(t, err)
	usdcAssetKeyHash, err := getKeyHash(xdr.LedgerEntry{
		Data: usdcContractData,
	})
	assert.NoError(t, err)
	set.createdExpirationEntries[usdcAssetKeyHash] = 150
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: usdcContractData,
		},
	})
	assert.NoError(t, err)

	rows, err := set.GetCreatedAssetContracts()
	assert.NoError(t, err)
	assert.Len(t, rows, 1)

	delete(set.createdExpirationEntries, usdcAssetKeyHash)
	rows, err = set.GetCreatedAssetContracts()
	assert.NoError(t, err)
	assert.Empty(t, rows)
}

func assetContractID(t *testing.T, code, issuer string) xdr.ContractId {
	contractID, err := xdr.MustNewCreditAsset(code, issuer).ContractID("passphrase")
	assert.NoError(t, err)
	return contractID
}

// reconcileTest adds updates of contract balance entries to a ContractAssetStatSet,
// along with the rows the db holds for them.
type reconcileTest struct {
	t      *testing.T
	mockQ  *history.MockQAssetStats
	set    *ContractAssetStatSet
	keys   []xdr.Hash
	stored []history.ContractAssetBalance
}

func newReconcileTest(t *testing.T) *reconcileTest {
	mockQ := &history.MockQAssetStats{}
	return &reconcileTest{
		t:     t,
		mockQ: mockQ,
		set: NewContractAssetStatSet(
			mockQ,
			"passphrase",
			150,
		),
	}
}

func (r *reconcileTest) update(contractID xdr.ContractId, holder [32]byte, pre, post xdr.LedgerEntryData) xdr.Hash {
	keyHash := getKeyHashForBalance(r.t, contractID, holder)
	r.keys = append(r.keys, keyHash)
	assert.NoError(r.t, r.set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre:  &xdr.LedgerEntry{Data: pre},
		Post: &xdr.LedgerEntry{Data: post},
	}))
	return keyHash
}

func (r *reconcileTest) store(keyHash xdr.Hash, contractID xdr.ContractId, amount string, expirationLedger uint32) {
	r.stored = append(r.stored, history.ContractAssetBalance{
		KeyHash:          keyHash[:],
		ContractID:       contractID[:],
		Amount:           amount,
		ExpirationLedger: expirationLedger,
	})
}

func (r *reconcileTest) reconcile() ([]history.ContractAssetBalance, []xdr.Hash) {
	ctx := context.Background()
	r.mockQ.On("GetContractAssetBalances", ctx, r.keys).Return(r.stored, nil).Once()
	upserts, removals, err := r.set.reconcileBalances(ctx)
	assert.NoError(r.t, err)
	assert.Empty(r.t, r.set.createdAssetContracts)
	r.mockQ.AssertExpectations(r.t)
	return upserts, removals
}

func TestReconcileBalanceAmounts(t *testing.T) {
	usdcID := assetContractID(t, "USDC", keypair.MustRandom().Address())
	etherIssuer := keypair.MustRandom().Address()
	etherID := assetContractID(t, "ETHER", etherIssuer)
	btcID := assetContractID(t, "BTC", etherIssuer)
	uniID := assetContractID(t, "UNI", etherIssuer)
	r := newReconcileTest(t)

	// the amount and ttl change
	usdcKeyHash := r.update(usdcID, [32]byte{},
		sac.BalanceToContractData(usdcID, [32]byte{}, 50),
		sac.BalanceToContractData(usdcID, [32]byte{}, 100),
	)
	r.set.updatedExpirationEntries[usdcKeyHash] = [2]uint32{160, 170}
	r.store(usdcKeyHash, usdcID, "50", 160)

	// the amount changes and the ttl is kept from the stored row
	usdcKeyHash2 := r.update(usdcID, [32]byte{2},
		sac.BalanceToContractData(usdcID, [32]byte{2}, 30),
		sac.BalanceToContractData(usdcID, [32]byte{2}, 100),
	)
	r.store(usdcKeyHash2, usdcID, "30", 165)

	etherKeyHash := r.update(etherID, [32]byte{},
		sac.BalanceToContractData(etherID, [32]byte{}, 200),
		sac.BalanceToContractData(etherID, [32]byte{}, 50),
	)
	r.store(etherKeyHash, etherID, "200", 165)

	// balances where the amount and ttl don't change are ignored
	btcKeyHash := r.update(btcID, [32]byte{2},
		sac.BalanceToContractData(btcID, [32]byte{2}, 300),
		sac.BalanceToContractData(btcID, [32]byte{2}, 300),
	)
	r.store(btcKeyHash, btcID, "300", 165)

	uniKeyHash := r.update(uniID, [32]byte{4},
		sac.BalanceToContractData(uniID, [32]byte{4}, 50),
		sac.BalanceToContractData(uniID, [32]byte{4}, 75),
	)
	r.set.updatedExpirationEntries[uniKeyHash] = [2]uint32{150, 170}
	r.store(uniKeyHash, uniID, "50", 150)

	upserts, removals := r.reconcile()
	assert.Empty(t, removals)
	assert.Equal(t, []history.ContractAssetBalance{
		{KeyHash: usdcKeyHash[:], ContractID: usdcID[:], Amount: "100", ExpirationLedger: 170},
		{KeyHash: usdcKeyHash2[:], ContractID: usdcID[:], Amount: "100", ExpirationLedger: 165},
		{KeyHash: etherKeyHash[:], ContractID: etherID[:], Amount: "50", ExpirationLedger: 165},
		{KeyHash: uniKeyHash[:], ContractID: uniID[:], Amount: "75", ExpirationLedger: 170},
	}, upserts)
	assert.ElementsMatch(t, r.set.contractAssetStats.rows(), []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat:       history.ContractStat{ActiveBalance: "120", ActiveHolders: 0},
		},
		{
			ContractID: etherID[:],
			Stat:       history.ContractStat{ActiveBalance: "-150", ActiveHolders: 0},
		},
		{
			ContractID: uniID[:],
			Stat:       history.ContractStat{ActiveBalance: "25", ActiveHolders: 0},
		},
	})
}

// TestReconcileBalanceValues covers updates which change whether the value of an
// entry is a balance. The row follows the latest value, whatever the previous one was.
func TestReconcileBalanceValues(t *testing.T) {
	etherIssuer := keypair.MustRandom().Address()
	etherID := assetContractID(t, "ETHER", etherIssuer)
	btcID := assetContractID(t, "BTC", etherIssuer)
	negative := xdr.Int128Parts{Hi: -1, Lo: 0}
	r := newReconcileTest(t)

	// a value which is not a valid balance removes the row
	etherKeyHash := r.update(etherID, [32]byte{1},
		sac.BalanceToContractData(etherID, [32]byte{1}, 40),
		sac.BalanceInt128ToContractData(etherID, [32]byte{1}, negative),
	)
	r.store(etherKeyHash, etherID, "40", 165)

	// a valid balance replacing a value which was not one is stored when its ttl is known
	btcKeyHash := r.update(btcID, [32]byte{5},
		sac.BalanceInt128ToContractData(btcID, [32]byte{5}, negative),
		sac.BalanceToContractData(btcID, [32]byte{5}, 10),
	)
	r.set.updatedExpirationEntries[btcKeyHash] = [2]uint32{160, 180}

	// and skipped when it is not
	r.update(btcID, [32]byte{6},
		sac.BalanceInt128ToContractData(btcID, [32]byte{6}, negative),
		sac.BalanceToContractData(btcID, [32]byte{6}, 10),
	)

	upserts, removals := r.reconcile()
	assert.Equal(t, []xdr.Hash{etherKeyHash}, removals)
	assert.Equal(t, []history.ContractAssetBalance{
		{KeyHash: btcKeyHash[:], ContractID: btcID[:], Amount: "10", ExpirationLedger: 180},
	}, upserts)
	assert.ElementsMatch(t, r.set.contractAssetStats.rows(), []history.ContractAssetStatRow{
		{
			ContractID: etherID[:],
			Stat:       history.ContractStat{ActiveBalance: "-40", ActiveHolders: -1},
		},
		{
			ContractID: btcID[:],
			Stat:       history.ContractStat{ActiveBalance: "10", ActiveHolders: 1},
		},
	})
}

// TestReconcileBalanceContract covers a stored row whose contract is not the contract
// of its ledger key. The row is rewritten, and its balance moves to the stats of the
// contract of the key.
func TestReconcileBalanceContract(t *testing.T) {
	usdcID := assetContractID(t, "USDC", keypair.MustRandom().Address())
	otherID := assetContractID(t, "EURC", keypair.MustRandom().Address())
	r := newReconcileTest(t)

	keyHash := r.update(usdcID, [32]byte{1},
		sac.BalanceToContractData(usdcID, [32]byte{1}, 50),
		sac.BalanceToContractData(usdcID, [32]byte{1}, 50),
	)
	r.store(keyHash, otherID, "50", 160)

	upserts, removals := r.reconcile()
	assert.Empty(t, removals)
	assert.Equal(t, []history.ContractAssetBalance{
		{KeyHash: keyHash[:], ContractID: usdcID[:], Amount: "50", ExpirationLedger: 160},
	}, upserts)
	assert.ElementsMatch(t, r.set.contractAssetStats.rows(), []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat:       history.ContractStat{ActiveBalance: "50", ActiveHolders: 1},
		},
		{
			ContractID: otherID[:],
			Stat:       history.ContractStat{ActiveBalance: "-50", ActiveHolders: -1},
		},
	})
}

func TestRemoveContractData(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	mockQ := &history.MockQAssetStats{}
	set := NewContractAssetStatSet(
		mockQ,
		"passphrase",
		150,
	)

	keyHash := getKeyHashForBalance(t, usdcID, [32]byte{})
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{}, 50),
		},
	})
	assert.NoError(t, err)

	keyHash1 := getKeyHashForBalance(t, usdcID, [32]byte{1})
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{1}, 20),
		},
	})
	assert.NoError(t, err)

	keyHash2 := getKeyHashForBalance(t, usdcID, [32]byte{2})
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{2}, 34),
		},
	})
	assert.NoError(t, err)
	assert.Empty(t, set.createdAssetContracts)

	// keyHash1 has no stored row, so it contributes nothing to the stats
	ctx := context.Background()
	mockQ.On("GetContractAssetBalances", ctx, []xdr.Hash{keyHash, keyHash1, keyHash2}).
		Return([]history.ContractAssetBalance{
			{KeyHash: keyHash[:], ContractID: usdcID[:], Amount: "50", ExpirationLedger: 170},
			{KeyHash: keyHash2[:], ContractID: usdcID[:], Amount: "34", ExpirationLedger: 170},
		}, nil).Once()
	upserts, removals, err := set.reconcileBalances(ctx)
	assert.NoError(t, err)
	assert.Empty(t, upserts)
	assert.Equal(t, []xdr.Hash{keyHash, keyHash2}, removals)

	assert.ElementsMatch(t, set.contractAssetStats.rows(), []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-84",
				ActiveHolders: -2,
			},
		},
	})
	mockQ.AssertExpectations(t)
}

func TestIngestExpiredBalances(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	etherIssuer := keypair.MustRandom().Address()
	etherAsset := xdr.MustNewCreditAsset("ETHER", etherIssuer)
	etherID, err := etherAsset.ContractID("passphrase")
	assert.NoError(t, err)

	mockQ := &history.MockQAssetStats{}
	set := NewContractAssetStatSet(
		mockQ,
		"passphrase",
		150,
	)

	usdcKeyHash := getKeyHashForBalance(t, usdcID, [32]byte{})
	usdcKeyHash1 := getKeyHashForBalance(t, usdcID, [32]byte{1})
	ethKeyHash := getKeyHashForBalance(t, etherID, [32]byte{})
	ctx := context.Background()
	mockQ.On("DeleteContractAssetBalancesExpiredBefore", ctx, set.currentLedger).
		Return([]history.ContractAssetBalance{
			{
				KeyHash:          usdcKeyHash[:],
				ContractID:       usdcID[:],
				Amount:           "67",
				ExpirationLedger: 149,
			},
			{
				KeyHash:          usdcKeyHash1[:],
				ContractID:       usdcID[:],
				Amount:           "200",
				ExpirationLedger: 120,
			},
			{
				KeyHash:          ethKeyHash[:],
				ContractID:       etherID[:],
				Amount:           "8",
				ExpirationLedger: 149,
			},
		}, nil).Once()

	assert.NoError(t, set.ingestExpiredBalances(ctx))
	assert.ElementsMatch(t, set.contractAssetStats.rows(), []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-267",
				ActiveHolders: -2,
			},
		},
		{
			ContractID: etherID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-8",
				ActiveHolders: -1,
			},
		},
	})
	mockQ.AssertExpectations(t)
}
