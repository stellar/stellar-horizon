//go:build !xdr_transaction_meta_v5

package contractevents

import (
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func parseSacEventFromTxMetaForXdrTransactionMetaV5(_ ingest.LedgerTransaction, _ *xdr.ContractEvent, _ string) (*StellarAssetContractEvent, error, bool) {
	return nil, nil, false
}
