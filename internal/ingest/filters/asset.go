package filters

import (
	"context"
	"fmt"

	"github.com/stellar/go-stellar-sdk/ingest"
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

// ParseWhitelistAsset parses one asset whitelist entry in the SEP-11 form,
// "CODE:ISSUER" or "native".
func ParseWhitelistAsset(entry string) (xdr.Asset, error) {
	assets, err := xdr.BuildAssets(entry)
	if err != nil {
		return xdr.Asset{}, err
	}
	if len(assets) != 1 {
		return xdr.Asset{}, fmt.Errorf("%q is not a single asset", entry)
	}
	return assets[0], nil
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
		f.canonicalAssetsLookup, f.assetsByContractID = f.lookupsForWhitelist(filterConfig.Whitelist)
		f.lastModified = filterConfig.LastModified
	}

	return nil
}

// lookupsForWhitelist stores each entry in canonical form, so an entry such as
// "NATIVE", saved before the admin API validated entries, still matches. An
// entry that does not parse is kept as it is. It never matches, but the filter
// stays enabled and keeps dropping unmatched transactions, as before.
func (f *assetFilter) lookupsForWhitelist(whitelist []string) (set.Set[string], map[xdr.ContractId]xdr.Asset) {
	canonical := set.NewSet[string](len(whitelist))
	byContractID := make(map[xdr.ContractId]xdr.Asset, len(whitelist))
	for _, entry := range whitelist {
		asset, err := ParseWhitelistAsset(entry)
		if err != nil {
			logger.Warnf("asset filter whitelist entry %q is not a canonical asset, it will never match", entry)
			canonical.Add(entry)
			continue
		}
		canonical.Add(asset.StringCanonical())
		id, err := asset.ContractID(f.networkPassphrase)
		if err != nil {
			logger.Warnf("could not derive contract id for asset filter whitelist entry %q: %v", entry, err)
			continue
		}
		byContractID[xdr.ContractId(id)] = asset
	}
	return canonical, byContractID
}

// FilterTransaction keeps a transaction when a whitelisted asset is named by
// one of its operations or touched by one of its ledger entry changes.
func (f *assetFilter) FilterTransaction(ctx context.Context, transaction ingest.LedgerTransaction) (bool, bool, error) {
	if !f.isEnabled() {
		return false, true, nil
	}

	// Operation bodies come first because they are cheaper to read. They are
	// also the only source for failed transactions, whose meta carries no
	// operation changes.
	if f.matchOperations(transaction) || f.matchMeta(transaction.UnsafeMeta) {
		return true, true, nil
	}

	logger.Debugf("No match, dropped tx with seq %v ", transaction.Envelope.SeqNum())
	return true, false, nil
}

func (f *assetFilter) matchOperations(transaction ingest.LedgerTransaction) bool {
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

// assetsNamedByRevokeSponsorship returns nothing for a pool share trustline.
// The key carries only the pool id, and the revoke does not change the pool
// entry, so such a revoke is not matched.
func assetsNamedByRevokeSponsorship(op xdr.RevokeSponsorshipOp) []xdr.Asset {
	key, ok := op.GetLedgerKey()
	if !ok || key.Type != xdr.LedgerEntryTypeTrustline {
		return nil
	}
	return assetsNamedByTrustLineAsset(key.TrustLine.Asset)
}

// matchMeta reads the ledger entry changes straight from the transaction meta.
// ingest.LedgerTransaction.GetChanges is not used: it rejects meta V0, and it
// sorts every change, which costs time on each transaction the filter drops.
func (f *assetFilter) matchMeta(meta xdr.TransactionMeta) bool {
	for _, changes := range ledgerEntryChangeGroups(meta) {
		if f.matchChanges(changes) {
			return true
		}
	}
	return false
}

// ledgerEntryChangeGroups returns the changes before the operations, the
// changes of each operation, and the changes after the operations. Fee changes
// are not in the meta and are not returned.
func ledgerEntryChangeGroups(meta xdr.TransactionMeta) []xdr.LedgerEntryChanges {
	var groups []xdr.LedgerEntryChanges
	addOperations := func(operations []xdr.OperationMeta) {
		for _, op := range operations {
			groups = append(groups, op.Changes)
		}
	}
	// GetOperations would dereference a nil pointer on an empty V0 meta.
	if meta.V == 0 {
		if meta.Operations != nil {
			addOperations(*meta.Operations)
		}
	} else if v1, ok := meta.GetV1(); ok {
		groups = append(groups, v1.TxChanges)
		addOperations(v1.Operations)
	} else if v2, ok := meta.GetV2(); ok {
		groups = append(groups, v2.TxChangesBefore, v2.TxChangesAfter)
		addOperations(v2.Operations)
	} else if v3, ok := meta.GetV3(); ok {
		groups = append(groups, v3.TxChangesBefore, v3.TxChangesAfter)
		addOperations(v3.Operations)
	} else if v4, ok := meta.GetV4(); ok {
		groups = append(groups, v4.TxChangesBefore, v4.TxChangesAfter)
		for _, op := range v4.Operations {
			groups = append(groups, op.Changes)
		}
	}
	return groups
}

// matchChanges skips removals. A removal carries only the ledger key, and core
// writes the state of the removed entry just before it.
func (f *assetFilter) matchChanges(changes xdr.LedgerEntryChanges) bool {
	for i := range changes {
		entry, ok := changes[i].GetLedgerEntry()
		if ok && f.anyAssetMatchedFilter(f.assetsTouchedByEntry(entry)) {
			return true
		}
	}
	return false
}

// assetsTouchedByEntry lists every asset a ledger entry names. Account entries
// hold only lumens and are not listed.
func (f *assetFilter) assetsTouchedByEntry(entry xdr.LedgerEntry) []xdr.Asset {
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
		return f.assetsTouchedByContractData(entry.Data.MustContractData())
	}
	return nil
}

// assetsTouchedByContractData matches every entry stored under the Stellar
// Asset Contract of a whitelisted asset. Examples are a holder balance, the
// contract instance written on deployment, and an allowance. The contract id
// is a hash of the asset and the network passphrase, so no other contract can
// store entries under it. This also covers the contract for native lumens.
func (f *assetFilter) assetsTouchedByContractData(data xdr.ContractDataEntry) []xdr.Asset {
	contractID := data.Contract.ContractId
	if contractID == nil {
		return nil
	}
	if asset, ok := f.assetsByContractID[*contractID]; ok {
		return []xdr.Asset{asset}
	}
	return nil
}

// assetsNamedByTrustLineAsset skips pool share trustlines. They carry a pool
// id, not an asset. A deposit or a withdrawal also changes the pool entry,
// which names the assets.
func assetsNamedByTrustLineAsset(trustLineAsset xdr.TrustLineAsset) []xdr.Asset {
	if trustLineAsset.Type == xdr.AssetTypeAssetTypePoolShare {
		return nil
	}
	return []xdr.Asset{trustLineAsset.ToAsset()}
}

func (f *assetFilter) anyAssetMatchedFilter(assets []xdr.Asset) bool {
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

func (f *assetFilter) isEnabled() bool {
	// filtering is disabled if the whitelist is empty for now as that is the only filter rule
	return len(f.canonicalAssetsLookup) >= 1 && f.enabled
}
