//go:build !xdr_hello_world

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func countOperationTypeForXdrHelloWorld(_ *StatsLedgerTransactionProcessorResults, _ xdr.OperationBody) bool {
	return false
}
