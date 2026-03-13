package actions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/support/render/problem"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/corestate"
	hProblem "github.com/stellar/stellar-horizon/internal/render/problem"
	"github.com/stellar/stellar-horizon/internal/txsub"
)

func TestStellarCoreMalformedTx(t *testing.T) {
	handler := SubmitTransactionHandler{}

	r := httptest.NewRequest("POST", "https://horizon.stellar.org/transactions", nil)
	w := httptest.NewRecorder()
	_, err := handler.GetResource(w, r)
	assert.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, err.(*problem.P).Status)
	assert.Equal(t, "Transaction Malformed", err.(*problem.P).Title)
}

type coreStateGetterMock struct {
	mock.Mock
}

func (m *coreStateGetterMock) GetCoreState() corestate.State {
	a := m.Called()
	return a.Get(0).(corestate.State)
}

type networkSubmitterMock struct {
	mock.Mock
}

func (m *networkSubmitterMock) Submit(ctx context.Context, rawTx string, envelope xdr.TransactionEnvelope, hash string) <-chan txsub.Result {
	a := m.Called()
	return a.Get(0).(chan txsub.Result)
}

func TestStellarCoreNotSynced(t *testing.T) {
	mock := &coreStateGetterMock{}
	mock.On("GetCoreState").Return(corestate.State{
		Synced: false,
	})

	handler := SubmitTransactionHandler{
		NetworkPassphrase: network.PublicNetworkPassphrase,
		CoreStateGetter:   mock,
	}

	form := url.Values{}
	form.Set("tx", "AAAAAAGUcmKO5465JxTSLQOQljwk2SfqAJmZSG6JH6wtqpwhAAABLAAAAAAAAAABAAAAAAAAAAEAAAALaGVsbG8gd29ybGQAAAAAAwAAAAAAAAAAAAAAABbxCy3mLg3hiTqX4VUEEp60pFOrJNxYM1JtxXTwXhY2AAAAAAvrwgAAAAAAAAAAAQAAAAAW8Qst5i4N4Yk6l+FVBBKetKRTqyTcWDNSbcV08F4WNgAAAAAN4Lazj4x61AAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLaqcIQAAAEBKwqWy3TaOxoGnfm9eUjfTRBvPf34dvDA0Nf+B8z4zBob90UXtuCqmQqwMCyH+okOI3c05br3khkH0yP4kCwcE")

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)
	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	assert.Error(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, err.(problem.P).Status)
	assert.Equal(t, "stale_history", err.(problem.P).Type)
	assert.Equal(t, "Historical DB Is Too Stale", err.(problem.P).Title)
}

func TestTimeoutSubmission(t *testing.T) {
	mockSubmitChannel := make(chan txsub.Result)

	mock := &coreStateGetterMock{}
	mock.On("GetCoreState").Return(corestate.State{
		Synced: true,
	})

	mockSubmitter := &networkSubmitterMock{}
	mockSubmitter.On("Submit").Return(mockSubmitChannel)

	handler := SubmitTransactionHandler{
		Submitter:         mockSubmitter,
		NetworkPassphrase: network.PublicNetworkPassphrase,
		CoreStateGetter:   mock,
	}

	form := url.Values{}
	form.Set("tx", "AAAAAAGUcmKO5465JxTSLQOQljwk2SfqAJmZSG6JH6wtqpwhAAABLAAAAAAAAAABAAAAAAAAAAEAAAALaGVsbG8gd29ybGQAAAAAAwAAAAAAAAAAAAAAABbxCy3mLg3hiTqX4VUEEp60pFOrJNxYM1JtxXTwXhY2AAAAAAvrwgAAAAAAAAAAAQAAAAAW8Qst5i4N4Yk6l+FVBBKetKRTqyTcWDNSbcV08F4WNgAAAAAN4Lazj4x61AAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLaqcIQAAAEBKwqWy3TaOxoGnfm9eUjfTRBvPf34dvDA0Nf+B8z4zBob90UXtuCqmQqwMCyH+okOI3c05br3khkH0yP4kCwcE")

	expectedTimeoutResponse := &problem.P{
		Type:   "transaction_submission_timeout",
		Title:  "Transaction Submission Timeout",
		Status: http.StatusGatewayTimeout,
		Detail: "Your transaction submission request has timed out. This does not necessarily mean the submission has failed. " +
			"Before resubmitting, please use the transaction hash provided in `extras.hash` to poll the GET /transactions endpoint for sometime and " +
			"check if it was included in a ledger.",
		Extras: map[string]interface{}{
			"hash":         "3389e9f0f1a65f19736cacf544c2e825313e8447f569233bb8db39aa607c8889",
			"envelope_xdr": "AAAAAAGUcmKO5465JxTSLQOQljwk2SfqAJmZSG6JH6wtqpwhAAABLAAAAAAAAAABAAAAAAAAAAEAAAALaGVsbG8gd29ybGQAAAAAAwAAAAAAAAAAAAAAABbxCy3mLg3hiTqX4VUEEp60pFOrJNxYM1JtxXTwXhY2AAAAAAvrwgAAAAAAAAAAAQAAAAAW8Qst5i4N4Yk6l+FVBBKetKRTqyTcWDNSbcV08F4WNgAAAAAN4Lazj4x61AAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLaqcIQAAAEBKwqWy3TaOxoGnfm9eUjfTRBvPf34dvDA0Nf+B8z4zBob90UXtuCqmQqwMCyH+okOI3c05br3khkH0yP4kCwcE",
		},
	}

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)

	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	ctx, cancel := context.WithTimeout(request.Context(), time.Duration(0))
	defer cancel()
	request = request.WithContext(ctx)

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	assert.Error(t, err)
	assert.Equal(t, expectedTimeoutResponse, err)
}

