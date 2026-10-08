package resourceadapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/guregu/null"
	"github.com/stellar/go-stellar-sdk/protocols/horizon/effects"
	"github.com/stellar/go-stellar-sdk/support/render/hal"
	"github.com/stellar/go-stellar-sdk/support/test"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEffectAllEffectsCovered(t *testing.T) {
	for typ, s := range EffectTypeNames {
		if typ == history.EffectAccountRemoved || typ == history.EffectAccountInflationDestinationUpdated {
			// these effects use the base representation
			continue
		}
		e := history.Effect{
			Type: typ,
		}
		result, err := NewEffect(context.TODO(), e, history.Ledger{})
		assert.NoError(t, err, s)
		// it shouldn't be a base type
		_, ok := result.(effects.Base)
		assert.False(t, ok, s)
	}

	// verify that the check works for an unknown effect
	e := history.Effect{
		Type: 20000,
	}
	result, err := NewEffect(context.TODO(), e, history.Ledger{})
	assert.NoError(t, err)
	_, ok := result.(effects.Base)
	assert.True(t, ok)
}

func TestEffectTypeNamesAreConsistentWithAdapterTypeNames(t *testing.T) {
	for typ, s := range EffectTypeNames {
		s2, ok := effects.EffectTypeNames[effects.EffectType(typ)]
		require.True(t, ok, s)
		require.Equal(t, s, s2)
	}
	for typ, s := range effects.EffectTypeNames {
		s2, ok := EffectTypeNames[history.EffectType(typ)]
		require.True(t, ok, s)
		require.Equal(t, s, s2)
	}
}

func TestNewEffect_EffectTrustlineAuthorizedToMaintainLiabilities(t *testing.T) {
	tt := assert.New(t)
	ctx, _ := test.ContextWithLogBuffer()

	details := `{
		"asset_code":   "COP",
		"asset_issuer": "GDRW375MAYR46ODGF2WGANQC2RRZL7O246DYHHCGWTV2RE7IHE2QUQLD",
		"asset_type":   "credit_alphanum4",
		"trustor":      "GDQNY3PBOJOKYZSRMK2S7LHHGWZIUISD4QORETLMXEWXBI7KFZZMKTL3"
	}`

	hEffect := history.Effect{
		Account:            "GDQNY3PBOJOKYZSRMK2S7LHHGWZIUISD4QORETLMXEWXBI7KFZZMKTL3",
		HistoryOperationID: 1,
		Order:              1,
		Type:               history.EffectTrustlineAuthorizedToMaintainLiabilities,
		DetailsString:      null.StringFrom(details),
	}
	resource, err := NewEffect(ctx, hEffect, history.Ledger{})
	tt.NoError(err)

	var resourcePage hal.Page
	resourcePage.Add(resource)

	effect, ok := resource.(effects.TrustlineAuthorizedToMaintainLiabilities)
	tt.True(ok)
	tt.Equal("trustline_authorized_to_maintain_liabilities", effect.Type)

	binary, err := json.Marshal(resourcePage)
	tt.NoError(err)

	var page effects.EffectsPage
	tt.NoError(json.Unmarshal(binary, &page))
	tt.Len(page.Embedded.Records, 1)
	tt.Equal(effect, page.Embedded.Records[0].(effects.TrustlineAuthorizedToMaintainLiabilities))
}

func TestNewEffect_EffectTrade_Muxed(t *testing.T) {
	tt := assert.New(t)
	ctx, _ := test.ContextWithLogBuffer()

	details := `{
	"seller": "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY",
	"seller_muxed": "MAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSAAAAAAAAAAE2LP26",
    "seller_muxed_id": 1234
	}`

	hEffect := history.Effect{
		Account:            "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY",
		AccountMuxed:       null.StringFrom("MAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSAAAAAAAAAAE2LP26"),
		HistoryOperationID: 1,
		Order:              1,
		Type:               history.EffectTrade,
		DetailsString:      null.StringFrom(details),
	}
	resource, err := NewEffect(ctx, hEffect, history.Ledger{})
	tt.NoError(err)

	var resourcePage hal.Page
	resourcePage.Add(resource)

	effect, ok := resource.(effects.Trade)
	tt.True(ok)
	tt.Equal("trade", effect.Type)

	binary, err := json.Marshal(resourcePage)
	tt.NoError(err)

	var page effects.EffectsPage
	tt.NoError(json.Unmarshal(binary, &page))
	tt.Len(page.Embedded.Records, 1)
	tt.Equal(effect, page.Embedded.Records[0].(effects.Trade))
	tt.Equal("GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY", effect.Account)
	tt.Equal("MAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSAAAAAAAAAAE2LP26", effect.AccountMuxed)
	tt.Equal(uint64(1234), effect.AccountMuxedID)
	tt.Equal("GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY", effect.Seller)
	tt.Equal("MAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSAAAAAAAAAAE2LP26", effect.SellerMuxed)
	tt.Equal(uint64(1234), effect.SellerMuxedID)
}

func TestNewEffect_EffectContractCredited_MuxedContract(t *testing.T) {
	tt := assert.New(t)
	ctx, _ := test.ContextWithLogBuffer()

	// The details the effects processor stores for a SAC transfer to a
	// CAP-0084 muxed contract. contract_muxed_id is a string so that ids above
	// 2^53 survive JSON.
	details := `{
		"amount":            "40.0000000",
		"asset_type":        "native",
		"contract":          "CAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWA3Y",
		"contract_muxed":    "WAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWDNU3JPX55ASWEDNS",
		"contract_muxed_id": "987654321987654321"
	}`

	hEffect := history.Effect{
		Account:            "GCE4HENKZ3ZIHQY4VEYCVX5ZE5LNDIN3FH4MHZCWFKXQZGQIOGAO77CN",
		HistoryOperationID: 1,
		Order:              2,
		Type:               history.EffectContractCredited,
		DetailsString:      null.StringFrom(details),
	}
	resource, err := NewEffect(ctx, hEffect, history.Ledger{})
	tt.NoError(err)

	effect, ok := resource.(effects.ContractCredited)
	tt.True(ok)
	tt.Equal("contract_credited", effect.Base.Type)
	tt.Equal("CAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWA3Y", effect.Contract)
	tt.Equal("WAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWDNU3JPX55ASWEDNS", effect.ContractMuxed)
	tt.Equal(uint64(987654321987654321), effect.ContractMuxedID)
	// account_muxed belongs to the operation source account, not the destination.
	tt.Empty(effect.AccountMuxed)

	var resourcePage hal.Page
	resourcePage.Add(resource)
	binary, err := json.Marshal(resourcePage)
	tt.NoError(err)
	tt.Contains(string(binary), `"contract_muxed_id":"987654321987654321"`)

	var page effects.EffectsPage
	tt.NoError(json.Unmarshal(binary, &page))
	tt.Len(page.Embedded.Records, 1)
	tt.Equal(effect, page.Embedded.Records[0].(effects.ContractCredited))
}
