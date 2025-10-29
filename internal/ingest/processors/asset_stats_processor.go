package processors

import (
	"context"
	"database/sql"
	"math/big"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/ingest/sac"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/xdr"
)

type AssetStatsProcessor struct {
	assetStatsQ              history.QAssetStats
	currentLedger            uint32
	assetStatSet             AssetStatSet
	contractDataChanges      []ingest.Change
	removedExpirationEntries map[xdr.Hash]uint32
	createdExpirationEntries map[xdr.Hash]uint32
	updatedExpirationEntries map[xdr.Hash][2]uint32
	ingestFromHistoryArchive bool
	networkPassphrase        string
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
		networkPassphrase:        networkPassphrase,
		assetStatSet:             NewAssetStatSet(),
		contractDataChanges:      []ingest.Change{},
		removedExpirationEntries: map[xdr.Hash]uint32{},
		createdExpirationEntries: map[xdr.Hash]uint32{},
		updatedExpirationEntries: map[xdr.Hash][2]uint32{},
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
		// only ingest contract data entries which could be relevant to
		// asset stats
		ledgerEntry := change.Post
		if ledgerEntry == nil {
			ledgerEntry = change.Pre
		}
		_, assetFound := sac.AssetFromContractData(*ledgerEntry, p.networkPassphrase)
		_, _, balanceFound := sac.ContractBalanceFromContractData(*ledgerEntry, p.networkPassphrase)
		if !assetFound && !balanceFound {
			return nil
		}
		p.contractDataChanges = append(p.contractDataChanges, change)
	case xdr.LedgerEntryTypeTtl:
		err = p.addExpirationChange(change)
	default:
		return errors.Errorf("Change type %v is unexpected", change.Type)
	}

	return err
}

// AssetStatsProcessor requires that the ttl changes it ingests are already compacted
// because the TTL change semantics in CAP-63 (see
// https://github.com/stellar/stellar-protocol/blob/master/core/cap-0063.md#ttl-ledger-change-semantics )
// are encapsulated in the ChangeCompactor
func (p *AssetStatsProcessor) checkTTLChangeIsCompacted(change ingest.Change) error {
	var keyHash xdr.Hash
	if change.Pre != nil {
		keyHash = change.Pre.Data.MustTtl().KeyHash
	} else {
		keyHash = change.Post.Data.MustTtl().KeyHash
	}
	if _, ok := p.removedExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	if _, ok := p.createdExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	if _, ok := p.updatedExpirationEntries[keyHash]; ok {
		return errors.Errorf("ttl change is not compacted Pre: %v Post: %v", change.Pre, change.Post)
	}
	return nil
}

func (p *AssetStatsProcessor) addExpirationChange(change ingest.Change) error {
	if err := p.checkTTLChangeIsCompacted(change); err != nil {
		return err
	}

	switch {
	case change.Pre == nil && change.Post != nil: // created
		post := change.Post.Data.MustTtl()
		p.createdExpirationEntries[post.KeyHash] = uint32(post.LiveUntilLedgerSeq)
	case change.Pre != nil && change.Post == nil: // removed
		pre := change.Pre.Data.MustTtl()
		p.removedExpirationEntries[pre.KeyHash] = uint32(pre.LiveUntilLedgerSeq)
	case change.Pre != nil && change.Post != nil: // updated
		pre := change.Pre.Data.MustTtl()
		post := change.Post.Data.MustTtl()
		// it's unclear if core could emit a ledger entry change where the
		// expiration ledger remains the same
		if pre.LiveUntilLedgerSeq == post.LiveUntilLedgerSeq {
			return nil
		}
		// but we expect that the expiration ledger will never decrease
		if pre.LiveUntilLedgerSeq > post.LiveUntilLedgerSeq {
			return errors.Errorf(
				"unexpected change in expiration ledger Pre: %v Post: %v",
				pre.LiveUntilLedgerSeq,
				post.LiveUntilLedgerSeq,
			)
		}

		// The previous expiration ledger must always be greater than or equal to the current ledger
		// because if the previous expiration ledger is less than the current ledger then it implies
		// the ledger entry was archived. However, an archived ledger entry cannot be updated without
		// first being restored.
		// Alternatively, the TTL can be updated without being restored if it is a temporary ledger
		// entry. However, in that case, we can ignore the ledger entry entirely because SAC
		// ledger entries are always kept in persistent storage.
		if uint32(pre.LiveUntilLedgerSeq) < p.currentLedger {
			return nil
		}
		p.updatedExpirationEntries[pre.KeyHash] = [2]uint32{
			uint32(pre.LiveUntilLedgerSeq),
			uint32(post.LiveUntilLedgerSeq),
		}
	default:
		return errors.Errorf("unexpected change Pre: %v Post: %v", change.Pre, change.Post)
	}

	return nil
}

