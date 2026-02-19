//go:build !xdr_transaction_meta_v5

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func elideTransactionMetaForXdrTransactionMetaV5(_ *xdr.TransactionMeta) bool {
	return false
}
