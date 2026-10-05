package filters

import (
	"context"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/support/collections/set"
	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/ingest/processors"
)

var (
	logger = log.WithFields(log.F{
		"ingest filter": "asset",
	})
)

type assetFilter struct {
	networkPassphrase     string
	canonicalAssetsLookup set.Set[string]
	// Whitelisted assets keyed by their Stellar Asset Contract id. Rebuilt in
	// RefreshAssetFilter so FilterTransaction never hashes per transaction.
	assetsByContractID map[xdr.ContractId]xdr.Asset
	lastModified       int64
	enabled            bool
}

type AssetFilter interface {
	processors.LedgerTransactionFilterer
	RefreshAssetFilter(filterConfig *history.AssetFilterConfig) error
}

func NewAssetFilter(networkPassphrase string) AssetFilter {
	return &assetFilter{
		networkPassphrase:     networkPassphrase,
		canonicalAssetsLookup: set.Set[string]{},
		assetsByContractID:    map[xdr.ContractId]xdr.Asset{},
	}
}

func (f *assetFilter) Name() string {
	return "filters.assetFilter"
}

func (f *assetFilter) RefreshAssetFilter(filterConfig *history.AssetFilterConfig) error {
	// only need to re-initialize the filter config state(rules) if it's cached version(in  memory)
	// is older than the incoming config version based on lastModified epoch timestamp
	if filterConfig.LastModified > f.lastModified {
		logger.Infof("New Asset Filter config detected, reloading new config %v ", *filterConfig)
		f.enabled = filterConfig.Enabled
		f.canonicalAssetsLookup = listToSet(filterConfig.Whitelist)
		f.assetsByContractID = f.assetsByContractIDForWhitelist(filterConfig.Whitelist)
		f.lastModified = filterConfig.LastModified
	}

	return nil
}

func (f *assetFilter) assetsByContractIDForWhitelist(whitelist []string) map[xdr.ContractId]xdr.Asset {
	byContractID := make(map[xdr.ContractId]xdr.Asset, len(whitelist))
	for _, entry := range whitelist {
		assets, err := xdr.BuildAssets(entry)
		if err != nil || len(assets) != 1 {
			logger.Warnf("asset filter whitelist entry %q is not a canonical asset, it will never match", entry)
			continue
		}
		id, err := assets[0].ContractID(f.networkPassphrase)
		if err != nil {
			logger.Warnf("could not derive contract id for asset filter whitelist entry %q: %v", entry, err)
			continue
		}
		byContractID[xdr.ContractId(id)] = assets[0]
	}
	return byContractID
}

// FilterTransaction keeps a transaction when a whitelisted asset is named by
// one of its operations or touched by one of its ledger entry changes.
func (f *assetFilter) FilterTransaction(ctx context.Context, transaction ingest.LedgerTransaction) (bool, bool, error) {
	if !f.isEnabled() {
		return false, true, nil
	}

	// Operation bodies come first. They are the only source available for
	// failed transactions, whose meta carries no operation changes.
	if f.matchOperations(transaction) {
		return true, true, nil
	}

	// The SDK cannot read changes out of TransactionMeta V0, so the operation
	// bodies are the only source for those transactions.
	if transaction.UnsafeMeta.V == 0 {
		logger.Debugf("No match, dropped tx with seq %v ", transaction.Envelope.SeqNum())
		return true, false, nil
	}

	changes, err := transaction.GetChanges()
	if err != nil {
		return true, false, err
	}
	if f.matchChanges(changes) {
		return true, true, nil
	}

	logger.Debugf("No match, dropped tx with seq %v ", transaction.Envelope.SeqNum())
	return true, false, nil
}

func (f assetFilter) matchOperations(transaction ingest.LedgerTransaction) bool {
	for _, operation := range transaction.Envelope.Operations() {
		if f.anyAssetMatchedFilter(assetsNamedByOperation(transaction, operation)) {
			return true
		}
	}
	return false
}

// assetsNamedByOperation lists every asset an operation body names.
func assetsNamedByOperation(transaction ingest.LedgerTransaction, operation xdr.Operation) []xdr.Asset {
	body := operation.Body
	switch body.Type {
	case xdr.OperationTypeChangeTrust:
		return assetsNamedByChangeTrust(*body.ChangeTrustOp)
	case xdr.OperationTypeManageSellOffer:
		return []xdr.Asset{body.ManageSellOfferOp.Buying, body.ManageSellOfferOp.Selling}
	case xdr.OperationTypeManageBuyOffer:
		return []xdr.Asset{body.ManageBuyOfferOp.Buying, body.ManageBuyOfferOp.Selling}
	case xdr.OperationTypeCreatePassiveSellOffer:
		return []xdr.Asset{body.CreatePassiveSellOfferOp.Buying, body.CreatePassiveSellOfferOp.Selling}
	case xdr.OperationTypeCreateClaimableBalance:
		return []xdr.Asset{body.CreateClaimableBalanceOp.Asset}
	case xdr.OperationTypeClawback:
		return []xdr.Asset{body.ClawbackOp.Asset}
	case xdr.OperationTypePayment:
		return []xdr.Asset{body.PaymentOp.Asset}
	case xdr.OperationTypePathPaymentStrictReceive:
		op := body.PathPaymentStrictReceiveOp
		return append([]xdr.Asset{op.SendAsset, op.DestAsset}, op.Path...)
	case xdr.OperationTypePathPaymentStrictSend:
		op := body.PathPaymentStrictSendOp
		return append([]xdr.Asset{op.SendAsset, op.DestAsset}, op.Path...)
	case xdr.OperationTypeSetTrustLineFlags:
		return []xdr.Asset{body.SetTrustLineFlagsOp.Asset}
	case xdr.OperationTypeAllowTrust:
		return []xdr.Asset{assetNamedByAllowTrust(transaction, operation)}
	case xdr.OperationTypeRevokeSponsorship:
		return assetsNamedByRevokeSponsorship(*body.RevokeSponsorshipOp)
	}
	return nil
}