func TestClientDisconnectSubmission(t *testing.T) {
	mockSubmitChannel := make(chan txsub.Result)

	mock := &coreStateGetterMock{}
	mock.On("GetCoreState").Return(corestate.State{
		Synced: true,
	})

	mockSubmitter := &networkSubmitterMock{}
	mockSubmitter.On("Submit").Return(mockSubmitChannel)

	handler := SubmitTransactionHandler{
		Submitter:         mockSubmitter,
		NetworkPassphrase: network.PublicNetworkPassphrase,
		CoreStateGetter:   mock,
	}

	form := url.Values{}
	form.Set("tx", "AAAAAAGUcmKO5465JxTSLQOQljwk2SfqAJmZSG6JH6wtqpwhAAABLAAAAAAAAAABAAAAAAAAAAEAAAALaGVsbG8gd29ybGQAAAAAAwAAAAAAAAAAAAAAABbxCy3mLg3hiTqX4VUEEp60pFOrJNxYM1JtxXTwXhY2AAAAAAvrwgAAAAAAAAAAAQAAAAAW8Qst5i4N4Yk6l+FVBBKetKRTqyTcWDNSbcV08F4WNgAAAAAN4Lazj4x61AAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLaqcIQAAAEBKwqWy3TaOxoGnfm9eUjfTRBvPf34dvDA0Nf+B8z4zBob90UXtuCqmQqwMCyH+okOI3c05br3khkH0yP4kCwcE")

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)

	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	assert.Equal(t, hProblem.ClientDisconnected, err)
}

func TestDisableTxSubFlagSubmission(t *testing.T) {
	mockSubmitChannel := make(chan txsub.Result)

	mock := &coreStateGetterMock{}
	mock.On("GetCoreState").Return(corestate.State{
		Synced: true,
	})

	mockSubmitter := &networkSubmitterMock{}
	mockSubmitter.On("Submit").Return(mockSubmitChannel)

	handler := SubmitTransactionHandler{
		Submitter:         mockSubmitter,
		NetworkPassphrase: network.PublicNetworkPassphrase,
		DisableTxSub:      true,
		CoreStateGetter:   mock,
	}

	form := url.Values{}

	var p = &problem.P{
		Type:   "transaction_submission_disabled",
		Title:  "Transaction Submission Disabled",
		Status: http.StatusForbidden,
		Detail: "Transaction submission has been disabled for Horizon. " +
			"To enable it again, remove env variable DISABLE_TX_SUB.",
		Extras: map[string]interface{}{},
	}

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)

	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	ctx, cancel := context.WithCancel(request.Context())
	cancel()
	request = request.WithContext(ctx)

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	assert.Equal(t, p, err)
}

