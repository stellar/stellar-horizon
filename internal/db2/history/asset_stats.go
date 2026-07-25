package history

import (
	"context"
	"fmt"
	"strings"

	sq "github.com/Masterminds/squirrel"

	"github.com/stellar/go-stellar-sdk/support/db"
	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2"
)

func assetStatToMap(assetStat ExpAssetStat) map[string]interface{} {
	return map[string]interface{}{
		"asset_type":   assetStat.AssetType,
		"asset_code":   assetStat.AssetCode,
		"asset_issuer": assetStat.AssetIssuer,
		"accounts":     assetStat.Accounts,
		"balances":     assetStat.Balances,
	}
}

func assetStatToPrimaryKeyMap(assetStat ExpAssetStat) map[string]interface{} {
	return map[string]interface{}{
		"asset_type":   assetStat.AssetType,
		"asset_code":   assetStat.AssetCode,
		"asset_issuer": assetStat.AssetIssuer,
	}
}

// ContractAssetStatRow represents a row in the contract_asset_stats table
type ContractAssetStatRow struct {
	// ContractID is the contract id of the stellar asset contract
	ContractID []byte `db:"contract_id"`
	// Stat is a json blob containing statistics on the contract holders
	// this asset
	Stat ContractStat `db:"stat"`
}

// InsertAssetStats a set of asset stats into the exp_asset_stats
func (q *Q) InsertAssetStats(ctx context.Context, assetStats []ExpAssetStat) error {
	if len(assetStats) == 0 {
		return nil
	}

	builder := &db.FastBatchInsertBuilder{}

	for _, assetStat := range assetStats {
		if err := builder.Row(assetStatToMap(assetStat)); err != nil {
			return errors.Wrap(err, "could not insert asset assetStat row")
		}
	}

	if err := builder.Exec(ctx, q, "exp_asset_stats"); err != nil {
		return errors.Wrap(err, "could not exec asset assetStats insert builder")
	}

	return nil
}

// InsertContractAssetStats inserts the given list of rows into the contract_asset_stats table
func (q *Q) InsertContractAssetStats(ctx context.Context, rows []ContractAssetStatRow) error {
	if len(rows) == 0 {
		return nil
	}
	builder := &db.FastBatchInsertBuilder{}

	for _, row := range rows {
		if err := builder.RowStruct(row); err != nil {
			return errors.Wrap(err, "could not insert asset assetStat row")
		}
	}

	if err := builder.Exec(ctx, q, "contract_asset_stats"); err != nil {
		return errors.Wrap(err, "could not exec asset assetStats insert builder")
	}

	return nil
}

// AssetContract represents a row in the asset_contracts table
type AssetContract struct {
	// KeyHash is a hash of the asset contract's ledger entry key
	KeyHash []byte `db:"key_hash"`
	// ContractID is the contract id of the stellar asset contract
	ContractID []byte `db:"contract_id"`
	// AssetType is the type of asset
	AssetType xdr.AssetType `db:"asset_type"`
	// AssetCode is the code for the asset
	AssetCode string `db:"asset_code"`
	// AssetIssuer is the issuer for the asset
	AssetIssuer string `db:"asset_issuer"`
	// ExpirationLedger is the latest ledger for which this stellar
	// asset contract is active
	ExpirationLedger uint32 `db:"expiration_ledger"`
}

// InsertAssetContracts will insert the given list of rows into the asset_contracts table
func (q *Q) InsertAssetContracts(ctx context.Context, rows []AssetContract) error {
	if len(rows) == 0 {
		return nil
	}
	builder := &db.FastBatchInsertBuilder{}

	for _, row := range rows {
		if err := builder.RowStruct(row); err != nil {
			return errors.Wrap(err, "could not insert asset contract row")
		}
	}

	if err := builder.Exec(ctx, q, "asset_contracts"); err != nil {
		return errors.Wrap(err, "could not exec asset contract insert builder")
	}

	return nil
}

// UpdateAssetContractExpirations will update the expiration ledgers for the given list of keys
// (if they exist in the db).
func (q *Q) UpdateAssetContractExpirations(ctx context.Context, keys []xdr.Hash, expirationLedgers []uint32) error {
	return q.updateExpirations(ctx, "asset_contracts", keys, expirationLedgers)
}

