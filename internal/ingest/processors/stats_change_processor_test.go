package processors

import (
	"context"
	"testing"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
)

func TestStatsChangeProcessor(t *testing.T) {
	ctx := context.Background()
	processor := &StatsChangeProcessor{}

	for ledgerEntryType := range xdr.LedgerEntryTypeMap {
		// Created
		assert.NoError(t, processor.ProcessChange(ctx, ingest.Change{
			Type:       xdr.LedgerEntryType(ledgerEntryType),
			Pre:        nil,
			Post:       &xdr.LedgerEntry{},
			ChangeType: xdr.LedgerEntryChangeTypeLedgerEntryCreated,
		}))
		// Updated
		assert.NoError(t, processor.ProcessChange(ctx, ingest.Change{
			Type:       xdr.LedgerEntryType(ledgerEntryType),
			Pre:        &xdr.LedgerEntry{},
			Post:       &xdr.LedgerEntry{},
			ChangeType: xdr.LedgerEntryChangeTypeLedgerEntryUpdated,
		}))
		// Removed
		assert.NoError(t, processor.ProcessChange(ctx, ingest.Change{
			Type:       xdr.LedgerEntryType(ledgerEntryType),
			Pre:        &xdr.LedgerEntry{},
			Post:       nil,
			ChangeType: xdr.LedgerEntryChangeTypeLedgerEntryRemoved,
		}))
		// Restored
		if xdr.LedgerEntryType(ledgerEntryType) == xdr.LedgerEntryTypeContractData ||
			xdr.LedgerEntryType(ledgerEntryType) == xdr.LedgerEntryTypeContractCode ||
			xdr.LedgerEntryType(ledgerEntryType) == xdr.LedgerEntryTypeTtl {
			assert.NoError(t, processor.ProcessChange(ctx, ingest.Change{
				Type:       xdr.LedgerEntryType(ledgerEntryType),
				Pre:        &xdr.LedgerEntry{},
				Post:       &xdr.LedgerEntry{},
				ChangeType: xdr.LedgerEntryChangeTypeLedgerEntryRestored,
			}))
		} else {
			assert.Contains(t, processor.ProcessChange(ctx, ingest.Change{
				Type:       xdr.LedgerEntryType(ledgerEntryType),
				Pre:        &xdr.LedgerEntry{},
				Post:       &xdr.LedgerEntry{},
				ChangeType: xdr.LedgerEntryChangeTypeLedgerEntryRestored,
			}).Error(), "unsupported ledger entry change type")

		}
	}

	processor.ProcessEvictions([]xdr.LedgerKey{{}})

	results := processor.GetResults()

	assert.Equal(t, int64(1), results.AccountsCreated)
	assert.Equal(t, int64(1), results.ClaimableBalancesCreated)
	assert.Equal(t, int64(1), results.DataCreated)
	assert.Equal(t, int64(1), results.OffersCreated)
	assert.Equal(t, int64(1), results.TrustLinesCreated)
	assert.Equal(t, int64(1), results.LiquidityPoolsCreated)
	assert.Equal(t, int64(1), results.ContractDataCreated)
	assert.Equal(t, int64(1), results.ContractCodeCreated)
	assert.Equal(t, int64(1), results.ConfigSettingsCreated)
	assert.Equal(t, int64(1), results.TtlCreated)

	assert.Equal(t, int64(1), results.AccountsUpdated)
	assert.Equal(t, int64(1), results.ClaimableBalancesUpdated)
	assert.Equal(t, int64(1), results.DataUpdated)
	assert.Equal(t, int64(1), results.OffersUpdated)
	assert.Equal(t, int64(1), results.TrustLinesUpdated)
	assert.Equal(t, int64(1), results.LiquidityPoolsUpdated)
	assert.Equal(t, int64(1), results.ContractDataUpdated)
	assert.Equal(t, int64(1), results.ContractCodeUpdated)
	assert.Equal(t, int64(1), results.ConfigSettingsUpdated)
	assert.Equal(t, int64(1), results.TtlUpdated)

	assert.Equal(t, int64(1), results.AccountsRemoved)
	assert.Equal(t, int64(1), results.ClaimableBalancesRemoved)
	assert.Equal(t, int64(1), results.DataRemoved)
	assert.Equal(t, int64(1), results.OffersRemoved)
	assert.Equal(t, int64(1), results.TrustLinesRemoved)
	assert.Equal(t, int64(1), results.LiquidityPoolsRemoved)
	assert.Equal(t, int64(1), results.ContractCodeRemoved)
	assert.Equal(t, int64(1), results.ContractDataRemoved)
	assert.Equal(t, int64(1), results.ConfigSettingsRemoved)
	assert.Equal(t, int64(1), results.TtlRemoved)

	assert.Equal(t, int64(1), results.ContractCodeRestored)
	assert.Equal(t, int64(1), results.ContractDataRestored)
	assert.Equal(t, int64(1), results.TtlRestored)

	assert.Equal(t, int64(1), results.LedgerEntriesEvicted)

	// "+3" for the three entry types (Ttl, Contract Code, and Contract Data) that will have a "restored" change type.
	// "+1" for the ledger entries evicted stat
	assert.Equal(t, len(xdr.LedgerEntryTypeMap)*3+3+1, len(results.Map()))
}
