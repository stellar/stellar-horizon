package history

import (
	"context"

	"github.com/stretchr/testify/mock"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/support/db"
)

type MockTransactionsBatchInsertBuilder struct {
	mock.Mock
}

func (m *MockTransactionsBatchInsertBuilder) Add(transaction ingest.LedgerTransaction, sequence uint32) error {
	a := m.Called(transaction, sequence)
	return a.Error(0)
}

func (m *MockTransactionsBatchInsertBuilder) Exec(ctx context.Context, session db.SessionInterface) error {
	a := m.Called(ctx, session)
	return a.Error(0)
}
