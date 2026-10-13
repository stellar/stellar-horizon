package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	ingestsdk "github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/db"
	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/ingest/processors"
	"github.com/stellar/stellar-horizon/internal/test"
)

const contractDataTestPassphrase = "contract data test network"

// contractDataTest ingests ledgers of contract data changes the way live ingestion
// does, while keeping a model of the contract data and ttl ledger entries, including
// entries which expired and were not restored.
type contractDataTest struct {
	t       *testing.T
	ctx     context.Context
	q       *history.Q
	rand    *rand.Rand
	ledger  uint32
	data    map[xdr.Hash]xdr.LedgerEntryData
	ttl     map[xdr.Hash]uint32
	changes []ingestsdk.Change

	assetContracts []xdr.ContractId
	instances      map[xdr.ContractId]xdr.LedgerEntryData
	otherContracts []xdr.ContractId
	holders        [][32]byte
}

func newContractDataTest(t *testing.T, seed int64) (*contractDataTest, func()) {
	tt := test.Start(t)
	test.ResetHorizonDB(t, tt.HorizonDB)

	c := &contractDataTest{
		t:         t,
		ctx:       tt.Ctx,
		q:         &history.Q{SessionInterface: &db.Session{DB: tt.HorizonDB}},
		rand:      rand.New(rand.NewSource(seed)), //nolint:gosec // seeded so that failures are reproducible
		ledger:    100,
		data:      map[xdr.Hash]xdr.LedgerEntryData{},
		ttl:       map[xdr.Hash]uint32{},
		instances: map[xdr.ContractId]xdr.LedgerEntryData{},
		holders:   [][32]byte{{1}, {2}, {3}, {4}},
	}
	issuer := keypair.MustRandom().Address()
	for _, code := range []string{"EUR", "USDOLLAR"} {
		contractID, err := xdr.MustNewCreditAsset(code, issuer).ContractID(contractDataTestPassphrase)
		require.NoError(t, err)
		instance, err := sac.AssetToContractData(false, code, issuer, contractID)
		require.NoError(t, err)
		c.assetContracts = append(c.assetContracts, contractID)
		c.instances[contractID] = instance
	}
	c.otherContracts = []xdr.ContractId{{0xAA}, {0xBB}}
	return c, tt.Finish
}

func contractDataKeyHash(t *testing.T, data xdr.LedgerEntryData) xdr.Hash {
	entry := xdr.LedgerEntry{Data: data}
	key, err := entry.LedgerKey()
	require.NoError(t, err)
	bin, err := key.MarshalBinary()
	require.NoError(t, err)
	return sha256.Sum256(bin)
}

func ttlEntryData(keyHash xdr.Hash, liveUntil uint32) xdr.LedgerEntryData {
	return xdr.LedgerEntryData{
		Type: xdr.LedgerEntryTypeTtl,
		Ttl:  &xdr.TtlEntry{KeyHash: keyHash, LiveUntilLedgerSeq: xdr.Uint32(liveUntil)},
	}
}

func ledgerEntryChange(changeType xdr.LedgerEntryChangeType, pre, post *xdr.LedgerEntryData) ingestsdk.Change {
	change := ingestsdk.Change{ChangeType: changeType}
	if pre != nil {
		change.Type = pre.Type
		change.Pre = &xdr.LedgerEntry{Data: *pre}
	}
	if post != nil {
		change.Type = post.Type
		change.Post = &xdr.LedgerEntry{Data: *post}
	}
	return change
}

// withValue returns a copy of a contract data entry with a different value.
func withValue(data xdr.LedgerEntryData, val xdr.ScVal) xdr.LedgerEntryData {
	contractData := *data.ContractData
	contractData.Val = val
	return xdr.LedgerEntryData{Type: xdr.LedgerEntryTypeContractData, ContractData: &contractData}
}

func (c *contractDataTest) create(data xdr.LedgerEntryData, liveUntil uint32) {
	keyHash := contractDataKeyHash(c.t, data)
	ttl := ttlEntryData(keyHash, liveUntil)
	c.data[keyHash] = data
	c.ttl[keyHash] = liveUntil
	c.changes = append(c.changes,
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryCreated, nil, &data),
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryCreated, nil, &ttl),
	)
}

