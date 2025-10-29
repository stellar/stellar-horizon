package resourceadapter

import (
	"context"

	protocol "github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/xdr"
)

func PopulateAsset(ctx context.Context, dest *protocol.Asset, asset xdr.Asset) error {
	return asset.Extract(&dest.Type, &dest.Code, &dest.Issuer)
}
