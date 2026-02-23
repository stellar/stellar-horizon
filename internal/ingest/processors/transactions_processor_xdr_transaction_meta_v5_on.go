//go:build xdr_transaction_meta_v5

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func elideTransactionMetaForXdrTransactionMetaV5(meta *xdr.TransactionMeta) bool {
	switch meta.V {
	case 5:
		meta.V5 = &xdr.TransactionMetaV5{
			Ext:              xdr.ExtensionPoint{},
			TxChangesBefore:  xdr.LedgerEntryChanges{},
			Operations:       []xdr.OperationMetaV2{},
			TxChangesAfter:   xdr.LedgerEntryChanges{},
			SorobanMeta:      nil,
			Events:           []xdr.TransactionEvent{},
			DiagnosticEvents: []xdr.DiagnosticEvent{},
		}
		return true
	default:
		return false
	}
}