// buildOversizedEnvelopeXDR creates a valid XDR TransactionEnvelope that, when
// decoded, exceeds the 1 MB memory limit. It does this by embedding 3122
// void-to-void ScMapEntry entries (each ~336 bytes in Go's representation).
func buildOversizedEnvelopeXDR(t *testing.T) string {
	t.Helper()

	entries := make([]xdr.ScMapEntry, 3122)
	for i := range entries {
		entries[i] = xdr.ScMapEntry{
			Key: xdr.ScVal{Type: xdr.ScValTypeScvVoid},
			Val: xdr.ScVal{Type: xdr.ScValTypeScvVoid},
		}
	}

	scMap := xdr.ScMap(entries)
	scMapPtr := &scMap
	contractHash := xdr.ContractId{}
	contractAddr := xdr.ScAddress{
		Type:       xdr.ScAddressTypeScAddressTypeContract,
		ContractId: &contractHash,
	}
	txB64, err := xdr.MarshalBase64(xdr.TransactionEnvelope{
		Type: xdr.EnvelopeTypeEnvelopeTypeTx,
		V1: &xdr.TransactionV1Envelope{
			Tx: xdr.Transaction{
				SourceAccount: xdr.MuxedAccount{
					Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
					Ed25519: &xdr.Uint256{},
				},
				Operations: []xdr.Operation{
					{
						Body: xdr.OperationBody{
							Type: xdr.OperationTypeInvokeHostFunction,
							InvokeHostFunctionOp: &xdr.InvokeHostFunctionOp{
								HostFunction: xdr.HostFunction{
									Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
									InvokeContract: &xdr.InvokeContractArgs{
										ContractAddress: contractAddr,
										FunctionName:    "f",
										Args: []xdr.ScVal{
											{
												Type: xdr.ScValTypeScvMap,
												Map:  &scMapPtr,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return txB64
}

func TestSubmitTransactionXDRDecodeLimitExceeded(t *testing.T) {
	txB64 := buildOversizedEnvelopeXDR(t)

	handler := SubmitTransactionHandler{
		NetworkPassphrase: network.PublicNetworkPassphrase,
		DecodeOptions:     xdr.DecodeOptions{MaxMemoryBytes: 1024 * 1024},
	}

	form := url.Values{}
	form.Set("tx", txB64)

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)
	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	assert.Error(t, err)
	assert.IsType(t, &problem.P{}, err)
	p := err.(*problem.P)
	assert.Equal(t, "transaction_malformed", p.Type)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestSubmissionSorobanDiagnosticEvents(t *testing.T) {
	mockSubmitChannel := make(chan txsub.Result, 1)
	mock := &coreStateGetterMock{}
	mock.On("GetCoreState").Return(corestate.State{
		Synced: true,
	})

	mockSubmitter := &networkSubmitterMock{}
	mockSubmitter.On("Submit").Return(mockSubmitChannel)
	mockSubmitChannel <- txsub.Result{
		Err: &txsub.FailedTransactionError{
			ResultXDR:           "AAAAAAABCdf////vAAAAAA==",
			DiagnosticEventsXDR: "AAAAAQAAAAAAAAAAAAAAAAAAAAIAAAAAAAAAAgAAAA8AAAAFZXJyb3IAAAAAAAACAAAAAwAAAAUAAAAQAAAAAQAAAAMAAAAOAAAAU3RyYW5zYWN0aW9uIGBzb3JvYmFuRGF0YS5yZXNvdXJjZUZlZWAgaXMgbG93ZXIgdGhhbiB0aGUgYWN0dWFsIFNvcm9iYW4gcmVzb3VyY2UgZmVlAAAAAAUAAAAAAAEJcwAAAAUAAAAAAAG6fA==",
		},
	}

	handler := SubmitTransactionHandler{
		Submitter:         mockSubmitter,
		NetworkPassphrase: network.PublicNetworkPassphrase,
		CoreStateGetter:   mock,
	}

	form := url.Values{}
	form.Set("tx", "AAAAAAGUcmKO5465JxTSLQOQljwk2SfqAJmZSG6JH6wtqpwhAAABLAAAAAAAAAABAAAAAAAAAAEAAAALaGVsbG8gd29ybGQAAAAAAwAAAAAAAAAAAAAAABbxCy3mLg3hiTqX4VUEEp60pFOrJNxYM1JtxXTwXhY2AAAAAAvrwgAAAAAAAAAAAQAAAAAW8Qst5i4N4Yk6l+FVBBKetKRTqyTcWDNSbcV08F4WNgAAAAAN4Lazj4x61AAAAAAAAAAFAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABLaqcIQAAAEBKwqWy3TaOxoGnfm9eUjfTRBvPf34dvDA0Nf+B8z4zBob90UXtuCqmQqwMCyH+okOI3c05br3khkH0yP4kCwcE")

	request, err := http.NewRequest(
		"POST",
		"https://horizon.stellar.org/transactions",
		strings.NewReader(form.Encode()),
	)

	require.NoError(t, err)
	request.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	w := httptest.NewRecorder()
	_, err = handler.GetResource(w, request)
	require.Error(t, err)
	require.IsType(t, &problem.P{}, err)
	require.Contains(t, err.(*problem.P).Extras, "diagnostic_events")
	require.IsType(t, []string{}, err.(*problem.P).Extras["diagnostic_events"])
	diagnosticEvents := err.(*problem.P).Extras["diagnostic_events"].([]string)
	require.Equal(t, diagnosticEvents, []string{"AAAAAAAAAAAAAAAAAAAAAgAAAAAAAAACAAAADwAAAAVlcnJvcgAAAAAAAAIAAAADAAAABQAAABAAAAABAAAAAwAAAA4AAABTdHJhbnNhY3Rpb24gYHNvcm9iYW5EYXRhLnJlc291cmNlRmVlYCBpcyBsb3dlciB0aGFuIHRoZSBhY3R1YWwgU29yb2JhbiByZXNvdXJjZSBmZWUAAAAABQAAAAAAAQlzAAAABQAAAAAAAbp8"})
}
