package history

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/lib/pq"
)

const (
	assetFilterRulesTableName   = "asset_filter_rules"
	accountFilterRulesTableName = "account_filter_rules"
	whitelistColumnName         = "whitelist"
	enabledColumnName           = "enabled"
	lastModifiedColumnName      = "last_modified"
)

type AssetFilterConfig struct {
	Enabled      bool           `db:"enabled"`
	Whitelist    pq.StringArray `db:"whitelist"`
	LastModified int64          `db:"last_modified"`
}

type AccountFilterConfig struct {
	Enabled      bool           `db:"enabled"`
	Whitelist    pq.StringArray `db:"whitelist"`
	LastModified int64          `db:"last_modified"`
}

type QFilter interface {
	GetAccountFilterConfig(ctx context.Context) (AccountFilterConfig, error)
	GetAssetFilterConfig(ctx context.Context) (AssetFilterConfig, error)
	UpdateAssetFilterConfig(ctx context.Context, config AssetFilterConfig) (AssetFilterConfig, error)
	UpdateAccountFilterConfig(ctx context.Context, config AccountFilterConfig) (AccountFilterConfig, error)
}

func (q *Q) GetAccountFilterConfig(ctx context.Context) (AccountFilterConfig, error) {
	filterConfig := AccountFilterConfig{}
	sql := sq.Select("*").From(accountFilterRulesTableName)
	err := q.Get(ctx, &filterConfig, sql)

	return filterConfig, err
}

func (q *Q) GetAssetFilterConfig(ctx context.Context) (AssetFilterConfig, error) {
	filterConfig := AssetFilterConfig{}
	sql := sq.Select("*").From(assetFilterRulesTableName)
	err := q.Get(ctx, &filterConfig, sql)

	return filterConfig, err
}

func (q *Q) UpdateAssetFilterConfig(ctx context.Context, config AssetFilterConfig) (AssetFilterConfig, error) {
	updateCols := map[string]interface{}{
		lastModifiedColumnName: sq.Expr(`extract(epoch from now() at time zone 'utc')`),
		enabledColumnName:      config.Enabled,
		whitelistColumnName:    config.Whitelist,
	}

	sqlUpdate := sq.Update(assetFilterRulesTableName).SetMap(updateCols)

	rowCnt, err := q.checkForError(sqlUpdate, ctx)
	if err != nil {
		return AssetFilterConfig{}, err
	}

	if rowCnt < 1 {
		return AssetFilterConfig{}, sql.ErrNoRows
	}
	return q.GetAssetFilterConfig(ctx)
}

func (q *Q) UpdateAccountFilterConfig(ctx context.Context, config AccountFilterConfig) (AccountFilterConfig, error) {
	updateCols := map[string]interface{}{
		lastModifiedColumnName: sq.Expr(`extract(epoch from now() at time zone 'utc')`),
		enabledColumnName:      config.Enabled,
		whitelistColumnName:    config.Whitelist,
	}

	sqlUpdate := sq.Update(accountFilterRulesTableName).SetMap(updateCols)

	rowCnt, err := q.checkForError(sqlUpdate, ctx)
	if err != nil {
		return AccountFilterConfig{}, err
	}

	if rowCnt < 1 {
		return AccountFilterConfig{}, sql.ErrNoRows
	}
	return q.GetAccountFilterConfig(ctx)
}

func (q *Q) checkForError(builder sq.Sqlizer, ctx context.Context) (int64, error) {
	result, err := q.Exec(ctx, builder)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
