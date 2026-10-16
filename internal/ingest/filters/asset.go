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
	networkPassphrase string
	whitelistedAssets map[assetKey]struct{}
	// Stellar Asset Contract ids of the whitelisted assets. Rebuilt in
	// RefreshAssetFilter so FilterTransaction never hashes per transaction.
	contractIDs set.Set[xdr.ContractId]
	// Number of entries in the stored whitelist, including entries that do not
	// parse. The filter is enabled only when the stored whitelist is not empty.
	whitelistLen int
	lastModified int64
	enabled      bool
}

// assetKey identifies an asset with plain values, so it can be a map key.
// xdr.Asset holds pointers and cannot be compared. Building an assetKey
// copies a few bytes. Building the canonical string, as
// xdr.Asset.StringCanonical does, encodes the issuer to strkey and allocates.
// The filter builds a key for every asset in every transaction, so the key
// keeps that cost low.
//
// The key matches the canonical string rules:
//   - A native asset is the zero key. A credit asset always has an issuer.
//   - The code is padded with zero bytes to 12 bytes. So "USDC" stored as a
//     4-byte code and "USDC" stored as a 12-byte code give the same key, the
//     same way both give the string "USDC:<issuer>".
type assetKey struct {
	code   [12]byte
	issuer xdr.Uint256
}

func newAssetKey(asset xdr.Asset) assetKey {
	return alphaNumKey(asset.Type, asset.AlphaNum4, asset.AlphaNum12)
}

// alphaNumKey builds the key from the pointers the XDR unions already hold, so
// xdr.Asset, xdr.TrustLineAsset and xdr.ChangeTrustAsset share one builder
// and no value is copied to the heap.
func alphaNumKey(assetType xdr.AssetType, alphaNum4 *xdr.AlphaNum4, alphaNum12 *xdr.AlphaNum12) assetKey {
	var key assetKey
	switch assetType {
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		copy(key.code[:], alphaNum4.AssetCode[:])
		key.issuer = *alphaNum4.Issuer.Ed25519
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		copy(key.code[:], alphaNum12.AssetCode[:])
		key.issuer = *alphaNum12.Issuer.Ed25519
	}
	return key
}

type AssetFilter interface {
	processors.LedgerTransactionFilterer
	RefreshAssetFilter(filterConfig *history.AssetFilterConfig) error
}

func NewAssetFilter(networkPassphrase string) AssetFilter {
	return &assetFilter{
		networkPassphrase: networkPassphrase,
		whitelistedAssets: map[assetKey]struct{}{},
		contractIDs:       set.NewSet[xdr.ContractId](0),
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
		f.whitelistedAssets, f.contractIDs = f.lookupsForWhitelist(filterConfig.Whitelist)
		f.whitelistLen = len(filterConfig.Whitelist)
		f.lastModified = filterConfig.LastModified
	}

	return nil
}

// lookupsForWhitelist parses each entry, so an entry such as "NATIVE", saved
// before the admin API validated entries, still matches. An entry that does
// not parse never matches. It still counts in whitelistLen, so the filter
// stays enabled and keeps dropping unmatched transactions, as before.
func (f *assetFilter) lookupsForWhitelist(whitelist []string) (map[assetKey]struct{}, set.Set[xdr.ContractId]) {
	assets := make(map[assetKey]struct{}, len(whitelist))
	contractIDs := set.NewSet[xdr.ContractId](len(whitelist))
	for _, entry := range whitelist {
		asset, err := ParseWhitelistAsset(entry)
		if err != nil {
			logger.Warnf("asset filter whitelist entry %q is not a canonical asset, it will never match", entry)
			continue
		}
		assets[newAssetKey(asset)] = struct{}{}
		id, err := asset.ContractID(f.networkPassphrase)
		if err != nil {
			logger.Warnf("could not derive contract id for asset filter whitelist entry %q: %v", entry, err)
			continue
		}
		contractIDs.Add(xdr.ContractId(id))
	}
	return assets, contractIDs
}