func (c *contractDataTest) update(keyHash xdr.Hash, data xdr.LedgerEntryData) {
	pre := c.data[keyHash]
	c.data[keyHash] = data
	c.changes = append(c.changes, ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryUpdated, &pre, &data))
}

func (c *contractDataTest) extend(keyHash xdr.Hash, liveUntil uint32) {
	pre := ttlEntryData(keyHash, c.ttl[keyHash])
	post := ttlEntryData(keyHash, liveUntil)
	c.ttl[keyHash] = liveUntil
	c.changes = append(c.changes, ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryUpdated, &pre, &post))
}

func (c *contractDataTest) remove(keyHash xdr.Hash) {
	data := c.data[keyHash]
	ttl := ttlEntryData(keyHash, c.ttl[keyHash])
	delete(c.data, keyHash)
	delete(c.ttl, keyHash)
	c.changes = append(c.changes,
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryRemoved, &data, nil),
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryRemoved, &ttl, nil),
	)
}

func (c *contractDataTest) restore(keyHash xdr.Hash, liveUntil uint32) {
	data := c.data[keyHash]
	ttl := ttlEntryData(keyHash, liveUntil)
	c.ttl[keyHash] = liveUntil
	c.changes = append(c.changes,
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryRestored, nil, &data),
		ledgerEntryChange(xdr.LedgerEntryChangeTypeLedgerEntryRestored, nil, &ttl),
	)
}

// closeLedger ingests the changes of the current ledger with the same change
// compactor configuration and processor as live ingestion, in one db transaction.
func (c *contractDataTest) closeLedger() {
	compactor := ingestsdk.NewChangeCompactor(ingestsdk.ChangeCompactorConfig{SuppressRemoveAfterRestoreChange: true})
	for _, change := range c.changes {
		require.NoError(c.t, compactor.AddChange(change))
	}

	require.NoError(c.t, c.q.BeginTx(c.ctx, &sql.TxOptions{}))
	processor := processors.NewAssetStatsProcessor(c.q, contractDataTestPassphrase, false, c.ledger)
	for _, change := range compactor.GetChanges() {
		require.NoError(c.t, processor.ProcessChange(c.ctx, change))
	}
	require.NoError(c.t, processor.Commit(c.ctx))
	require.NoError(c.t, c.q.Commit())

	c.changes = nil
	c.ledger++
}

func (c *contractDataTest) liveUntil() uint32 {
	return c.ledger + c.rand.Uint32()%40
}

// randomValue returns the value of a balance entry. Stellar Asset Contracts always
// write balances, while other contracts can write anything.
func (c *contractDataTest) randomValue(contractID xdr.ContractId, holder [32]byte, assetContract bool) xdr.LedgerEntryData {
	balance := sac.BalanceToContractData(contractID, holder, c.rand.Uint64()%1000)
	if assetContract {
		return balance
	}
	switch c.rand.Intn(4) {
	case 0:
		val := xdr.Uint32(c.rand.Uint32() % 10)
		return withValue(balance, xdr.ScVal{Type: xdr.ScValTypeScvU32, U32: &val})
	case 1:
		return sac.BalanceInt128ToContractData(contractID, holder, xdr.Int128Parts{Hi: -1, Lo: 0})
	default:
		return balance
	}
}

// randomLedger changes a few random balance entries and asset contract instances.
func (c *contractDataTest) randomLedger() {
	contracts := append(append([]xdr.ContractId{}, c.assetContracts...), c.otherContracts...)
	changed := map[xdr.Hash]bool{}
	for range 1 + c.rand.Intn(4) {
		contractIndex := c.rand.Intn(len(contracts))
		contractID := contracts[contractIndex]
		holder := c.holders[c.rand.Intn(len(c.holders))]
		keyHash := contractDataKeyHash(c.t, sac.BalanceToContractData(contractID, holder, 0))
		if !changed[keyHash] {
			changed[keyHash] = true
			c.randomBalanceChange(keyHash, contractID, holder, contractIndex < len(c.assetContracts))
		}
	}
	c.randomInstanceChanges()
}

