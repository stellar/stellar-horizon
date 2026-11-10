package processors

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/guregu/null"

	"github.com/stellar/go-stellar-sdk/exp/orderbook"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/support/db"
	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/go-stellar-sdk/toid"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2/history"
)

// TradeProcessor operations processor
type TradeProcessor struct {
	accountLoader *history.AccountLoader
	lpLoader      *history.LiquidityPoolLoader
	assetLoader   *history.AssetLoader
	batch         history.TradeBatchInsertBuilder
	trades        []ingestTrade
	stats         TradeStats
}

func NewTradeProcessor(
	accountLoader *history.AccountLoader,
	lpLoader *history.LiquidityPoolLoader,
	assetLoader *history.AssetLoader,
	batch history.TradeBatchInsertBuilder,
) *TradeProcessor {
	return &TradeProcessor{
		accountLoader: accountLoader,
		lpLoader:      lpLoader,
		assetLoader:   assetLoader,
		batch:         batch,
	}
}

type TradeStats struct {
	count int64
}

func (p *TradeProcessor) GetStats() TradeStats {
	return p.stats
}

func (stats *TradeStats) Map() map[string]interface{} {
	return map[string]interface{}{
		"stats_count": stats.count,
	}
}

func (p *TradeProcessor) Name() string {
	return "processors.TradeProcessor"
}

// ProcessTransaction process the given transaction
func (p *TradeProcessor) ProcessTransaction(lcm xdr.LedgerCloseMeta, transaction ingest.LedgerTransaction) (err error) {
	if !transaction.Result.Successful() {
		return nil
	}

	trades, err := p.extractTrades(lcm.LedgerHeaderHistoryEntry(), transaction)
	if err != nil {
		return err
	}

	for _, trade := range trades {
		if trade.buyerAccount != "" {
			p.accountLoader.GetFuture(trade.buyerAccount)
		}
		if trade.sellerAccount != "" {
			p.accountLoader.GetFuture(trade.sellerAccount)
		}
		if trade.liquidityPoolID != "" {
			p.lpLoader.GetFuture(trade.liquidityPoolID)
		}
		p.assetLoader.GetFuture(history.AssetKeyFromXDR(trade.boughtAsset))
		p.assetLoader.GetFuture(history.AssetKeyFromXDR(trade.soldAsset))
	}

	p.trades = append(p.trades, trades...)
	p.stats.count += int64(len(trades))
	return nil
}

func (p *TradeProcessor) Flush(ctx context.Context, session db.SessionInterface) error {
	if len(p.trades) == 0 {
		return nil
	}

	for _, trade := range p.trades {
		row := trade.row
		if trade.sellerAccount != "" {
			val, err := p.accountLoader.GetNow(trade.sellerAccount)
			if err != nil {
				return err
			}
			row.BaseAccountID = null.IntFrom(val)
		}
		if trade.buyerAccount != "" {
			val, err := p.accountLoader.GetNow(trade.buyerAccount)
			if err != nil {
				return err
			}
			row.CounterAccountID = null.IntFrom(val)
		}
		if trade.liquidityPoolID != "" {
			val, err := p.lpLoader.GetNow(trade.liquidityPoolID)
			if err != nil {
				return err
			}
			row.BaseLiquidityPoolID = null.IntFrom(val)
		}

		val, err := p.assetLoader.GetNow(history.AssetKeyFromXDR(trade.soldAsset))
		if err != nil {
			return err
		}
		row.BaseAssetID = val

		val, err = p.assetLoader.GetNow(history.AssetKeyFromXDR(trade.boughtAsset))
		if err != nil {
			return err
		}
		row.CounterAssetID = val

		if row.BaseAssetID > row.CounterAssetID {
			row.BaseIsSeller = false
			row.BaseAccountID, row.CounterAccountID = row.CounterAccountID, row.BaseAccountID
			row.BaseAssetID, row.CounterAssetID = row.CounterAssetID, row.BaseAssetID
			row.BaseAmount, row.CounterAmount = row.CounterAmount, row.BaseAmount
			row.BaseLiquidityPoolID, row.CounterLiquidityPoolID = row.CounterLiquidityPoolID, row.BaseLiquidityPoolID
			row.BaseOfferID, row.CounterOfferID = row.CounterOfferID, row.BaseOfferID
			row.PriceN, row.PriceD = row.PriceD, row.PriceN

			if row.BaseIsExact.Valid {
				row.BaseIsExact = null.BoolFrom(!row.BaseIsExact.Bool)
			}
		}

		if err := p.batch.Add(row); err != nil {
			return errors.Wrap(err, "Error adding trade to batch")
		}
	}

	if err := p.batch.Exec(ctx, session); err != nil {
		return errors.Wrap(err, "Error flushing operation batch")
	}
	return nil
}