// DeleteAssetContractsExpiringAt deletes all contract asset contract rows which are active
// at `ledger` and expired at `ledger+1`
func (q *Q) DeleteAssetContractsExpiringAt(ctx context.Context, ledger uint32) (int64, error) {
	sql := sq.Delete("asset_contracts").
		Where(map[string]interface{}{"expiration_ledger": ledger})
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// ContractAssetBalance represents a row in the contract_asset_balances table
type ContractAssetBalance struct {
	// KeyHash is a hash of the contract balance's ledger entry key
	KeyHash []byte `db:"key_hash"`
	// ContractID is the contract id of the stellar asset contract
	ContractID []byte `db:"asset_contract_id"`
	// Amount is the amount held by the contract
	Amount string `db:"amount"`
	// ExpirationLedger is the latest ledger for which this contract balance
	// ledger entry is active
	ExpirationLedger uint32 `db:"expiration_ledger"`
}

// InsertContractAssetBalances will insert the given list of rows into the contract_asset_balances table
func (q *Q) InsertContractAssetBalances(ctx context.Context, rows []ContractAssetBalance) error {
	if len(rows) == 0 {
		return nil
	}
	builder := &db.FastBatchInsertBuilder{}

	for _, row := range rows {
		if err := builder.RowStruct(row); err != nil {
			return errors.Wrap(err, "could not insert asset assetStat row")
		}
	}

	if err := builder.Exec(ctx, q, "contract_asset_balances"); err != nil {
		return errors.Wrap(err, "could not exec asset assetStats insert builder")
	}

	return nil
}

const maxUpdateBatchSize = 30000

// UpdateContractAssetBalanceAmounts will update the expiration ledgers for the given list of keys
// (if they exist in the db).
func (q *Q) UpdateContractAssetBalanceAmounts(ctx context.Context, keys []xdr.Hash, amounts []string) error {
	for len(keys) > 0 {
		var args []interface{}
		var values []string

		for i := 0; len(keys) > 0 && i < maxUpdateBatchSize; i++ {
			args = append(args, keys[0][:], amounts[0])
			values = append(values, "(cast(? as bytea), cast(? as numeric))")
			keys = keys[1:]
			amounts = amounts[1:]
		}

		sql := fmt.Sprintf(`
			UPDATE contract_asset_balances
			SET
			  amount = myvalues.amount
			FROM (
			  VALUES
				%s
			) AS myvalues (key_hash, amount)
			WHERE contract_asset_balances.key_hash = myvalues.key_hash`,
			strings.Join(values, ","),
		)

		_, err := q.ExecRaw(ctx, sql, args...)
		if err != nil {
			return err
		}
	}
	return nil
}

// UpdateContractAssetBalanceExpirations will update the expiration ledgers for the given list of keys
// (if they exist in the db).
func (q *Q) UpdateContractAssetBalanceExpirations(ctx context.Context, keys []xdr.Hash, expirationLedgers []uint32) error {
	return q.updateExpirations(ctx, "contract_asset_balances", keys, expirationLedgers)
}

func (q *Q) updateExpirations(ctx context.Context, table string, keys []xdr.Hash, expirationLedgers []uint32) error {
	for len(keys) > 0 {
		var args []interface{}
		var values []string

		for i := 0; len(keys) > 0 && i < maxUpdateBatchSize; i++ {
			args = append(args, keys[0][:], expirationLedgers[0])
			values = append(values, "(cast(? as bytea), cast(? as integer))")
			keys = keys[1:]
			expirationLedgers = expirationLedgers[1:]
		}

		sql := fmt.Sprintf(`
			UPDATE %s 
			SET
			  expiration_ledger = myvalues.expiration
			FROM (
			  VALUES
				%s
			) AS myvalues (key_hash, expiration)
			WHERE %s.key_hash = myvalues.key_hash`,
			table,
			strings.Join(values, ","),
			table,
		)

		_, err := q.ExecRaw(ctx, sql, args...)
		if err != nil {
			return err
		}
	}

	return nil
}

// DeleteContractAssetBalancesExpiringAt deletes and returns all contract asset balances which are active
// at `ledger` and expired at `ledger+1`
func (q *Q) DeleteContractAssetBalancesExpiringAt(ctx context.Context, ledger uint32) ([]ContractAssetBalance, error) {
	sql := sq.Delete("contract_asset_balances").
		Where(map[string]interface{}{"expiration_ledger": ledger}).Suffix("RETURNING *")
	var balances []ContractAssetBalance
	err := q.Select(ctx, &balances, sql)
	return balances, err
}

// GetContractAssetBalances fetches all contract_asset_balances rows for the
// given list of key hashes.
func (q *Q) GetContractAssetBalances(ctx context.Context, keys []xdr.Hash) ([]ContractAssetBalance, error) {
	keyBytes := make([][]byte, len(keys))
	for i := range keys {
		keyBytes[i] = keys[i][:]
	}
	sql := sq.Select("contract_asset_balances.*").From("contract_asset_balances").
		Where(map[string]interface{}{"key_hash": keyBytes})
	var balances []ContractAssetBalance
	err := q.Select(ctx, &balances, sql)
	return balances, err
}

// RemoveContractAssetBalances removes rows from the contract_asset_balances table
func (q *Q) RemoveContractAssetBalances(ctx context.Context, keys []xdr.Hash) error {
	if len(keys) == 0 {
		return nil
	}
	keyBytes := make([][]byte, len(keys))
	for i := range keys {
		keyBytes[i] = keys[i][:]
	}

	_, err := q.Exec(ctx, sq.Delete("contract_asset_balances").
		Where(map[string]interface{}{
			"key_hash": keyBytes,
		}))
	return err
}

// InsertAssetStat a single asset assetStat row into the exp_asset_stats
// Returns number of rows affected and error.
func (q *Q) InsertAssetStat(ctx context.Context, assetStat ExpAssetStat) (int64, error) {
	sql := sq.Insert("exp_asset_stats").SetMap(assetStatToMap(assetStat))
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// InsertContractAssetStat inserts a row into the contract_asset_stats table
func (q *Q) InsertContractAssetStat(ctx context.Context, row ContractAssetStatRow) (int64, error) {
	sql := sq.Insert("contract_asset_stats").SetMap(map[string]interface{}{
		"contract_id": row.ContractID,
		"stat":        row.Stat,
	})
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// UpdateAssetStat updates a row in the exp_asset_stats table.
// Returns number of rows affected and error.
func (q *Q) UpdateAssetStat(ctx context.Context, assetStat ExpAssetStat) (int64, error) {
	sql := sq.Update("exp_asset_stats").
		SetMap(assetStatToMap(assetStat)).
		Where(assetStatToPrimaryKeyMap(assetStat))
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// UpdateContractAssetStat updates a row in the contract_asset_stats table.
// Returns number of rows afected and error.
func (q *Q) UpdateContractAssetStat(ctx context.Context, row ContractAssetStatRow) (int64, error) {
	sql := sq.Update("contract_asset_stats").Set("stat", row.Stat).
		Where("contract_id = ?", row.ContractID)
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// RemoveAssetStat removes a row in the exp_asset_stats table.
func (q *Q) RemoveAssetStat(ctx context.Context, assetType xdr.AssetType, assetCode, assetIssuer string) (int64, error) {
	sql := sq.Delete("exp_asset_stats").
		Where(map[string]interface{}{
			"asset_type":   assetType,
			"asset_code":   assetCode,
			"asset_issuer": assetIssuer,
		})
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// RemoveAssetContractStat removes a row in the contract_asset_stats table.
func (q *Q) RemoveAssetContractStat(ctx context.Context, contractID []byte) (int64, error) {
	sql := sq.Delete("contract_asset_stats").
		Where("contract_id = ?", contractID)
	result, err := q.Exec(ctx, sql)
	if err != nil {
		return 0, err
	}

	return result.RowsAffected()
}

// GetAssetStat returns a row in the exp_asset_stats table.
func (q *Q) GetAssetStat(ctx context.Context, assetType xdr.AssetType, assetCode, assetIssuer string) (ExpAssetStat, error) {
	sql := selectAssetStats.Where(map[string]interface{}{
		"asset_type":   assetType,
		"asset_code":   assetCode,
		"asset_issuer": assetIssuer,
	})
	var assetStat ExpAssetStat
	err := q.Get(ctx, &assetStat, sql)
	return assetStat, err
}

// GetContractAssetStat returns a row in the contract_asset_stats table.
func (q *Q) GetContractAssetStat(ctx context.Context, contractID []byte) (ContractAssetStatRow, error) {
	sql := sq.Select("*").From("contract_asset_stats").Where("contract_id = ?", contractID)
	var assetStat ContractAssetStatRow
	err := q.Get(ctx, &assetStat, sql)
	return assetStat, err
}

func parseAssetStatsCursor(cursor string) (string, string, error) {
	parts := strings.SplitN(cursor, "_", 3)
	if len(parts) != 3 {
		return "", "", fmt.Errorf("invalid asset stats cursor: %v", cursor)
	}

	code, issuer, assetType := parts[0], parts[1], parts[2]
	var issuerAccount xdr.AccountId
	var asset xdr.Asset

	if err := issuerAccount.SetAddress(issuer); err != nil {
		return "", "", errors.Wrap(
			err,
			fmt.Sprintf("invalid issuer in asset stats cursor: %v", cursor),
		)
	}

	if err := asset.SetCredit(code, issuerAccount); err != nil {
		return "", "", errors.Wrap(
			err,
			fmt.Sprintf("invalid asset stats cursor: %v", cursor),
		)
	}

	if _, ok := xdr.StringToAssetType[assetType]; !ok {
		return "", "", errors.Errorf("invalid asset type in asset stats cursor: %v", cursor)
	}

	return code, issuer, nil
}

// GetAssetStats returns a page of exp_asset_stats rows.
func (q *Q) GetAssetStats(ctx context.Context, assetCode, assetIssuer string, page db2.PageQuery) ([]AssetAndContractStat, error) {
	// AssetAndContractStat contains the information listed below which is included in the /assets response:
	//
	// 1. amount of trustlines, liquidity pools, trustlines, and claimable balances which hold an asset.
	// 2. the contract id of the SAC which corresponds to the asset, if it exists and is live.
	// 3. amount of live contract balances which hold an asset.
	//
	// (1) is stored in the exp_asset_stats table and is derived by ingesting trustline, liquidity pool,
	// and claimable balance ledger entries.
	//
	// (2) is stored in the asset_contracts table and is derived by ingesting SAC contracts.
	//
	// (3) is stored in the contract_asset_stats table and is derived by ingesting SAC contract balances.
	//
	var cursorComparison, orderBy string
	switch page.Order {
	case "asc":
		cursorComparison, orderBy = ">", "asc"
	case "desc":
		cursorComparison, orderBy = "<", "desc"
	default:
		return nil, fmt.Errorf("invalid page order %s", page.Order)
	}

	cursorCode, cursorIssuer := "", ""
	if page.Cursor != "" {
		var err error
		cursorCode, cursorIssuer, err = parseAssetStatsCursor(page.Cursor)
		if err != nil {
			return nil, err
		}
	}

	expAssetStatsSQL := sq.Select(
		"exp_asset_stats.asset_type as asset_type",
		"exp_asset_stats.asset_code as asset_code",
		"exp_asset_stats.asset_issuer as asset_issuer",
		"exp_asset_stats.accounts",
		"exp_asset_stats.balances",
		"asset_contracts.contract_id as contract_id",
		"contract_asset_stats.stat as contracts",
	).
		From("exp_asset_stats").
		LeftJoin("asset_contracts ON " +
			"exp_asset_stats.asset_type = asset_contracts.asset_type AND " +
			"exp_asset_stats.asset_code = asset_contracts.asset_code AND " +
			"exp_asset_stats.asset_issuer = asset_contracts.asset_issuer").
		LeftJoin("contract_asset_stats ON asset_contracts.contract_id = contract_asset_stats.contract_id")

	contractOnlySQL := sq.Select(
		"asset_contracts.asset_type as asset_type",
		"asset_contracts.asset_code as asset_code",
		"asset_contracts.asset_issuer as asset_issuer",
		`'{"authorized":0,"authorized_to_maintain_liabilities":0,"claimable_balances":0,"liquidity_pools":0,"unauthorized":0}'::jsonb as accounts`,
		`'{"authorized":"0","authorized_to_maintain_liabilities":"0","claimable_balances":"0","liquidity_pools":"0","unauthorized":"0"}'::jsonb as balances`,
		"asset_contracts.contract_id as contract_id",
		"contract_asset_stats.stat as contracts",
	).
		From("asset_contracts").
		LeftJoin("contract_asset_stats ON asset_contracts.contract_id = contract_asset_stats.contract_id").
		Where(`NOT EXISTS (
			SELECT 1
			FROM exp_asset_stats
			WHERE exp_asset_stats.asset_type = asset_contracts.asset_type
				AND exp_asset_stats.asset_code = asset_contracts.asset_code
				AND exp_asset_stats.asset_issuer = asset_contracts.asset_issuer
		)`)

	if assetCode != "" {
		expAssetStatsSQL = expAssetStatsSQL.Where("exp_asset_stats.asset_code = ?", assetCode)
		contractOnlySQL = contractOnlySQL.Where("asset_contracts.asset_code = ?", assetCode)
	}
	if assetIssuer != "" {
		expAssetStatsSQL = expAssetStatsSQL.Where("exp_asset_stats.asset_issuer = ?", assetIssuer)
		contractOnlySQL = contractOnlySQL.Where("asset_contracts.asset_issuer = ?", assetIssuer)
	}
	if page.Cursor != "" {
		expAssetStatsSQL = expAssetStatsSQL.Where(
			"((exp_asset_stats.asset_code, exp_asset_stats.asset_issuer) "+cursorComparison+" (?,?))",
			cursorCode,
			cursorIssuer,
		)
		contractOnlySQL = contractOnlySQL.Where(
			"((asset_contracts.asset_code, asset_contracts.asset_issuer) "+cursorComparison+" (?,?))",
			cursorCode,
			cursorIssuer,
		)
	}

	expAssetStatsSQL = expAssetStatsSQL.
		OrderBy("(exp_asset_stats.asset_code, exp_asset_stats.asset_issuer) " + orderBy).
		Limit(page.Limit)
	contractOnlySQL = contractOnlySQL.
		OrderBy("(asset_contracts.asset_code, asset_contracts.asset_issuer) " + orderBy).
		Limit(page.Limit)

	expAssetStatsCTE, expAssetStatsArgs, err := expAssetStatsSQL.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "could not build exp_asset_stats select query")
	}
	contractOnlyCTE, contractOnlyArgs, err := contractOnlySQL.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "could not build contract-only select query")
	}

	// fmt.Sprintf is safe here because it only stitches together SQL generated by
	// Squirrel plus the fixed order keyword chosen from "asc"/"desc". User input
	// remains in the bound args produced by ToSql().
	sql := fmt.Sprintf(`
		WITH exp_rows AS (
			%s
		), contract_only_rows AS (
			%s
		)
		SELECT *
		FROM (
			SELECT * FROM exp_rows
			UNION ALL
			SELECT * FROM contract_only_rows
		) merged
		ORDER BY asset_code %s, asset_issuer %s
		LIMIT ?`,
		expAssetStatsCTE,
		contractOnlyCTE,
		orderBy,
		orderBy,
	)

	args := append(expAssetStatsArgs, contractOnlyArgs...)
	args = append(args, page.Limit)

	var results []AssetAndContractStat
	if err := q.SelectRaw(ctx, &results, sql, args...); err != nil {
		return nil, errors.Wrapf(err, "could not run select query: %s", sql)
	}
	if len(results) == 0 {
		return nil, nil
	}

	return results, nil
}

var selectAssetStats = sq.Select("exp_asset_stats.*").From("exp_asset_stats")