func assetsNamedByChangeTrust(op xdr.ChangeTrustOp) []xdr.Asset {
	if pool, ok := op.Line.GetLiquidityPool(); ok {
		return []xdr.Asset{pool.ConstantProduct.AssetA, pool.ConstantProduct.AssetB}
	}
	return []xdr.Asset{op.Line.ToAsset()}
}

// assetNamedByAllowTrust rebuilds the asset from the code in the operation.
// The issuer is the operation source, or the transaction source when the
// operation has none.
func assetNamedByAllowTrust(transaction ingest.LedgerTransaction, operation xdr.Operation) xdr.Asset {
	issuer := transaction.Envelope.SourceAccount()
	if operation.SourceAccount != nil {
		issuer = *operation.SourceAccount
	}
	return operation.Body.AllowTrustOp.Asset.ToAsset(issuer.ToAccountId())
}

func assetsNamedByRevokeSponsorship(op xdr.RevokeSponsorshipOp) []xdr.Asset {
	key, ok := op.GetLedgerKey()
	if !ok || key.Type != xdr.LedgerEntryTypeTrustline {
		return nil
	}
	return assetsNamedByTrustLineAsset(key.TrustLine.Asset)
}

func (f assetFilter) matchChanges(changes []ingest.Change) bool {
	for _, change := range changes {
		entry := change.Post
		if entry == nil {
			entry = change.Pre
		}
		if entry != nil && f.anyAssetMatchedFilter(f.assetsTouchedByEntry(*entry)) {
			return true
		}
	}
	return false
}

// assetsTouchedByEntry lists every asset a ledger entry names. Account entries
// hold only lumens and are not listed.
func (f assetFilter) assetsTouchedByEntry(entry xdr.LedgerEntry) []xdr.Asset {
	switch entry.Data.Type {
	case xdr.LedgerEntryTypeTrustline:
		return assetsNamedByTrustLineAsset(entry.Data.MustTrustLine().Asset)
	case xdr.LedgerEntryTypeOffer:
		offer := entry.Data.MustOffer()
		return []xdr.Asset{offer.Selling, offer.Buying}
	case xdr.LedgerEntryTypeClaimableBalance:
		return []xdr.Asset{entry.Data.MustClaimableBalance().Asset}
	case xdr.LedgerEntryTypeLiquidityPool:
		if pool, ok := entry.Data.MustLiquidityPool().Body.GetConstantProduct(); ok {
			return []xdr.Asset{pool.Params.AssetA, pool.Params.AssetB}
		}
	case xdr.LedgerEntryTypeContractData:
		return f.assetsTouchedByContractData(entry)
	}
	return nil
}

// assetsTouchedByContractData recognizes the two entries the Stellar Asset
// Contract writes: the per-holder balance entry and the per-asset metadata
// entry. Both are stored under the asset's contract id, so a balance entry is
// matched by contract id and a metadata entry by the asset it describes.
func (f assetFilter) assetsTouchedByContractData(entry xdr.LedgerEntry) []xdr.Asset {
	if _, _, ok := sac.ContractBalanceFromContractData(entry, f.networkPassphrase); ok {
		contractID := entry.Data.MustContractData().Contract.ContractId
		if contractID == nil {
			return nil
		}
		if asset, ok := f.assetsByContractID[*contractID]; ok {
			return []xdr.Asset{asset}
		}
		return nil
	}
	if asset, ok := sac.AssetFromContractData(entry, f.networkPassphrase); ok {
		return []xdr.Asset{asset}
	}
	return nil
}

// assetsNamedByTrustLineAsset skips pool share trustlines. They carry a pool
// id, not an asset, and the pool entry in the same transaction names the assets.
func assetsNamedByTrustLineAsset(trustLineAsset xdr.TrustLineAsset) []xdr.Asset {
	if trustLineAsset.Type == xdr.AssetTypeAssetTypePoolShare {
		return nil
	}
	return []xdr.Asset{trustLineAsset.ToAsset()}
}

func (f assetFilter) anyAssetMatchedFilter(assets []xdr.Asset) bool {
	for i := range assets {
		if f.assetMatchedFilter(&assets[i]) {
			return true
		}
	}
	return false
}

func (f *assetFilter) assetMatchedFilter(asset *xdr.Asset) bool {
	return f.canonicalAssetsLookup.Contains(asset.StringCanonical())
}

func listToSet(list []string) set.Set[string] {
	set := set.NewSet[string](len(list))
	for i := 0; i < len(list); i++ {
		set.Add(list[i])
	}
	return set
}

func (f assetFilter) isEnabled() bool {
	// filtering is disabled if the whitelist is empty for now as that is the only filter rule
	return len(f.canonicalAssetsLookup) >= 1 && f.enabled
}