func (p *TradeProcessor) findTradeSellPrice(
	entries operationEntries,
	trade xdr.ClaimAtom,
) (int64, int64, error) {
	if trade.Type == xdr.ClaimAtomTypeClaimAtomTypeLiquidityPool {
		return int64(trade.AmountBought()), int64(trade.AmountSold()), nil
	}

	offer, ok := entries.offers[trade.OfferId()]
	if !ok {
		return 0, 0, errors.Errorf("could not find change for trade offer %d", trade.OfferId())
	}

	return int64(offer.Price.N), int64(offer.Price.D), nil
}

// operationEntries contains the offers and liquidity pools that one operation
// changed, in their state before the operation.
type operationEntries struct {
	offers map[xdr.Int64]xdr.OfferEntry
	pools  map[xdr.PoolId]ingest.Change
}

func newOperationEntries(tx ingest.LedgerTransaction, opidx int) (operationEntries, error) {
	changes, err := tx.GetOperationChanges(uint32(opidx))
	if err != nil {
		return operationEntries{}, errors.Wrap(err, "could not determine changes for operation")
	}

	entries := operationEntries{
		offers: map[xdr.Int64]xdr.OfferEntry{},
		pools:  map[xdr.PoolId]ingest.Change{},
	}
	// A later change replaces an earlier change for the same entry.
	for _, change := range changes {
		if change.Pre == nil {
			continue
		}
		switch change.Type {
		case xdr.LedgerEntryTypeOffer:
			offer := change.Pre.Data.MustOffer()
			entries.offers[offer.OfferId] = offer
		case xdr.LedgerEntryTypeLiquidityPool:
			entries.pools[change.Pre.Data.MustLiquidityPool().LiquidityPoolId] = change
		}
	}
	return entries, nil
}

func (p *TradeProcessor) liquidityPoolChange(
	entries operationEntries,
	trade xdr.ClaimAtom,
) (*ingest.Change, error) {
	if trade.Type != xdr.ClaimAtomTypeClaimAtomTypeLiquidityPool {
		return nil, nil
	}

	change, ok := entries.pools[trade.LiquidityPool.LiquidityPoolId]
	if !ok {
		return nil, errors.New("could not find change for liquidity pool")
	}
	return &change, nil
}

func (p *TradeProcessor) liquidityPoolReserves(trade xdr.ClaimAtom, change *ingest.Change) (int64, int64) {
	pre := change.Pre.Data.MustLiquidityPool().Body.ConstantProduct
	a := int64(pre.ReserveA)
	b := int64(pre.ReserveB)
	if !trade.AssetSold().Equals(pre.Params.AssetA) {
		a, b = b, a
	}
	return a, b
}