// FilterTransaction keeps a transaction when a whitelisted asset is named by
// its envelope or touched by one of its ledger entry changes.
func (f *assetFilter) FilterTransaction(ctx context.Context, transaction ingest.LedgerTransaction) (bool, bool, error) {
	if !f.isEnabled() {
		return false, true, nil
	}

	// The envelope comes first because it is cheaper to read. It is also the
	// only source for a failed transaction, whose meta carries no operation
	// changes.
	if f.matchEnvelope(transaction) {
		return true, true, nil
	}
	matched, err := f.matchMeta(transaction.UnsafeMeta)
	if err != nil {
		return true, false, err
	}
	if matched {
		return true, true, nil
	}

	logger.Debugf("No match, dropped tx with seq %v ", transaction.Envelope.SeqNum())
	return true, false, nil
}

func (f *assetFilter) matchEnvelope(transaction ingest.LedgerTransaction) bool {
	for _, operation := range transaction.Envelope.Operations() {
		if f.matchOperation(transaction, operation) {
			return true
		}
	}
	return f.matchFootprint(transaction.Envelope)
}

// matchOperation reads every asset an operation body names.
func (f *assetFilter) matchOperation(transaction ingest.LedgerTransaction, operation xdr.Operation) bool {
	body := operation.Body
	switch body.Type {
	case xdr.OperationTypeChangeTrust:
		return f.matchChangeTrustAsset(&body.ChangeTrustOp.Line)
	case xdr.OperationTypeManageSellOffer:
		return f.matchAsset(&body.ManageSellOfferOp.Buying) || f.matchAsset(&body.ManageSellOfferOp.Selling)
	case xdr.OperationTypeManageBuyOffer:
		return f.matchAsset(&body.ManageBuyOfferOp.Buying) || f.matchAsset(&body.ManageBuyOfferOp.Selling)
	case xdr.OperationTypeCreatePassiveSellOffer:
		return f.matchAsset(&body.CreatePassiveSellOfferOp.Buying) || f.matchAsset(&body.CreatePassiveSellOfferOp.Selling)
	case xdr.OperationTypeCreateClaimableBalance:
		return f.matchAsset(&body.CreateClaimableBalanceOp.Asset)
	case xdr.OperationTypeClawback:
		return f.matchAsset(&body.ClawbackOp.Asset)
	case xdr.OperationTypePayment:
		return f.matchAsset(&body.PaymentOp.Asset)
	case xdr.OperationTypePathPaymentStrictReceive:
		op := body.PathPaymentStrictReceiveOp
		return f.matchAsset(&op.SendAsset) || f.matchAsset(&op.DestAsset) || f.matchAnyAsset(op.Path)
	case xdr.OperationTypePathPaymentStrictSend:
		op := body.PathPaymentStrictSendOp
		return f.matchAsset(&op.SendAsset) || f.matchAsset(&op.DestAsset) || f.matchAnyAsset(op.Path)
	case xdr.OperationTypeSetTrustLineFlags:
		return f.matchAsset(&body.SetTrustLineFlagsOp.Asset)
	case xdr.OperationTypeAllowTrust:
		return f.matchAllowTrust(transaction, operation)
	case xdr.OperationTypeRevokeSponsorship:
		return f.matchRevokeSponsorship(body.RevokeSponsorshipOp)
	case xdr.OperationTypeInvokeHostFunction:
		return f.matchHostFunction(&body.InvokeHostFunctionOp.HostFunction)
	}
	return false
}

// matchAllowTrust rebuilds the asset from the code in the operation. The
// issuer is the operation source, or the transaction source when the
// operation has none. For a fee-bump, Envelope.SourceAccount is the inner
// transaction source.
func (f *assetFilter) matchAllowTrust(transaction ingest.LedgerTransaction, operation xdr.Operation) bool {
	issuer := transaction.Envelope.SourceAccount()
	if operation.SourceAccount != nil {
		issuer = *operation.SourceAccount
	}
	var key assetKey
	switch code := operation.Body.AllowTrustOp.Asset; code.Type {
	case xdr.AssetTypeAssetTypeCreditAlphanum4:
		copy(key.code[:], code.AssetCode4[:])
	case xdr.AssetTypeAssetTypeCreditAlphanum12:
		copy(key.code[:], code.AssetCode12[:])
	default:
		return false
	}
	key.issuer = ed25519Of(issuer)
	_, ok := f.whitelistedAssets[key]
	return ok
}

func ed25519Of(account xdr.MuxedAccount) xdr.Uint256 {
	if account.Type == xdr.CryptoKeyTypeKeyTypeMuxedEd25519 {
		return account.Med25519.Ed25519
	}
	return *account.Ed25519
}

