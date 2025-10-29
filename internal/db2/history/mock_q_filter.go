package history

import (
	"context"

	"github.com/stretchr/testify/mock"
)

// MockQAccountFilterWhitelist is a mock implementation of the QAccountFilterWhitelist interface
type MockQFilter struct {
	mock.Mock
}

func (m *MockQFilter) GetAccountFilterConfig(ctx context.Context) (AccountFilterConfig, error) {
	a := m.Called(ctx)
	return a.Get(0).(AccountFilterConfig), a.Error(1)
}

func (m *MockQFilter) GetAssetFilterConfig(ctx context.Context) (AssetFilterConfig, error) {
	a := m.Called(ctx)
	return a.Get(0).(AssetFilterConfig), a.Error(1)
}

func (m *MockQFilter) UpdateAccountFilterConfig(ctx context.Context, config AccountFilterConfig) (AccountFilterConfig, error) {
	a := m.Called(ctx, config)
	return a.Get(0).(AccountFilterConfig), a.Error(0)
}

func (m *MockQFilter) UpdateAssetFilterConfig(ctx context.Context, config AssetFilterConfig) (AssetFilterConfig, error) {
	a := m.Called(ctx, config)
	return a.Get(0).(AssetFilterConfig), a.Error(0)
}