func (c *contractDataTest) randomBalanceChange(keyHash xdr.Hash, contractID xdr.ContractId, holder [32]byte, assetContract bool) {
	_, exists := c.data[keyHash]
	switch {
	case !exists:
		c.create(c.randomValue(contractID, holder, assetContract), c.liveUntil())
	case c.ttl[keyHash] < c.ledger:
		// archived entries can only be restored, sometimes along with an update
		if c.rand.Intn(2) == 0 {
			c.restore(keyHash, c.liveUntil())
			if c.rand.Intn(2) == 0 {
				c.update(keyHash, c.randomValue(contractID, holder, assetContract))
			}
		}
	default:
		switch c.rand.Intn(5) {
		case 0:
			c.update(keyHash, c.randomValue(contractID, holder, assetContract))
		case 1:
			c.extend(keyHash, c.ttl[keyHash]+1+c.rand.Uint32()%20)
		case 2:
			c.update(keyHash, c.randomValue(contractID, holder, assetContract))
			c.extend(keyHash, c.ttl[keyHash]+1+c.rand.Uint32()%20)
		case 3:
			c.remove(keyHash)
		}
	}
}

func (c *contractDataTest) randomInstanceChanges() {
	for _, contractID := range c.assetContracts {
		keyHash := contractDataKeyHash(c.t, c.instances[contractID])
		switch _, exists := c.data[keyHash]; {
		case !exists:
			c.create(c.instances[contractID], c.liveUntil())
		case c.ttl[keyHash] < c.ledger:
			if c.rand.Intn(10) == 0 {
				c.restore(keyHash, c.liveUntil())
			}
		case c.rand.Intn(10) == 0:
			c.extend(keyHash, c.ttl[keyHash]+1+c.rand.Uint32()%20)
		}
	}
}

// checkAssetContractBalances checks that the stored balances of the Stellar Asset
// Contracts match the balance entries which are active in the given ledger.
func (c *contractDataTest) checkAssetContractBalances(ledger uint32) {
	var keys []xdr.Hash
	expected := map[xdr.Hash]history.ContractAssetBalance{}
	for _, contractID := range c.assetContracts {
		for _, holder := range c.holders {
			keyHash := contractDataKeyHash(c.t, sac.BalanceToContractData(contractID, holder, 0))
			keys = append(keys, keyHash)
			data, ok := c.data[keyHash]
			if !ok || c.ttl[keyHash] < ledger {
				continue
			}
			_, amount, ok := sac.ContractBalanceFromContractData(xdr.LedgerEntry{Data: data}, contractDataTestPassphrase)
			require.True(c.t, ok)
			expected[keyHash] = history.ContractAssetBalance{
				KeyHash:          keyHash[:],
				ContractID:       contractID[:],
				Amount:           amount.String(),
				ExpirationLedger: c.ttl[keyHash],
			}
		}
	}

	rows, err := c.q.GetContractAssetBalances(c.ctx, keys)
	require.NoError(c.t, err)
	actual := map[xdr.Hash]history.ContractAssetBalance{}
	for _, row := range rows {
		var keyHash xdr.Hash
		copy(keyHash[:], row.KeyHash)
		actual[keyHash] = row
	}
	require.Equal(c.t, expected, actual, "contract balances of asset contracts at ledger %d", ledger)
}

// newSystem returns a system which last ingested the given ledger, and whose history
// archive holds the ledger entries of the model at that ledger.
func (c *contractDataTest) newSystem(ledger uint32) (*system, *mockHistoryArchiveAdapter) {
	reader := &memoryChangeReader{}
	addEntry := func(entry xdr.LedgerEntry) {
		reader.changes = append(reader.changes, xdr.LedgerEntryChange{
			Type:  xdr.LedgerEntryChangeTypeLedgerEntryState,
			State: &entry,
		})
	}
	for _, data := range c.data {
		if entry := (xdr.LedgerEntry{Data: data}); ledgerEntryFilter(contractDataTestPassphrase, entry) {
			addEntry(entry)
		}
	}
	for keyHash, liveUntil := range c.ttl {
		addEntry(xdr.LedgerEntry{Data: ttlEntryData(keyHash, liveUntil)})
	}

	historyAdapter := &mockHistoryArchiveAdapter{}
	historyAdapter.On("GetLatestLedgerSequence").Return(ledger, nil).Maybe()
	historyAdapter.On("GetState", mock.Anything, ledger).Return(reader, nil).Once()

	require.NoError(c.t, c.q.UpdateLastLedgerIngest(c.ctx, ledger))
	sys := &system{
		ctx:                          c.ctx,
		historyQ:                     c.q,
		historyAdapter:               historyAdapter,
		runStateVerificationOnLedger: func(uint32) bool { return true },
		config:                       Config{NetworkPassphrase: contractDataTestPassphrase},
	}
	sys.initMetrics()
	return sys, historyAdapter
}