// matchRevokeSponsorship matches nothing for a pool share trustline. The key
// carries only the pool id, and the revoke does not change the pool entry.
func (f *assetFilter) matchRevokeSponsorship(op *xdr.RevokeSponsorshipOp) bool {
	if op.LedgerKey == nil || op.LedgerKey.Type != xdr.LedgerEntryTypeTrustline {
		return false
	}
	return f.matchTrustLineAsset(&op.LedgerKey.TrustLine.Asset)
}

// matchHostFunction matches a call into the Stellar Asset Contract of a
// whitelisted asset, and the deployment of that contract. A failed call has
// no operation meta, so this is the only source that can match it.
func (f *assetFilter) matchHostFunction(fn *xdr.HostFunction) bool {
	switch fn.Type {
	case xdr.HostFunctionTypeHostFunctionTypeInvokeContract:
		return f.matchContractAddress(&fn.InvokeContract.ContractAddress)
	case xdr.HostFunctionTypeHostFunctionTypeCreateContract:
		return f.matchContractPreimage(&fn.CreateContract.ContractIdPreimage)
	case xdr.HostFunctionTypeHostFunctionTypeCreateContractV2:
		return f.matchContractPreimage(&fn.CreateContractV2.ContractIdPreimage)
	}
	return false
}

func (f *assetFilter) matchContractPreimage(preimage *xdr.ContractIdPreimage) bool {
	return preimage.FromAsset != nil && f.matchAsset(preimage.FromAsset)
}

// matchFootprint reads the Soroban footprint of the envelope. It names every
// contract data entry a Soroban transaction reads or writes, so it matches
// ExtendFootprintTTL and RestoreFootprint on the entries of a whitelisted
// Stellar Asset Contract. Those operations write only TTL entries to the meta
// before protocol 23, and a TTL entry carries only a key hash.
func (f *assetFilter) matchFootprint(envelope xdr.TransactionEnvelope) bool {
	var ext *xdr.TransactionExt
	switch envelope.Type {
	case xdr.EnvelopeTypeEnvelopeTypeTx:
		ext = &envelope.V1.Tx.Ext
	case xdr.EnvelopeTypeEnvelopeTypeTxFeeBump:
		ext = &envelope.FeeBump.Tx.InnerTx.V1.Tx.Ext
	default:
		return false
	}
	if ext.SorobanData == nil {
		return false
	}
	footprint := &ext.SorobanData.Resources.Footprint
	return f.matchLedgerKeys(footprint.ReadOnly) || f.matchLedgerKeys(footprint.ReadWrite)
}

func (f *assetFilter) matchLedgerKeys(keys []xdr.LedgerKey) bool {
	for i := range keys {
		if keys[i].Type == xdr.LedgerEntryTypeContractData && f.matchContractAddress(&keys[i].ContractData.Contract) {
			return true
		}
	}
	return false
}

// matchMeta reads the ledger entry changes straight from the transaction meta,
// in order: changes before the operations, the changes of each operation, and
// the changes after the operations. Fee changes are not in the meta and are
// not read. ingest.LedgerTransaction.GetChanges is not used: it rejects meta
// V0, and it sorts every change, which costs time on each transaction the
// filter drops.
func (f *assetFilter) matchMeta(meta xdr.TransactionMeta) (bool, error) {
	switch meta.V {
	case 0:
		// GetOperations would dereference a nil pointer on an empty V0 meta.
		return meta.Operations != nil && f.matchOperationChanges(*meta.Operations), nil
	case 1:
		return f.matchChanges(meta.V1.TxChanges) || f.matchOperationChanges(meta.V1.Operations), nil
	case 2:
		return f.matchChanges(meta.V2.TxChangesBefore) ||
			f.matchOperationChanges(meta.V2.Operations) ||
			f.matchChanges(meta.V2.TxChangesAfter), nil
	case 3:
		return f.matchChanges(meta.V3.TxChangesBefore) ||
			f.matchOperationChanges(meta.V3.Operations) ||
			f.matchChanges(meta.V3.TxChangesAfter), nil
	case 4:
		return f.matchChanges(meta.V4.TxChangesBefore) ||
			f.matchOperationChangesV2(meta.V4.Operations) ||
			f.matchChanges(meta.V4.TxChangesAfter), nil
	default:
		return false, fmt.Errorf("unsupported transaction meta version %d", meta.V)
	}
}

func (f *assetFilter) matchOperationChanges(operations []xdr.OperationMeta) bool {
	for i := range operations {
		if f.matchChanges(operations[i].Changes) {
			return true
		}
	}
	return false
}

