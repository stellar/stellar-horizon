package history

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/test"
)

// manyKeys is the number of keys passed in the batch removal tests.
const manyKeys = 70000

// withUnknownKeys returns key followed by keys of rows which do not exist, so that
// there are manyKeys keys in all.
func withUnknownKeys[T any](key T, unknown func(i int) T) []T {
	keys := []T{key}
	for i := 1; i < manyKeys; i++ {
		keys = append(keys, unknown(i))
	}
	return keys
}

func TestRemoveAccountsManyKeys(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}

	require.NoError(t, q.UpsertAccounts(tt.Ctx, []AccountEntry{account1, account2}))

	rows, err := q.RemoveAccounts(tt.Ctx, withUnknownKeys(account1.AccountID, func(i int) string {
		return fmt.Sprintf("unknown-%d", i)
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	accounts, err := q.GetAccountsByIDs(tt.Ctx, []string{account1.AccountID, account2.AccountID})
	require.NoError(t, err)
	require.Len(t, accounts, 1)
	assert.Equal(t, account2.AccountID, accounts[0].AccountID)
}

func TestRemoveAccountDataManyKeys(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}

	require.NoError(t, q.UpsertAccountData(tt.Ctx, []Data{data1, data2}))

	key := AccountDataKey{AccountID: data1.AccountID, DataName: data1.Name}
	rows, err := q.RemoveAccountData(tt.Ctx, withUnknownKeys(key, func(i int) AccountDataKey {
		return AccountDataKey{AccountID: data1.AccountID, DataName: fmt.Sprintf("unknown-%d", i)}
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	other := AccountDataKey{AccountID: data2.AccountID, DataName: data2.Name}
	datas, err := q.GetAccountDataByKeys(tt.Ctx, []AccountDataKey{key, other})
	require.NoError(t, err)
	require.Len(t, datas, 1)
	assert.Equal(t, data2.Name, datas[0].Name)
}

func TestRemoveClaimableBalancesManyKeys(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}
	require.NoError(t, q.BeginTx(tt.Ctx, &sql.TxOptions{}))
	defer func() {
		_ = q.Rollback()
	}()

	accountID := "GC3C4AKRBQLHOJ45U4XG35ESVWRDECWO5XLDGYADO6DPR3L7KIDVUMML"
	balanceID := func(i int) string {
		hash := xdr.Hash(sha256.Sum256([]byte(fmt.Sprintf("balance-%d", i))))
		id, err := xdr.MarshalHex(xdr.ClaimableBalanceId{
			Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
			V0:   &hash,
		})
		require.NoError(t, err)
		return id
	}
	balanceBatchInsertBuilder := q.NewClaimableBalanceBatchInsertBuilder()
	claimantsBatchInsertBuilder := q.NewClaimableBalanceClaimantBatchInsertBuilder()
	for _, id := range []string{balanceID(0), balanceID(1)} {
		require.NoError(t, balanceBatchInsertBuilder.Add(ClaimableBalance{
			BalanceID: id,
			Claimants: []Claimant{{
				Destination: accountID,
				Predicate:   xdr.ClaimPredicate{Type: xdr.ClaimPredicateTypeClaimPredicateUnconditional},
			}},
			Asset:              xdr.MustNewCreditAsset("USD", accountID),
			LastModifiedLedger: 123,
			Amount:             10,
		}))
		require.NoError(t, claimantsBatchInsertBuilder.Add(ClaimableBalanceClaimant{
			BalanceID:          id,
			Destination:        accountID,
			LastModifiedLedger: 123,
		}))
	}
	require.NoError(t, balanceBatchInsertBuilder.Exec(tt.Ctx))
	require.NoError(t, claimantsBatchInsertBuilder.Exec(tt.Ctx))

	// The unknown ids start at 2, after the ids of the two stored balances.
	ids := withUnknownKeys(balanceID(0), func(i int) string { return balanceID(i + 1) })
	removed, err := q.RemoveClaimableBalances(tt.Ctx, ids)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)
	removed, err = q.RemoveClaimableBalanceClaimants(tt.Ctx, ids)
	require.NoError(t, err)
	assert.Equal(t, int64(1), removed)

	var cbs []ClaimableBalance
	require.NoError(t, q.Select(tt.Ctx, &cbs, selectClaimableBalances))
	require.Len(t, cbs, 1)
	assert.Equal(t, balanceID(1), cbs[0].BalanceID)
}

func TestRemoveContractAssetBalancesManyKeys(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}
	require.NoError(t, q.Begin(tt.Ctx))
	defer func() {
		_ = q.Rollback()
	}()

	keyHash := xdr.Hash{1}
	otherKeyHash := xdr.Hash{2}
	contractID := [32]byte{3}
	balances := []ContractAssetBalance{
		{KeyHash: keyHash[:], ContractID: contractID[:], Amount: "100", ExpirationLedger: 10},
		{KeyHash: otherKeyHash[:], ContractID: contractID[:], Amount: "101", ExpirationLedger: 11},
	}
	require.NoError(t, q.InsertContractAssetBalances(tt.Ctx, balances))

	require.NoError(t, q.RemoveContractAssetBalances(tt.Ctx, withUnknownKeys(keyHash, func(i int) xdr.Hash {
		return xdr.Hash(sha256.Sum256([]byte(fmt.Sprintf("unknown-%d", i))))
	})))

	remaining, err := q.GetContractAssetBalances(tt.Ctx, []xdr.Hash{keyHash, otherKeyHash})
	require.NoError(t, err)
	assertContractAssetBalancesEqual(t, remaining, balances[1:])
}
