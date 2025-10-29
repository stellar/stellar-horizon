package filters

import (
	"testing"

	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/test"
)

func TestItGetsFilters(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{tt.HorizonSession()}

	filtersService := NewFilters()

	ingestFilters := filtersService.GetFilters(q, tt.Ctx)

	// should be total of filters implemented in the system
	tt.Assert.Len(ingestFilters, 2)
}
