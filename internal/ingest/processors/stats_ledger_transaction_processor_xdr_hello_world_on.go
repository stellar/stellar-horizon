//go:build xdr_hello_world

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func countOperationTypeForXdrHelloWorld(results *StatsLedgerTransactionProcessorResults, opBody xdr.OperationBody) bool {
	switch opBody.Type {
	case xdr.OperationTypeHelloWorld:
		results.OperationsHelloWorld++
		return true
	default:
		return false
	}
}