func (f *assetFilter) matchOperationChangesV2(operations []xdr.OperationMetaV2) bool {
	for i := range operations {
		if f.matchChanges(operations[i].Changes) {
			return true
		}
	}
	return false
}

// matchChanges skips removals. A removal carries only the ledger key, and core
// writes the state of the removed entry just before it.
func (f *assetFilter) matchChanges(changes xdr.LedgerEntryChanges) bool {
	for i := range changes {
		var entry *xdr.LedgerEntry
		switch change := &changes[i]; change.Type {
		case xdr.LedgerEntryChangeTypeLedgerEntryCreated:
			entry = change.Created
		case xdr.LedgerEntryChangeTypeLedgerEntryUpdated:
			entry = change.Updated
		case xdr.LedgerEntryChangeTypeLedgerEntryState:
			entry = change.State
		case xdr.LedgerEntryChangeTypeLedgerEntryRestored:
			entry = change.Restored
		}
		if entry != nil && f.matchEntry(entry) {
			return true
		}
	}
	return false
}

// matchEntry reads every asset a ledger entry names. Account entries hold
// only lumens and are not read.
func (f *assetFilter) matchEntry(entry *xdr.LedgerEntry) bool {
	switch data := &entry.Data; data.Type {
	case xdr.LedgerEntryTypeTrustline:
		return f.matchTrustLineAsset(&data.TrustLine.Asset)
	case xdr.LedgerEntryTypeOffer:
		return f.matchAsset(&data.Offer.Selling) || f.matchAsset(&data.Offer.Buying)
	case xdr.LedgerEntryTypeClaimableBalance:
		return f.matchAsset(&data.ClaimableBalance.Asset)
	case xdr.LedgerEntryTypeLiquidityPool:
		pool := data.LiquidityPool.Body.ConstantProduct
		return pool != nil && (f.matchAsset(&pool.Params.AssetA) || f.matchAsset(&pool.Params.AssetB))
	case xdr.LedgerEntryTypeContractData:
		// Every entry stored under the Stellar Asset Contract of a whitelisted
		// asset matches: a holder balance, the contract instance written on
		// deployment, an allowance. The contract id is a hash of the asset and
		// the network passphrase, so no other contract can store entries under
		// it. For native lumens this covers holders with a contract address.
		// A holder with an account address keeps lumens in its account entry,
		// which is not read; matchHostFunction covers that transfer.
		return f.matchContractAddress(&data.ContractData.Contract)
	}
	return false
}

func (f *assetFilter) matchContractAddress(address *xdr.ScAddress) bool {
	return address.ContractId != nil && f.contractIDs.Contains(*address.ContractId)
}

// matchTrustLineAsset skips pool share trustlines. They carry a pool id, not
// an asset. A deposit or a withdrawal also changes the pool entry, which
// names the assets.
func (f *assetFilter) matchTrustLineAsset(asset *xdr.TrustLineAsset) bool {
	if asset.Type == xdr.AssetTypeAssetTypePoolShare {
		return false
	}
	_, ok := f.whitelistedAssets[alphaNumKey(asset.Type, asset.AlphaNum4, asset.AlphaNum12)]
	return ok
}

func (f *assetFilter) matchChangeTrustAsset(asset *xdr.ChangeTrustAsset) bool {
	if asset.Type == xdr.AssetTypeAssetTypePoolShare {
		pool := asset.LiquidityPool.ConstantProduct
		return pool != nil && (f.matchAsset(&pool.AssetA) || f.matchAsset(&pool.AssetB))
	}
	_, ok := f.whitelistedAssets[alphaNumKey(asset.Type, asset.AlphaNum4, asset.AlphaNum12)]
	return ok
}

func (f *assetFilter) matchAnyAsset(assets []xdr.Asset) bool {
	for i := range assets {
		if f.matchAsset(&assets[i]) {
			return true
		}
	}
	return false
}

func (f *assetFilter) matchAsset(asset *xdr.Asset) bool {
	_, ok := f.whitelistedAssets[alphaNumKey(asset.Type, asset.AlphaNum4, asset.AlphaNum12)]
	return ok
}

func (f *assetFilter) isEnabled() bool {
	// filtering is disabled if the whitelist is empty for now as that is the only filter rule
	return f.whitelistLen >= 1 && f.enabled
}
