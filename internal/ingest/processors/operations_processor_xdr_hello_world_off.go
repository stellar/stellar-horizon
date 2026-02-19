//go:build !xdr_hello_world

package processors

import "github.com/stellar/go-stellar-sdk/xdr"

func detailsForXdrHelloWorld(_ *transactionOperationWrapper) (map[string]interface{}, error, bool) {
	return nil, nil, false
}

func participantsForXdrHelloWorld(_ *transactionOperationWrapper) ([]xdr.AccountId, error, bool) {
	return nil, nil, false
}
