package processors

import (
	"context"
	"database/sql"
	"math/big"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/xdr"

	"github.com/stellar/stellar-horizon/internal/db2/history"
)

type AssetStatsProcessor struct {
	assetStatsQ              history.QAssetStats
	currentLedger            uint32
	assetStatSet             AssetStatSet
	contractAssetStatSet     *ContractAssetStatSet
	ingestFromHistoryArchive bool
}

// NewAssetStatsProcessor constructs a new AssetStatsProcessor instance.
func NewAssetStatsProcessor(
	assetStatsQ history.QAssetStats,
	networkPassphrase string,
	ingestFromHistoryArchive bool,
	currentLedger uint32,
) *AssetStatsProcessor {
	p := &AssetStatsProcessor{
		currentLedger:            currentLedger,
		assetStatsQ:              assetStatsQ,
		ingestFromHistoryArchive: ingestFromHistoryArchive,
		assetStatSet:             NewAssetStatSet(),
		contractAssetStatSet:     NewContractAssetStatSet(assetStatsQ, networkPassphrase, currentLedger),
	}
	return p
}

func (p *AssetStatsProcessor) Name() string {
	return "processors.AssetStatsProcessor"
}

func (p *AssetStatsProcessor) ProcessChange(ctx context.Context, change ingest.Change) error {
	if change.Type != xdr.LedgerEntryTypeLiquidityPool &&
		change.Type != xdr.LedgerEntryTypeClaimableBalance &&
		change.Type != xdr.LedgerEntryTypeTrustline &&
		change.Type != xdr.LedgerEntryTypeContractData &&
		change.Type != xdr.LedgerEntryTypeTtl {
		return nil
	}

	var err error
	switch change.Type {
	case xdr.LedgerEntryTypeLiquidityPool:
		err = p.assetStatSet.AddLiquidityPool(change)
	case xdr.LedgerEntryTypeClaimableBalance:
		err = p.assetStatSet.AddClaimableBalance(change)
	case xdr.LedgerEntryTypeTrustline:
		err = p.assetStatSet.AddTrustline(change)
	case xdr.LedgerEntryTypeContractData:
		err = p.contractAssetStatSet.AddContractData(change)
	case xdr.LedgerEntryTypeTtl:
		err = p.contractAssetStatSet.AddTTL(change)
	default:
		return errors.Errorf("Change type %v is unexpected", change.Type)
	}

	return err
}

func (p *AssetStatsProcessor) Commit(ctx context.Context) error {
	if p.ingestFromHistoryArchive {
		// When ingesting from the history archives we can take advantage of the fact
		// that there are only created ledger entries. We don't need to execute any
		// updates or removals on the asset stats tables. And we can also skip
		// deleting expired contract balances.
		assetStatsDeltas := p.assetStatSet.All()
		if len(assetStatsDeltas) > 0 {
			if err := p.assetStatsQ.InsertAssetStats(ctx, assetStatsDeltas); err != nil {
				return errors.Wrap(err, "Error inserting asset stats")
			}
		}

		balances, statRows := p.contractAssetStatSet.GetCreatedBalancesAndStats()
		if len(statRows) > 0 {
			if err := p.assetStatsQ.InsertContractAssetStats(ctx, statRows); err != nil {
				return errors.Wrap(err, "Error inserting asset contract stats")
			}
		}

		if len(balances) > 0 {
			if err := p.assetStatsQ.InsertContractAssetBalances(ctx, balances); err != nil {
				return errors.Wrap(err, "Error inserting contract asset balances")
			}
		}

		rows, err := p.contractAssetStatSet.GetCreatedAssetContracts()
		if err != nil {
			return errors.Wrap(err, "Error getting created asset contracts")
		}
		if len(rows) > 0 {
			if err = p.assetStatsQ.InsertAssetContracts(ctx, rows); err != nil {
				return errors.Wrap(err, "Error inserting asset contracts")
			}
		}
		return nil
	}

	assetStatsDeltas := p.assetStatSet.All()

	if err := p.updateAssetStats(ctx, assetStatsDeltas); err != nil {
		return err
	}

	// Apply the ttl changes first, so that rows whose expiration was extended in this
	// ledger are not reaped below.
	if err := p.updateContractDataExpirations(ctx); err != nil {
		return err
	}

	// Reap the rows which are no longer active before reading the stored rows of the
	// changed balances. An entry archived at ledger E can be restored in E+1, which
	// stellar-core emits as LEDGER_ENTRY_RESTORED (Change.Pre == nil), and its expired
	// row must be removed from the stats once, when it is reaped.
	if err := p.contractAssetStatSet.ingestExpiredBalances(ctx); err != nil {
		return err
	}

	if _, err := p.assetStatsQ.DeleteAssetContractsExpiredBefore(ctx, p.currentLedger); err != nil {
		return errors.Wrap(err, "Error deleting expired asset contracts")
	}

	upserts, removals, err := p.contractAssetStatSet.reconcileBalances(ctx)
	if err != nil {
		return err
	}
	if len(removals) > 0 {
		if err = p.assetStatsQ.RemoveContractAssetBalances(ctx, removals); err != nil {
			return errors.Wrap(err, "Error removing contract asset balances")
		}
	}
	if len(upserts) > 0 {
		if err = p.assetStatsQ.UpsertContractAssetBalances(ctx, upserts); err != nil {
			return errors.Wrap(err, "Error upserting contract asset balances")
		}
	}

	assetContractRows, err := p.contractAssetStatSet.GetCreatedAssetContracts()
	if err != nil {
		return errors.Wrap(err, "Error getting created asset contracts")
	}
	if len(assetContractRows) > 0 {
		if err = p.assetStatsQ.UpsertAssetContracts(ctx, assetContractRows); err != nil {
			return errors.Wrap(err, "Error upserting asset contracts")
		}
	}

	return p.updateContractAssetStats(ctx)
}