func (p *TradeProcessor) roundingSlippage(
	transaction ingest.LedgerTransaction,
	opidx int,
	trade xdr.ClaimAtom,
	change *ingest.Change,
) (null.Int, error) {
	disbursedReserves, depositedReserves := p.liquidityPoolReserves(trade, change)

	pre := change.Pre.Data.MustLiquidityPool().Body.ConstantProduct

	op, found := transaction.GetOperation(uint32(opidx))
	if !found {
		return null.Int{}, errors.New("could not find operation")
	}

	amountDeposited := trade.AmountBought()
	amountDisbursed := trade.AmountSold()

	switch op.Body.Type {
	case xdr.OperationTypePathPaymentStrictReceive:
		// User specified the disbursed amount
		_, roundingSlippageBips, ok := orderbook.CalculatePoolExpectation(
			xdr.Int64(depositedReserves),
			xdr.Int64(disbursedReserves),
			amountDisbursed,
			pre.Params.Fee,
			true,
		)
		if !ok {
			// Temporary workaround for https://github.com/stellar/go-stellar-sdk/issues/4203
			// Given strict receives that would underflow here, set maximum
			// slippage so they get excluded.
			roundingSlippageBips = xdr.Int64(math.MaxInt64)
		}
		return null.IntFrom(int64(roundingSlippageBips)), nil
	case xdr.OperationTypePathPaymentStrictSend:
		// User specified the disbursed amount
		_, roundingSlippageBips, ok := orderbook.CalculatePoolPayout(
			xdr.Int64(depositedReserves),
			xdr.Int64(disbursedReserves),
			amountDeposited,
			pre.Params.Fee,
			true,
		)
		if !ok {
			// Temporary workaround for https://github.com/stellar/go-stellar-sdk/issues/4203
			// Given strict sends that would overflow here, set maximum slippage
			// so they get excluded.
			roundingSlippageBips = xdr.Int64(math.MaxInt64)
		}
		return null.IntFrom(int64(roundingSlippageBips)), nil
	default:
		return null.Int{}, fmt.Errorf("unexpected trade operation type: %v", op.Body.Type)
	}
}

func (p *TradeProcessor) findPoolFee(
	entries operationEntries,
	poolID xdr.PoolId,
) (uint32, error) {
	change, ok := entries.pools[poolID]
	if !ok {
		return 0, errors.New("could not find change for liquidity pool")
	}

	return uint32(change.Pre.Data.MustLiquidityPool().Body.MustConstantProduct().Params.Fee), nil
}

type ingestTrade struct {
	row             history.InsertTrade
	sellerAccount   string
	liquidityPoolID string
	buyerAccount    string
	boughtAsset     xdr.Asset
	soldAsset       xdr.Asset
}