// verifyState runs the state verification of a system whose history archive holds the
// ledger entries of the model at the given ledger.
func (c *contractDataTest) verifyState(ledger uint32) error {
	sys, historyAdapter := c.newSystem(ledger)
	err := sys.verifyState(false, ledger, xdr.Hash{})
	historyAdapter.AssertExpectations(c.t)
	return err
}

// checkAssetStats runs the check of asset stats of the state verification, in the
// current transaction of the test, with the ledger entries in the model.
func (c *contractDataTest) checkAssetStats(ledger uint32) error {
	set := processors.NewContractAssetStatSet(c.q, contractDataTestPassphrase, ledger)
	for keyHash, liveUntil := range c.ttl {
		entry := xdr.LedgerEntry{Data: ttlEntryData(keyHash, liveUntil)}
		if err := set.AddTTL(ingestsdk.Change{Type: xdr.LedgerEntryTypeTtl, Post: &entry}); err != nil {
			return err
		}
	}
	for _, data := range c.data {
		if err := set.AddContractData(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeContractData,
			Post: &xdr.LedgerEntry{Data: data},
		}); err != nil {
			return err
		}
	}
	return checkAssetStats(c.ctx, processors.NewAssetStatSet(), set, c.q)
}

// TestContractDataIngestionMatchesVerification ingests ledgers of random changes to
// contract balance entries and asset contract instances. The values written by
// contracts which are not Stellar Asset Contracts change between balances and other
// values. After every ledger the stored balances of the Stellar Asset Contracts must
// match the ledger, and every 10 ledgers the state verification must pass.
func TestContractDataIngestionMatchesVerification(t *testing.T) {
	for _, seed := range []int64{1, 2, 3} {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			c, finish := newContractDataTest(t, seed)
			defer finish()

			for range 300 {
				c.randomLedger()
				c.closeLedger()
				ledger := c.ledger - 1
				c.checkAssetContractBalances(ledger)
				if ledger%10 == 0 {
					require.NoError(t, c.verifyState(ledger), "state verification at ledger %d", ledger)
				}
			}
		})
	}
}

func (c *contractDataTest) balanceRow(
	contractID xdr.ContractId, holder [32]byte, amount string, expirationLedger uint32,
) history.ContractAssetBalance {
	keyHash := contractDataKeyHash(c.t, sac.BalanceToContractData(contractID, holder, 0))
	return history.ContractAssetBalance{
		KeyHash:          keyHash[:],
		ContractID:       contractID[:],
		Amount:           amount,
		ExpirationLedger: expirationLedger,
	}
}

type contractDataMismatchCase struct {
	name     string
	change   func() error
	expected []string
}

