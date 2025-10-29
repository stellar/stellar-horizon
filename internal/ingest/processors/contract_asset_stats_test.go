package processors

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/ingest/sac"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/xdr"
)

func getKeyHashForBalance(t *testing.T, assetContractId, holderID [32]byte) xdr.Hash {
	ledgerKey := sac.ContractBalanceLedgerKey(assetContractId, holderID)
	bin, err := ledgerKey.MarshalBinary()
	assert.NoError(t, err)
	return sha256.Sum256(bin)
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
		map[xdr.Hash]uint32{},
		map[xdr.Hash]uint32{},
		map[xdr.Hash][2]uint32{},
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
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceInt128ToContractData(etherID, [32]byte{1}, xdr.Int128Parts{Hi: -1, Lo: 0}),
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

	assert.Empty(t, set.updatedBalances)
	assert.Empty(t, set.removedBalances)
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
	}, set.createdBalances)
	assert.ElementsMatch(t, set.GetContractStats(), []history.ContractAssetStatRow{
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

func TestGetCreatedAssetContractsRequiresExpiration(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	set := NewContractAssetStatSet(
		&history.MockQAssetStats{},
		"passphrase",
		map[xdr.Hash]uint32{},
		map[xdr.Hash]uint32{},
		map[xdr.Hash][2]uint32{},
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

	delete(set.createdExpirationEntries, usdcAssetKeyHash)
	_, err = set.GetCreatedAssetContracts()
	assert.ErrorContains(t, err, "could not find expiration ledger entry for asset contract")
}

func TestUpdateContractBalance(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)
	etherIssuer := keypair.MustRandom().Address()
	etherAsset := xdr.MustNewCreditAsset("ETHER", etherIssuer)
	etherID, err := etherAsset.ContractID("passphrase")
	assert.NoError(t, err)
	btcAsset := xdr.MustNewCreditAsset("BTC", etherIssuer)
	btcID, err := btcAsset.ContractID("passphrase")
	assert.NoError(t, err)
	uniAsset := xdr.MustNewCreditAsset("UNI", etherIssuer)
	uniID, err := uniAsset.ContractID("passphrase")
	assert.NoError(t, err)

	mockQ := &history.MockQAssetStats{}
	set := NewContractAssetStatSet(
		mockQ,
		"passphrase",
		map[xdr.Hash]uint32{},
		map[xdr.Hash]uint32{},
		map[xdr.Hash][2]uint32{},
		150,
	)
	expectedBalances := map[xdr.Hash]string{}

	keyHash := getKeyHashForBalance(t, usdcID, [32]byte{})
	set.updatedExpirationEntries[keyHash] = [2]uint32{160, 170}
	expectedBalances[keyHash] = "100"
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{}, 50),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{}, 100),
		},
	})
	assert.NoError(t, err)

	keyHash = getKeyHashForBalance(t, usdcID, [32]byte{2})
	expectedBalances[keyHash] = "100"
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{2}, 30),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{2}, 100),
		},
	})
	assert.NoError(t, err)

	keyHash = getKeyHashForBalance(t, etherID, [32]byte{})
	expectedBalances[keyHash] = "50"
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{}, 200),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{}, 50),
		},
	})
	assert.NoError(t, err)

	// negative balances will be ignored
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{}, 200),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceInt128ToContractData(etherID, [32]byte{1}, xdr.Int128Parts{Hi: -1, Lo: 0}),
		},
	})
	assert.NoError(t, err)

	// negative balances will be ignored
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceInt128ToContractData(etherID, [32]byte{1}, xdr.Int128Parts{Hi: -1, Lo: 0}),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(etherID, [32]byte{}, 200),
		},
	})
	assert.NoError(t, err)

	// balances where the amount doesn't change will be ignored
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(btcID, [32]byte{2}, 300),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(btcID, [32]byte{2}, 300),
		},
	})
	assert.NoError(t, err)

	keyHash = getKeyHashForBalance(t, uniID, [32]byte{4})
	set.updatedExpirationEntries[keyHash] = [2]uint32{150, 170}
	expectedBalances[keyHash] = "75"
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(uniID, [32]byte{4}, 50),
		},
		Post: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(uniID, [32]byte{4}, 75),
		},
	})
	assert.NoError(t, err)

	assert.Empty(t, set.createdAssetContracts)
	assert.Empty(t, set.removedBalances)
	assert.Empty(t, set.createdExpirationEntries)
	for key, amt := range set.updatedBalances {
		assert.Equal(t, expectedBalances[key], amt.String())
		delete(expectedBalances, key)
	}
	assert.Empty(t, expectedBalances)

	result := set.GetContractStats()
	assert.ElementsMatch(t, result, []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat: history.ContractStat{
				ActiveBalance: "120",
				ActiveHolders: 0,
			},
		},
		{
			ContractID: etherID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-150",
				ActiveHolders: 0,
			},
		},
		{
			ContractID: uniID[:],
			Stat: history.ContractStat{
				ActiveBalance: "25",
				ActiveHolders: 0,
			},
		},
	})

	mockQ.AssertExpectations(t)
}

