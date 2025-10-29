package paths

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRateLimitedFinder(t *testing.T) {
	for _, limit := range []int{0, 1, 5} {
		t.Run("Limit of "+strconv.Itoa(limit), func(t *testing.T) {
			totalCalls := limit + 4
			errorChan := make(chan error, totalCalls)
			find := func(finder Finder) {
				_, _, err := finder.Find(context.Background(), Query{}, 1)
				errorChan <- err
			}
			findFixedPaths := func(finder Finder) {
				_, _, err := finder.FindFixedPaths(
					context.Background(),
					xdr.MustNewNativeAsset(),
					10,
					nil,
					0,
				)
				errorChan <- err
			}

			wg := &sync.WaitGroup{}
			mockFinder := &MockFinder{}
			mockFinder.On("Find", mock.Anything, mock.Anything, mock.Anything).
				Return([]Path{}, uint32(0), nil).Maybe().Times(limit).
				Run(func(args mock.Arguments) {
					wg.Done()
					wg.Wait()
				})
			mockFinder.On("FindFixedPaths", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
				Return([]Path{}, uint32(0), nil).Maybe().Times(limit).
				Run(func(args mock.Arguments) {
					wg.Done()
					wg.Wait()
				})

			for _, f := range []func(Finder){find, findFixedPaths} {
				wg.Add(totalCalls)
				rateLimitedFinder := NewRateLimitedFinder(mockFinder, uint(limit))
				assert.Equal(t, limit, rateLimitedFinder.Limit())
				for i := 0; i < totalCalls; i++ {
					go f(rateLimitedFinder)
				}

				requestsExceedingLimit := totalCalls - limit
				for i := 0; i < requestsExceedingLimit; i++ {
					err := <-errorChan
					assert.Equal(t, ErrRateLimitExceeded, err)
				}

				wg.Add(-requestsExceedingLimit)
				for i := 0; i < limit; i++ {
					assert.NoError(t, <-errorChan)
				}
			}
			mockFinder.AssertExpectations(t)
		})
	}
}