// contractDataMismatchCases returns changes to the stored contract data of a test whose
// first asset contract holds a balance of 10 for its first holder and of 20 for its
// second holder, along with parts of the mismatches each change must produce.
func contractDataMismatchCases(
	c *contractDataTest, liveUntil uint32, extraAssetContract history.AssetContract,
) []contractDataMismatchCase {
	assetContractID := c.assetContracts[0]
	upsertBalance := func(contractID xdr.ContractId, holder [32]byte, amount string, expirationLedger uint32) error {
		return c.q.UpsertContractAssetBalances(c.ctx, []history.ContractAssetBalance{
			c.balanceRow(contractID, holder, amount, expirationLedger),
		})
	}
	return []contractDataMismatchCase{
		{
			name:     "balance amount",
			change:   func() error { return upsertBalance(assetContractID, c.holders[0], "11", liveUntil) },
			expected: []string{"has amount 10 in HAS but 11 in db"},
		},
		{
			name:     "balance expiration",
			change:   func() error { return upsertBalance(assetContractID, c.holders[0], "10", liveUntil+1) },
			expected: []string{fmt.Sprintf("has expiration %d in HAS but %d in db", liveUntil, liveUntil+1)},
		},
		{
			name: "missing balance",
			change: func() error {
				keyHash := contractDataKeyHash(c.t, sac.BalanceToContractData(assetContractID, c.holders[1], 0))
				return c.q.RemoveContractAssetBalances(c.ctx, []xdr.Hash{keyHash})
			},
			expected: []string{"is missing from db"},
		},
		{
			name: "contract stats",
			change: func() error {
				_, err := c.q.UpdateContractAssetStat(c.ctx, history.ContractAssetStatRow{
					ContractID: assetContractID[:],
					Stat:       history.ContractStat{ActiveBalance: "31", ActiveHolders: 2},
				})
				return err
			},
			expected: []string{"in HAS but {ActiveBalance:31 ActiveHolders:2} in db"},
		},
		{
			name: "missing asset contract",
			change: func() error {
				_, err := c.q.DeleteAssetContractsExpiredBefore(c.ctx, liveUntil+1)
				return err
			},
			expected: []string{"HAS contains 1 asset contracts which are missing from db"},
		},
		{
			name:     "extra asset contract",
			change:   func() error { return c.q.InsertAssetContracts(c.ctx, []history.AssetContract{extraAssetContract}) },
			expected: []string{"db contains asset contract of USDOLLAR:"},
		},
		{
			name:   "balance of a contract which is not an asset contract",
			change: func() error { return upsertBalance(c.otherContracts[0], c.holders[0], "31", liveUntil) },
		},
	}
}

// requireContractDataError checks that err reports a difference in contract data, which
// is not a StateError.
func requireContractDataError(t *testing.T, err error, expected ...string) {
	require.Error(t, err)
	_, isStateError := errors.Cause(err).(ingestsdk.StateError)
	require.False(t, isStateError, "a difference in contract data is a StateError: %v", err)
	for _, e := range expected {
		require.Contains(t, err.Error(), e)
	}
}

// TestCheckAssetStatsReportsContractDataMismatches changes the stored contract data in
// different ways, and checks that the state verification of asset stats reports each
// difference as an error which is not a StateError. Differences in the balances of
// contracts which are not Stellar Asset Contracts are not reported. It then checks
// that the periodic state verification does not mark the state as invalid for such a
// difference.
func TestCheckAssetStatsReportsContractDataMismatches(t *testing.T) {
	c, finish := newContractDataTest(t, 1)
	defer finish()

	assetContractID := c.assetContracts[0]
	liveUntil := c.ledger + 100
	c.create(c.instances[assetContractID], liveUntil)
	c.create(sac.BalanceToContractData(assetContractID, c.holders[0], 10), liveUntil)
	c.create(sac.BalanceToContractData(assetContractID, c.holders[1], 20), liveUntil)
	c.create(sac.BalanceToContractData(c.otherContracts[0], c.holders[0], 30), liveUntil)
	c.closeLedger()
	ledger := c.ledger - 1
	require.NoError(t, c.checkAssetStats(ledger))

	// an asset contract whose instance is not in the ledger
	instance := c.instances[c.assetContracts[1]]
	asset, ok := sac.AssetFromContractData(xdr.LedgerEntry{Data: instance}, contractDataTestPassphrase)
	require.True(t, ok)
	instanceKeyHash := contractDataKeyHash(t, instance)
	extraAssetContract := history.AssetContract{
		KeyHash:          instanceKeyHash[:],
		ContractID:       c.assetContracts[1][:],
		ExpirationLedger: liveUntil,
	}
	require.NoError(t, asset.Extract(
		&extraAssetContract.AssetType, &extraAssetContract.AssetCode, &extraAssetContract.AssetIssuer,
	))

	for _, testCase := range contractDataMismatchCases(c, liveUntil, extraAssetContract) {
		t.Run(testCase.name, func(t *testing.T) {
			require.NoError(t, c.q.Begin(c.ctx))
			defer func() {
				require.NoError(t, c.q.Rollback())
			}()
			require.NoError(t, testCase.change())

			err := c.checkAssetStats(ledger)
			if len(testCase.expected) == 0 {
				require.NoError(t, err)
				return
			}
			requireContractDataError(t, err, testCase.expected...)
		})
	}

	c.checkPeriodicStateVerification(ledger, liveUntil)
}