func TestRemoveContractData(t *testing.T) {
	usdcIssuer := keypair.MustRandom().Address()
	usdcAsset := xdr.MustNewCreditAsset("USDC", usdcIssuer)
	usdcID, err := usdcAsset.ContractID("passphrase")
	assert.NoError(t, err)

	set := NewContractAssetStatSet(
		&history.MockQAssetStats{},
		"passphrase",
		map[xdr.Hash]uint32{},
		map[xdr.Hash]uint32{},
		map[xdr.Hash][2]uint32{},
		150,
	)

	keyHash := getKeyHashForBalance(t, usdcID, [32]byte{})
	set.removedExpirationEntries[keyHash] = 170
	err = set.AddContractData(ingest.Change{
		Type: xdr.LedgerEntryTypeContractData,
		Pre: &xdr.LedgerEntry{
			Data: sac.BalanceToContractData(usdcID, [32]byte{}, 50),
		},
	})
	assert.NoError(t, err)

	keyHash1 := getKeyHashForBalance(t, usdcID, [32]byte{1})
	set.removedExpirationEntries[keyHash1] = 100
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

	assert.Equal(t, []xdr.Hash{keyHash, keyHash1, keyHash2}, set.removedBalances)
	assert.Empty(t, set.createdAssetContracts)

	assert.ElementsMatch(t, set.GetContractStats(), []history.ContractAssetStatRow{
		{
			ContractID: usdcID[:],
			Stat: history.ContractStat{
				ActiveBalance: "-50",
				ActiveHolders: -1,
			},
		},
	})
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
		map[xdr.Hash]uint32{},
		map[xdr.Hash]uint32{},
		map[xdr.Hash][2]uint32{},
		150,
	)

	usdcKeyHash := getKeyHashForBalance(t, usdcID, [32]byte{})
	usdcKeyHash1 := getKeyHashForBalance(t, usdcID, [32]byte{1})
	ethKeyHash := getKeyHashForBalance(t, etherID, [32]byte{})
	ethKeyHash1 := getKeyHashForBalance(t, etherID, [32]byte{1})
	set.updatedExpirationEntries[ethKeyHash1] = [2]uint32{149, 180}
	ctx := context.Background()
	mockQ.On("DeleteContractAssetBalancesExpiringAt", ctx, set.currentLedger-1).
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
				ExpirationLedger: 149,
			},
			{
				KeyHash:          ethKeyHash[:],
				ContractID:       etherID[:],
				Amount:           "8",
				ExpirationLedger: 149,
			},
			{
				KeyHash:          ethKeyHash1[:],
				ContractID:       etherID[:],
				Amount:           "67",
				ExpirationLedger: 149,
			},
		}, nil).Once()

	assert.NoError(t, set.ingestExpiredBalances(ctx))
	assert.ElementsMatch(t, set.GetContractStats(), []history.ContractAssetStatRow{
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
