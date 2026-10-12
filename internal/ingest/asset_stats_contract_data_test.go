package ingest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"maps"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"

	ingestsdk "github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/support/db"
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

// verify runs the state verification of asset stats with the ledger entries in the
// model, the way verifyState builds them from the history archive.
func (c *contractDataTest) verify(ledger uint32) error {
	set := processors.NewContractAssetStatSet(
		c.q,
		contractDataTestPassphrase,
		map[xdr.Hash]uint32{},
		maps.Clone(c.ttl),
		map[xdr.Hash][2]uint32{},
		ledger,
	)
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
// match the ledger, and the state verification of asset stats must pass.
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
					require.NoError(t, c.verify(ledger), "state verification at ledger %d", ledger)
				}
			}
		})
	}
}
