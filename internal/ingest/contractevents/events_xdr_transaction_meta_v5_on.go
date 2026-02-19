//go:build xdr_transaction_meta_v5

package contractevents

import (
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/xdr"
)

func parseSacEventFromTxMetaForXdrTransactionMetaV5(tx ingest.LedgerTransaction, event *xdr.ContractEvent, networkPassphrase string) (*StellarAssetContractEvent, error, bool) {
	switch tx.UnsafeMeta.V {
	case 5:
		result, err := parseSacEventFromTxMetaV4(event, networkPassphrase)
		return result, err, true
	default:
		return nil, nil, false
	}
}
