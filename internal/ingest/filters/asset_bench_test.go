package filters

import (
	"context"
	"testing"

	"github.com/stellar/go-stellar-sdk/xdr"
)

// The common case on a filtered node is a dropped transaction, so the benchmark
// measures a credit/credit offer that crosses three offers and matches nothing.
func BenchmarkAssetFilterDropsCrossedOffer(b *testing.B) {
	issuer := testIssuer
	selling := xdr.MustNewCreditAsset("EURC", issuer)
	buying := xdr.MustNewCreditAsset("GBPC", issuer)
	filter := newTestAssetFilter(b, xdr.MustNewCreditAsset("USDC", issuer))

	offer := func(id xdr.Int64, amount xdr.Int64) xdr.LedgerEntry {
		return xdr.LedgerEntry{Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeOffer,
			Offer: &xdr.OfferEntry{
				SellerId: xdr.MustAddress(testHolder),
				OfferId:  id,
				Selling:  buying,
				Buying:   selling,
				Amount:   amount,
				Price:    xdr.Price{N: 1, D: 1},
			},
		}}
	}
	var changes xdr.LedgerEntryChanges
	for id := xdr.Int64(1); id <= 3; id++ {
		changes = append(changes, stateAndUpdated(offer(id, 100), offer(id, 50))...)
		changes = append(changes, stateAndUpdated(holderTrustLineEntry(selling, 100), holderTrustLineEntry(selling, 150))...)
		changes = append(changes, stateAndUpdated(holderTrustLineEntry(buying, 100), holderTrustLineEntry(buying, 50))...)
	}
	tx := successfulTxWithMetaV3(testHolder, []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypeManageSellOffer,
		ManageSellOfferOp: &xdr.ManageSellOfferOp{
			Selling: selling,
			Buying:  buying,
			Amount:  150,
			Price:   xdr.Price{N: 1, D: 1},
		},
	}}}, changes)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, include, err := filter.FilterTransaction(ctx, tx)
		if err != nil || include {
			b.Fatal("expected the transaction to be dropped")
		}
	}
}