func (p *AssetStatsProcessor) updateContractDataExpirations(ctx context.Context) error {
	updatedExpirationEntries := p.contractAssetStatSet.updatedExpirationEntries
	keys := make([]xdr.Hash, 0, len(updatedExpirationEntries))
	expirationLedgers := make([]uint32, 0, len(updatedExpirationEntries))
	for key, update := range updatedExpirationEntries {
		keys = append(keys, key)
		expirationLedgers = append(expirationLedgers, update[1])
	}
	if err := p.assetStatsQ.UpdateContractAssetBalanceExpirations(ctx, keys, expirationLedgers); err != nil {
		return errors.Wrap(err, "Error updating contract asset balance expirations")
	}
	if err := p.assetStatsQ.UpdateAssetContractExpirations(ctx, keys, expirationLedgers); err != nil {
		return errors.Wrap(err, "Error updating asset contract expirations")
	}
	return nil
}

func (p *AssetStatsProcessor) updateAssetStats(
	ctx context.Context,
	assetStatsDeltas []history.ExpAssetStat,
) error {
	for _, delta := range assetStatsDeltas {
		var rowsAffected int64
		var stat history.ExpAssetStat
		var err error

		stat, err = p.assetStatsQ.GetAssetStat(ctx,
			delta.AssetType,
			delta.AssetCode,
			delta.AssetIssuer,
		)
		assetStatNotFound := err == sql.ErrNoRows
		if !assetStatNotFound && err != nil {
			return errors.Wrap(err, "could not fetch asset stat from db")
		}

		if assetStatNotFound {
			// Safety checks
			if delta.Accounts.Authorized < 0 {
				return ingest.NewStateError(errors.Errorf(
					"Authorized accounts negative but DB entry does not exist for asset: %s %s %s",
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				))
			} else if delta.Accounts.AuthorizedToMaintainLiabilities < 0 {
				return ingest.NewStateError(errors.Errorf(
					"AuthorizedToMaintainLiabilities accounts negative but DB entry does not exist for asset: %s %s %s",
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				))
			} else if delta.Accounts.Unauthorized < 0 {
				return ingest.NewStateError(errors.Errorf(
					"Unauthorized accounts negative but DB entry does not exist for asset: %s %s %s",
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				))
			} else if delta.Accounts.ClaimableBalances < 0 {
				return ingest.NewStateError(errors.Errorf(
					"Claimable balance negative but DB entry does not exist for asset: %s %s %s",
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				))
			} else if delta.Accounts.LiquidityPools < 0 {
				return ingest.NewStateError(errors.Errorf(
					"Liquidity pools negative but DB entry does not exist for asset: %s %s %s",
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				))
			}

			// Insert
			var errInsert error
			rowsAffected, errInsert = p.assetStatsQ.InsertAssetStat(ctx, delta)
			if errInsert != nil {
				return errors.Wrap(errInsert, "could not insert asset stat")
			}
		} else {
			var statBalances assetStatBalances
			if err = statBalances.Parse(stat.Balances); err != nil {
				return errors.Wrap(err, "Error parsing balances")
			}

			var deltaBalances assetStatBalances
			if err = deltaBalances.Parse(delta.Balances); err != nil {
				return errors.Wrap(err, "Error parsing balances")
			}

			statBalances = statBalances.Add(deltaBalances)
			statAccounts := stat.Accounts.Add(delta.Accounts)

			// only remove asset stat if the Metadata contract data ledger entry for the token contract
			// has also been removed.
			if statAccounts.IsZero() {
				// Remove stats
				if !statBalances.IsZero() {
					return ingest.NewStateError(errors.Errorf(
						"Removing asset stat by final amount non-zero for: %s %s %s",
						delta.AssetType,
						delta.AssetCode,
						delta.AssetIssuer,
					))
				}
				rowsAffected, err = p.assetStatsQ.RemoveAssetStat(ctx,
					delta.AssetType,
					delta.AssetCode,
					delta.AssetIssuer,
				)
				if err != nil {
					return errors.Wrap(err, "could not remove asset stat")
				}
			} else {
				// Update
				rowsAffected, err = p.assetStatsQ.UpdateAssetStat(ctx, history.ExpAssetStat{
					AssetType:   delta.AssetType,
					AssetCode:   delta.AssetCode,
					AssetIssuer: delta.AssetIssuer,
					Accounts:    statAccounts,
					Balances:    statBalances.ConvertToHistoryObject(),
				})
				if err != nil {
					return errors.Wrap(err, "could not update asset stat")
				}
			}
		}

		if rowsAffected != 1 {
			return ingest.NewStateError(errors.Errorf(
				"%d rows affected (expected exactly 1) when adjusting asset stat for asset: %s %s %s",
				rowsAffected,
				delta.AssetType,
				delta.AssetCode,
				delta.AssetIssuer,
			))
		}
	}
	return nil
}

