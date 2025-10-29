package filters

import (
	"context"
	"sync"
	"time"

	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/ingest/processors"
	"github.com/stellar/go/support/log"
)

var (

	// the filter config cache will be checked against latest from db at most once per each of this interval.
	//lint:ignore ST1011, don't need the linter warn on literal assignment
	filterConfigCheckIntervalSeconds     time.Duration = 100
	filterConfigCheckIntervalSecondsLock sync.RWMutex
)

func GetFilterConfigCheckIntervalSeconds() time.Duration {
	filterConfigCheckIntervalSecondsLock.RLock()
	defer filterConfigCheckIntervalSecondsLock.RUnlock()
	return filterConfigCheckIntervalSeconds
}

func SetFilterConfigCheckIntervalSeconds(t time.Duration) {
	filterConfigCheckIntervalSecondsLock.Lock()
	defer filterConfigCheckIntervalSecondsLock.Unlock()
	filterConfigCheckIntervalSeconds = t
}

var (
	LOG = log.WithFields(log.F{
		"filters": "load",
	})
)

type filtersCache struct {
	assetFilter                    AssetFilter
	accountFilter                  AccountFilter
	lastFilterConfigCheckUnixEpoch int64
}

type Filters interface {
	GetFilters(filterQ history.QFilter, ctx context.Context) []processors.LedgerTransactionFilterer
}

func NewFilters() Filters {
	return &filtersCache{
		assetFilter:   NewAssetFilter(),
		accountFilter: NewAccountFilter(),
	}
}

// Provide list of the active filters. Optimize performance by caching the list, only
// rebuild the list on expiration time interval. Method is NOT thread-safe.
func (f *filtersCache) GetFilters(filterQ history.QFilter, ctx context.Context) []processors.LedgerTransactionFilterer {
	// only attempt to refresh filter config cache state at configured interval limit
	if time.Now().Unix() < (f.lastFilterConfigCheckUnixEpoch + int64(GetFilterConfigCheckIntervalSeconds().Seconds())) {
		return f.convertCacheToList()
	}

	f.lastFilterConfigCheckUnixEpoch = time.Now().Unix()

	LOG.Info("expired filter config cache, refresh from db")

	if filterConfig, err := filterQ.GetAssetFilterConfig(ctx); err != nil {
		LOG.Errorf("unable to refresh asset filter config %v", err)
	} else {
		if err := f.assetFilter.RefreshAssetFilter(&filterConfig); err != nil {
			LOG.Errorf("unable to refresh asset filter config %v", err)
		}
	}

	if filterConfig, err := filterQ.GetAccountFilterConfig(ctx); err != nil {
		LOG.Errorf("unable to refresh account filter config %v", err)
	} else {
		if err := f.accountFilter.RefreshAccountFilter(&filterConfig); err != nil {
			LOG.Errorf("unable to refresh account filter config %v", err)
		}
	}

	return f.convertCacheToList()
}

func (f *filtersCache) convertCacheToList() []processors.LedgerTransactionFilterer {
	return []processors.LedgerTransactionFilterer{f.assetFilter, f.accountFilter}
}
