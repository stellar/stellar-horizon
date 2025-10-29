package resourceadapter

import (
	"context"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/services/horizon/internal/ingest"
	"github.com/stellar/go/services/horizon/internal/ledger"
)

func TestPopulateRoot(t *testing.T) {
	res := &horizon.Root{}
	templates := map[string]string{
		"accounts":           "/accounts{?signer,asset_type,asset_issuer,asset_code}",
		"offers":             "/offers",
		"strictReceivePaths": "/paths/strict-receive",
		"strictSendPaths":    "/paths/strict-send",
	}

	PopulateRoot(context.Background(),
		res,
		ledger.Status{
			CoreStatus: ledger.CoreStatus{
				CoreLatest: 1,
			},
			HorizonStatus: ledger.HorizonStatus{
				HistoryLatest: 3, HistoryElder: 2,
			},
		},
		"hVersion",
		"cVersion",
		"passphrase",
		100,
		101,
		urlMustParse(t, "https://friendbot.example.com"),
		templates,
	)

	assert.Equal(t, int32(1), res.CoreSequence)
	assert.Equal(t, int32(2), res.HistoryElderSequence)
	assert.Equal(t, int32(3), res.HorizonSequence)
	assert.Equal(t, "hVersion", res.HorizonVersion)
	assert.Equal(t, "cVersion", res.StellarCoreVersion)
	assert.Equal(t, "passphrase", res.NetworkPassphrase)
	assert.Equal(t, "https://friendbot.example.com/{?addr}", res.Links.Friendbot.Href)
	assert.Equal(t, uint32(ingest.MaxSupportedProtocolVersion), res.SupportedProtocolVersion)

	// Without testbot
	res = &horizon.Root{}
	PopulateRoot(context.Background(),
		res,
		ledger.Status{
			CoreStatus: ledger.CoreStatus{
				CoreLatest: 1,
			},
			HorizonStatus: ledger.HorizonStatus{
				HistoryLatest: 3, HistoryElder: 2,
			},
		},
		"hVersion",
		"cVersion",
		"passphrase",
		100,
		101,
		nil,
		templates,
	)

	assert.Equal(t, int32(1), res.CoreSequence)
	assert.Equal(t, int32(2), res.HistoryElderSequence)
	assert.Equal(t, int32(3), res.HorizonSequence)
	assert.Equal(t, "hVersion", res.HorizonVersion)
	assert.Equal(t, "cVersion", res.StellarCoreVersion)
	assert.Equal(t, "passphrase", res.NetworkPassphrase)
	assert.Empty(t, res.Links.Friendbot)

	res = &horizon.Root{}
	PopulateRoot(context.Background(),
		res,
		ledger.Status{
			CoreStatus: ledger.CoreStatus{
				CoreLatest: 1,
			},
			HorizonStatus: ledger.HorizonStatus{
				HistoryLatest: 3, HistoryElder: 2,
			},
		},
		"hVersion",
		"cVersion",
		"passphrase",
		100,
		101,
		urlMustParse(t, "https://friendbot.example.com"),
		templates,
	)

	assert.Equal(t, templates["accounts"], res.Links.Accounts.Href)
	assert.Equal(t, "/offers/{offer_id}", res.Links.Offer.Href)
	assert.Equal(
		t,
		templates["offers"],
		res.Links.Offers.Href,
	)
	assert.Equal(
		t,
		templates["strictReceivePaths"],
		res.Links.StrictReceivePaths.Href,
	)
	assert.Equal(
		t,
		templates["strictSendPaths"],
		res.Links.StrictSendPaths.Href,
	)
}

func urlMustParse(t *testing.T, s string) *url.URL {
	if u, err := url.Parse(s); err != nil {
		t.Fatalf("Unable to parse URL: %s/%v", s, err)
		return nil
	} else {
		return u
	}
}