func (p *AssetStatsProcessor) addContractAssetStat(contractAssetStat assetContractStatValue, row *history.ContractAssetStatRow) error {
	row.Stat.ActiveHolders += contractAssetStat.activeHolders
	activeBalance, ok := new(big.Int).SetString(row.Stat.ActiveBalance, 10)
	if !ok {
		return errors.New("Error parsing: " + row.Stat.ActiveBalance)
	}
	row.Stat.ActiveBalance = activeBalance.Add(activeBalance, contractAssetStat.activeBalance).String()
	return nil
}

func (p *AssetStatsProcessor) updateContractAssetStats(ctx context.Context) error {
	for contractID, contractAssetStat := range p.contractAssetStatSet.contractAssetStats {
		if err := p.updateAssetContractStats(ctx, contractID, contractAssetStat); err != nil {
			return err
		}
	}
	return nil
}

// updateAssetContractStats will look up an asset contract stat by contract id and
// it will adjust the contract balance and holders based on assetContractStat
func (p *AssetStatsProcessor) updateAssetContractStats(
	ctx context.Context,
	contractID xdr.ContractId,
	assetContractStat assetContractStatValue,
) error {
	var rowsAffected int64
	row, err := p.assetStatsQ.GetContractAssetStat(ctx, contractID[:])
	if err == sql.ErrNoRows {
		rowsAffected, err = p.assetStatsQ.InsertContractAssetStat(ctx, assetContractStat.ConvertToHistoryObject())
		if err != nil {
			return errors.Wrap(err, "error inserting asset contract stat")
		}
	} else if err != nil {
		return errors.Wrap(err, "error querying asset by contract id")
	} else {
		if err = p.addContractAssetStat(assetContractStat, &row); err != nil {
			return errors.Wrapf(err, "could not update asset stat with contract id %v with contract delta", contractID)
		}

		if row.Stat == (history.ContractStat{
			ActiveBalance: "0",
			ActiveHolders: 0,
		}) {
			rowsAffected, err = p.assetStatsQ.RemoveAssetContractStat(ctx, contractID[:])
		} else {
			rowsAffected, err = p.assetStatsQ.UpdateContractAssetStat(ctx, row)
		}

		if err != nil {
			return errors.Wrap(err, "could not update asset stat")
		}
	}

	if rowsAffected != 1 {
		// assert that we have updated exactly one row
		return ingest.NewStateError(errors.Errorf(
			"%d rows affected (expected exactly 1) when adjusting asset contract stat for contract: %s",
			rowsAffected,
			contractID,
		))
	}
	return nil
}