func (p *TradeProcessor) extractTrades(
	ledger xdr.LedgerHeaderHistoryEntry,
	transaction ingest.LedgerTransaction,
) ([]ingestTrade, error) {
	var result []ingestTrade

	closeTime := time.Unix(int64(ledger.Header.ScpValue.CloseTime), 0).UTC()

	opResults, ok := transaction.Result.OperationResults()
	if !ok {
		return result, errors.New("transaction has no operation results")
	}
	for opidx, op := range transaction.Envelope.Operations() {
		var trades []xdr.ClaimAtom
		var buyOfferExists bool
		var buyOffer xdr.OfferEntry

		switch op.Body.Type {
		case xdr.OperationTypePathPaymentStrictReceive:
			trades = opResults[opidx].MustTr().MustPathPaymentStrictReceiveResult().
				MustSuccess().
				Offers

		case xdr.OperationTypePathPaymentStrictSend:
			trades = opResults[opidx].MustTr().
				MustPathPaymentStrictSendResult().
				MustSuccess().
				Offers

		case xdr.OperationTypeManageBuyOffer:
			manageOfferResult := opResults[opidx].MustTr().MustManageBuyOfferResult().
				MustSuccess()
			trades = manageOfferResult.OffersClaimed
			buyOffer, buyOfferExists = manageOfferResult.Offer.GetOffer()

		case xdr.OperationTypeManageSellOffer:
			manageOfferResult := opResults[opidx].MustTr().MustManageSellOfferResult().
				MustSuccess()
			trades = manageOfferResult.OffersClaimed
			buyOffer, buyOfferExists = manageOfferResult.Offer.GetOffer()

		case xdr.OperationTypeCreatePassiveSellOffer:
			result := opResults[opidx].MustTr()

			// KNOWN ISSUE:  stellar-core creates results for CreatePassiveOffer operations
			// with the wrong result arm set.
			if result.Type == xdr.OperationTypeManageSellOffer {
				manageOfferResult := result.MustManageSellOfferResult().MustSuccess()
				trades = manageOfferResult.OffersClaimed
				buyOffer, buyOfferExists = manageOfferResult.Offer.GetOffer()
			} else {
				passiveOfferResult := result.MustCreatePassiveSellOfferResult().MustSuccess()
				trades = passiveOfferResult.OffersClaimed
				buyOffer, buyOfferExists = passiveOfferResult.Offer.GetOffer()
			}
		}

		opID := toid.New(
			int32(ledger.Header.LedgerSeq), int32(transaction.Index), int32(opidx+1),
		).ToInt64()
		if len(trades) == 0 {
			continue
		}
		entries, err := newOperationEntries(transaction, opidx)
		if err != nil {
			return result, err
		}
		for order, trade := range trades {
			// stellar-core will opportunistically garbage collect invalid offers (in the
			// event that a trader spends down their balance).  These garbage collected
			// offers get emitted in the result with the amount values set to zero.
			//
			// These zeroed ClaimOfferAtom values do not represent trades, and so we
			// skip them.
			if trade.AmountBought() == 0 && trade.AmountSold() == 0 {
				continue
			}

			sellPriceN, sellPriceD, err := p.findTradeSellPrice(entries, trade)
			if err != nil {
				return result, err
			}

			row := history.InsertTrade{
				HistoryOperationID: opID,
				Order:              int32(order),
				LedgerCloseTime:    closeTime,
				CounterAmount:      int64(trade.AmountBought()),
				BaseAmount:         int64(trade.AmountSold()),
				BaseIsSeller:       true,
				PriceN:             sellPriceN,
				PriceD:             sellPriceD,
			}

			switch op.Body.Type {
			case xdr.OperationTypePathPaymentStrictSend:
				row.BaseIsExact = null.BoolFrom(false)
			case xdr.OperationTypePathPaymentStrictReceive:
				row.BaseIsExact = null.BoolFrom(true)
			}

			var sellerAccount, liquidityPoolID string
			if trade.Type == xdr.ClaimAtomTypeClaimAtomTypeLiquidityPool {
				id := trade.MustLiquidityPool().LiquidityPoolId
				liquidityPoolID = PoolIDToString(id)
				var fee uint32
				if fee, err = p.findPoolFee(entries, id); err != nil {
					return nil, err
				}
				row.LiquidityPoolFee = null.IntFrom(int64(fee))
				row.Type = history.LiquidityPoolTradeType

				change, err := p.liquidityPoolChange(entries, trade)
				if err != nil {
					return nil, err
				}
				if change != nil {
					row.RoundingSlippage, err = p.roundingSlippage(transaction, opidx, trade, change)
					if err != nil {
						return nil, err
					}
				}
			} else {
				row.BaseOfferID = null.IntFrom(int64(trade.OfferId()))
				sellerAccount = trade.SellerId().Address()
				row.Type = history.OrderbookTradeType
			}

			if buyOfferExists {
				row.CounterOfferID = null.IntFrom(EncodeOfferId(uint64(buyOffer.OfferId), CoreOfferIDType))
			} else {
				row.CounterOfferID = null.IntFrom(EncodeOfferId(uint64(opID), TOIDType))
			}

			var buyerAddress string
			if buyer := op.SourceAccount; buyer != nil {
				accid := buyer.ToAccountId()
				buyerAddress = accid.Address()
			} else {
				sa := transaction.Envelope.SourceAccount().ToAccountId()
				buyerAddress = sa.Address()
			}

			result = append(result, ingestTrade{
				row:             row,
				sellerAccount:   sellerAccount,
				liquidityPoolID: liquidityPoolID,
				buyerAccount:    buyerAddress,
				boughtAsset:     trade.AssetBought(),
				soldAsset:       trade.AssetSold(),
			})
		}
	}

	return result, nil
}

func (p *TradeProcessor) ResetStats() {
	p.stats = TradeStats{}
}