func (p *AssetStatsProcessor) Commit(ctx context.Context) error {
	contractAssetStatSet := NewContractAssetStatSet(
		p.assetStatsQ,
		p.networkPassphrase,
		p.removedExpirationEntries,
		p.createdExpirationEntries,
		p.updatedExpirationEntries,
		p.currentLedger,
	)
	for _, change := range p.contractDataChanges {
		if err := contractAssetStatSet.AddContractData(change); err != nil {
			return errors.Wrap(err, "Error ingesting contract data")
		}
	}

	return p.updateDB(ctx, contractAssetStatSet)
}

func (p *AssetStatsProcessor) updateDB(
	ctx context.Context,
	contractAssetStatSet *ContractAssetStatSet,
) error {
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

		if rows := contractAssetStatSet.GetContractStats(); len(rows) > 0 {
			if err := p.assetStatsQ.InsertContractAssetStats(ctx, rows); err != nil {
				return errors.Wrap(err, "Error inserting asset contract stats")
			}
		}

		if len(contractAssetStatSet.createdBalances) > 0 {
			if err := p.assetStatsQ.InsertContractAssetBalances(ctx, contractAssetStatSet.createdBalances); err != nil {
				return errors.Wrap(err, "Error inserting asset contract stats")
			}
		}

		rows, err := contractAssetStatSet.GetCreatedAssetContracts()
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

	assetContractRows, err := contractAssetStatSet.GetCreatedAssetContracts()
	if err != nil {
		return errors.Wrap(err, "Error getting created asset contracts")
	}
	if err = p.assetStatsQ.InsertAssetContracts(ctx, assetContractRows); err != nil {
		return errors.Wrap(err, "Error inserting asset contracts")
	}

	if err := p.assetStatsQ.RemoveContractAssetBalances(ctx, contractAssetStatSet.removedBalances); err != nil {
		return errors.Wrap(err, "Error removing contract asset balances")
	}

	if err := p.updateContractAssetBalanceAmounts(ctx, contractAssetStatSet.updatedBalances); err != nil {
		return err
	}

	if err := p.assetStatsQ.InsertContractAssetBalances(ctx, contractAssetStatSet.createdBalances); err != nil {
		return errors.Wrap(err, "Error inserting contract asset balances")
	}

	if err := p.updateContractDataExpirations(ctx); err != nil {
		return err
	}

	if err := contractAssetStatSet.ingestExpiredBalances(ctx); err != nil {
		return err
	}

	if _, err := p.assetStatsQ.DeleteAssetContractsExpiringAt(ctx, p.currentLedger-1); err != nil {
		return errors.Wrap(err, "Error fetching contract asset balances")
	}

	return p.updateContractAssetStats(ctx, contractAssetStatSet.contractAssetStats)
}

func (p *AssetStatsProcessor) updateContractAssetBalanceAmounts(ctx context.Context, updatedBalances map[xdr.Hash]*big.Int) error {
	keys := make([]xdr.Hash, 0, len(updatedBalances))
	amounts := make([]string, 0, len(updatedBalances))
	for key, amount := range updatedBalances {
		keys = append(keys, key)
		amounts = append(amounts, amount.String())
	}
	if err := p.assetStatsQ.UpdateContractAssetBalanceAmounts(ctx, keys, amounts); err != nil {
		return errors.Wrap(err, "Error updating contract asset balance amounts")
	}
	return nil
}

func (p *AssetStatsProcessor) updateContractDataExpirations(ctx context.Context) error {
	keys := make([]xdr.Hash, 0, len(p.updatedExpirationEntries))
	expirationLedgers := make([]uint32, 0, len(p.updatedExpirationEntries))
	for key, update := range p.updatedExpirationEntries {
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

func (p *AssetStatsProcessor) updateContractAssetStats(
	ctx context.Context,
	contractAssetStats map[xdr.ContractId]assetContractStatValue,
) error {
	for contractID, contractAssetStat := range contractAssetStats {
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