// checkPeriodicStateVerification changes the stored balance of the first holder of the
// first asset contract, and checks that the state verification reports it as an error
// while the periodic state verification does not mark the state as invalid. A
// difference in the classic asset stats still marks the state as invalid.
func (c *contractDataTest) checkPeriodicStateVerification(ledger, liveUntil uint32) {
	t := c.t
	require.NoError(t, c.verifyState(ledger))
	require.NoError(t, c.q.UpsertContractAssetBalances(c.ctx, []history.ContractAssetBalance{
		c.balanceRow(c.assetContracts[0], c.holders[0], "11", liveUntil),
	}))
	requireContractDataError(t, c.verifyState(ledger), "has amount 10 in HAS but 11 in db")

	sys, historyAdapter := c.newSystem(ledger)
	sys.maybeVerifyState(ledger, xdr.Hash{})
	sys.wg.Wait()
	historyAdapter.AssertExpectations(t)
	require.Equal(t, 1, sys.stateVerificationErrors)
	invalid, err := c.q.GetExpStateInvalid(c.ctx)
	require.NoError(t, err)
	require.False(t, invalid)

	_, err = c.q.InsertAssetStat(c.ctx, history.ExpAssetStat{
		AssetType:   xdr.AssetTypeAssetTypeCreditAlphanum4,
		AssetCode:   "GBP",
		AssetIssuer: keypair.MustRandom().Address(),
		Accounts:    history.ExpAssetStatAccounts{Authorized: 1},
		Balances: history.ExpAssetStatBalances{
			Authorized:                      "1",
			AuthorizedToMaintainLiabilities: "0",
			ClaimableBalances:               "0",
			LiquidityPools:                  "0",
			Unauthorized:                    "0",
		},
	})
	require.NoError(t, err)
	sys, historyAdapter = c.newSystem(ledger)
	sys.maybeVerifyState(ledger, xdr.Hash{})
	sys.wg.Wait()
	historyAdapter.AssertExpectations(t)
	invalid, err = c.q.GetExpStateInvalid(c.ctx)
	require.NoError(t, err)
	require.True(t, invalid)
}

// TestCheckAssetStatsRejectsClassicAssetStatsWithoutAccounts checks that a classic
// asset stat row without accounts, which ingestion never stores, is a StateError,
// whether or not its asset has an asset contract.
func TestCheckAssetStatsRejectsClassicAssetStatsWithoutAccounts(t *testing.T) {
	c, finish := newContractDataTest(t, 1)
	defer finish()

	assetContractID := c.assetContracts[0]
	c.create(c.instances[assetContractID], c.ledger+100)
	c.closeLedger()
	ledger := c.ledger - 1
	require.NoError(t, c.checkAssetStats(ledger))

	asset, ok := sac.AssetFromContractData(xdr.LedgerEntry{Data: c.instances[assetContractID]}, contractDataTestPassphrase)
	require.True(t, ok)
	withAssetContract := history.ExpAssetStat{Balances: emptyClassicBalances}
	withAssetContract.Balances.Authorized = "5"
	require.NoError(t, asset.Extract(
		&withAssetContract.AssetType, &withAssetContract.AssetCode, &withAssetContract.AssetIssuer,
	))
	withoutAssetContract := history.ExpAssetStat{
		AssetType:   xdr.AssetTypeAssetTypeCreditAlphanum4,
		AssetCode:   "GBP",
		AssetIssuer: keypair.MustRandom().Address(),
		Balances:    emptyClassicBalances,
	}

	for _, assetStat := range []history.ExpAssetStat{withAssetContract, withoutAssetContract} {
		t.Run(assetStat.AssetCode, func(t *testing.T) {
			require.NoError(t, c.q.Begin(c.ctx))
			defer func() {
				require.NoError(t, c.q.Rollback())
			}()
			_, err := c.q.InsertAssetStat(c.ctx, assetStat)
			require.NoError(t, err)
			_, isStateError := errors.Cause(c.checkAssetStats(ledger)).(ingestsdk.StateError)
			require.True(t, isStateError)
		})
	}
}
