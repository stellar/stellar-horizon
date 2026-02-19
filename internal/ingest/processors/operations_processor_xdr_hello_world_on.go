//go:build xdr_hello_world

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func detailsForXdrHelloWorld(operation *transactionOperationWrapper) (map[string]interface{}, error, bool) {
	switch operation.OperationType() {
	case xdr.OperationTypeHelloWorld:
		return map[string]interface{}{}, nil, true
	default:
		return nil, nil, false
	}
}

func participantsForXdrHelloWorld(operation *transactionOperationWrapper) ([]xdr.AccountId, error, bool) {
	switch operation.OperationType() {
	case xdr.OperationTypeHelloWorld:
		// the only direct participant is the source_account, already added
		return nil, nil, true
	default:
		return nil, nil, false
	}
}
