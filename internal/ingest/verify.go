package ingest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/guregu/null"
	"github.com/prometheus/client_golang/prometheus"

	ingestsdk "github.com/stellar/go/ingest"

	"github.com/stellar/go/services/horizon/internal/db2"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/ingest/processors"
	"github.com/stellar/go/support/errors"
	logpkg "github.com/stellar/go/support/log"
	"github.com/stellar/go/xdr"
)

const assetStatsBatchSize = 500
const verifyBatchSize = 50000

// TransformLedgerEntryFunction is a function that transforms ledger entry
// into a form that should be compared to checkpoint state. It can be also used
// to decide if the given entry should be ignored during verification.
// Sometimes the application needs only specific type entries or specific fields
// for a given entry type. Use this function to create a common form of an entry
// that will be used for equality check.
type TransformLedgerEntryFunction func(xdr.LedgerEntry) (ignore bool, newEntry xdr.LedgerEntry)

// StateVerifier verifies if ledger entries provided by Add method are the same
// as in the checkpoint ledger entries provided by CheckpointChangeReader.
// The algorithm works in the following way:
//  0. Develop `transformFunction`. It should remove all fields and objects not
//     stored in your app. For example, if you only store accounts, all other
//     ledger entry types should be ignored (return ignore = true).
//  1. In a loop, get entries from history archive by calling GetEntries()
//     and Write() your version of entries found in the batch (in any order).
//  2. When GetEntries() return no more entries, call Verify with a number of
//     entries in your storage (to find if some extra entires exist in your
//     storage).
//
// Functions will return StateError type if state is found to be incorrect.
// It's user responsibility to call `stateReader.Close()` when reading is done.
// Check Horizon for an example how to use this tool.
type StateVerifier struct {
	stateReader ingestsdk.ChangeReader
	// transformFunction transforms (or ignores) ledger entries streamed from
	// checkpoint buckets to match the form added by `Write`. Read
	// TransformLedgerEntryFunction godoc for more information.
	transformFunction TransformLedgerEntryFunction

	readEntries int
	readingDone bool

	currentEntries map[string]xdr.LedgerEntry
	encodingBuffer *xdr.EncodingBuffer
}

// StateVerifierExpectedIngestionVersion defines a version of ingestion system
// required by state verifier. This is done to prevent situations where
// ingestion has been updated with new features but state verifier does not
// check them.
// There is a test that checks it, to fix it: update the actual `verifyState`
// method instead of just updating this value!
const StateVerifierExpectedIngestionVersion = 20

func NewStateVerifier(stateReader ingestsdk.ChangeReader, tf TransformLedgerEntryFunction) *StateVerifier {
	return &StateVerifier{
		stateReader:       stateReader,
		transformFunction: tf,
		encodingBuffer:    xdr.NewEncodingBuffer(),
	}
}

// GetLedgerEntries returns up to `count` ledger entries from history buckets
// and stores the entries in cache to compare in Write.
func (v *StateVerifier) GetLedgerEntries(count int) ([]xdr.LedgerEntry, error) {
	err := v.checkUnreadEntries()
	if err != nil {
		return nil, err
	}

	entries := make([]xdr.LedgerEntry, 0, count)
	v.currentEntries = make(map[string]xdr.LedgerEntry, count)

	for count > 0 {
		entryChange, err := v.stateReader.Read()
		if err != nil {
			if err == io.EOF {
				v.readingDone = true
				return entries, nil
			}
			return entries, err
		}

		entry := *entryChange.Post

		if v.transformFunction != nil {
			ignore, _ := v.transformFunction(entry)
			if ignore {
				continue
			}
		}

		ledgerKey, err := entry.LedgerKey()
		if err != nil {
			return entries, errors.Wrap(err, "Error marshaling ledgerKey")
		}
		key, err := v.encodingBuffer.MarshalBinary(ledgerKey)
		if err != nil {
			return entries, errors.Wrap(err, "Error marshaling ledgerKey")
		}

		entry.Normalize()
		entries = append(entries, entry)
		v.currentEntries[string(key)] = entry

		count--
		v.readEntries++
	}

	return entries, nil
}

// Write compares the entry with entries in the latest batch of entries fetched
// using `GetEntries`. Entries don't need to follow the order in entries returned
// by `GetEntries`.
// Warning: Write will call Normalize() on `entry` that can modify it!
// Any `StateError` returned by this method indicates invalid state!
func (v *StateVerifier) Write(entry xdr.LedgerEntry) error {
	actualEntry := entry.Normalize()
	actualEntryMarshaled, err := v.encodingBuffer.MarshalBinary(actualEntry)
	if err != nil {
		return errors.Wrap(err, "Error marshaling actualEntry")
	}

	// safe, since we convert to string right away (causing a copy)
	key, err := actualEntry.LedgerKey()
	if err != nil {
		return errors.Wrap(err, "Error marshaling ledgerKey")
	}
	keyBinary, err := v.encodingBuffer.UnsafeMarshalBinary(key)
	if err != nil {
		return errors.Wrap(err, "Error marshaling ledgerKey")
	}
	keyString := string(keyBinary)
	expectedEntry, exist := v.currentEntries[keyString]
	if !exist {
		return ingestsdk.NewStateError(errors.Errorf(
			"Cannot find entry in currentEntries map: %s (key = %s)",
			base64.StdEncoding.EncodeToString(actualEntryMarshaled),
			base64.StdEncoding.EncodeToString(keyBinary),
		))
	}
	delete(v.currentEntries, keyString)

	preTransformExpectedEntry := expectedEntry
	preTransformExpectedEntryMarshaled, err := v.encodingBuffer.MarshalBinary(&preTransformExpectedEntry)
	if err != nil {
		return errors.Wrap(err, "Error marshaling preTransformExpectedEntry")
	}

	if v.transformFunction != nil {
		var ignore bool
		ignore, expectedEntry = v.transformFunction(expectedEntry)
		// Extra check: if entry was ignored in GetEntries, it shouldn't be
		// ignored here.
		if ignore {
			return errors.Errorf(
				"Entry ignored in GetEntries but not ignored in Write: %s. Possibly transformFunction is buggy.",
				base64.StdEncoding.EncodeToString(preTransformExpectedEntryMarshaled),
			)
		}
	}

	expectedEntryMarshaled, err := v.encodingBuffer.MarshalBinary(&expectedEntry)
	if err != nil {
		return errors.Wrap(err, "Error marshaling expectedEntry")
	}

	if !bytes.Equal(actualEntryMarshaled, expectedEntryMarshaled) {
		return ingestsdk.NewStateError(errors.Errorf(
			"Entry does not match the fetched entry. Expected (history archive): %s (pretransform = %s), actual (horizon): %s",
			base64.StdEncoding.EncodeToString(expectedEntryMarshaled),
			base64.StdEncoding.EncodeToString(preTransformExpectedEntryMarshaled),
			base64.StdEncoding.EncodeToString(actualEntryMarshaled),
		))
	}

	return nil
}

// Verify should be run after all GetEntries/Write calls. If there were no errors
// so far it means that all entries present in history buckets matches the entries
// in application storage. However, it's still possible that state is invalid when:
//   - Not all entries have been read from history buckets (ex. due to a bug).
//   - Some entries were not compared using Write.
//   - There are some extra entries in application storage not present in history
//     buckets.
//
// Any `StateError` returned by this method indicates invalid state!
func (v *StateVerifier) Verify(countAll int) error {
	err := v.checkUnreadEntries()
	if err != nil {
		return err
	}

	if !v.readingDone {
		return errors.New("There are unread entries in state reader. Process all entries before calling Verify.")
	}

	if v.readEntries != countAll {
		return ingestsdk.NewStateError(errors.Errorf(
			"Number of entries read using GetEntries (%d) does not match number of entries in your storage (%d).",
			v.readEntries,
			countAll,
		))
	}

	return nil
}

func (v *StateVerifier) checkUnreadEntries() error {
	if len(v.currentEntries) > 0 {
		var entry xdr.LedgerEntry
		for _, e := range v.currentEntries {
			entry = e
			break
		}

		// Ignore error as StateError below is more important
		entryString, _ := v.encodingBuffer.MarshalBase64(&entry)
		return ingestsdk.NewStateError(errors.Errorf(
			"Entries (%d) not found locally, example: %s",
			len(v.currentEntries),
			entryString,
		))
	}

	return nil
}

// verifyState is called as a go routine from pipeline post hook every 64
// ledgers. It checks if the state is correct. If another go routine is already
// running it exits.
func (s *system) verifyState(verifyAgainstLatestCheckpoint bool, checkpointSequence uint32, expectedBucketListHash xdr.Hash) error {
	s.stateVerificationMutex.Lock()
	if s.stateVerificationRunning {
		log.Warn("State verification is already running...")
		s.stateVerificationMutex.Unlock()
		return nil
	}
	s.stateVerificationRunning = true
	s.stateVerificationMutex.Unlock()
	defer func() {
		s.stateVerificationMutex.Lock()
		s.stateVerificationRunning = false
		s.stateVerificationMutex.Unlock()
	}()

	updateMetrics := false

	if StateVerifierExpectedIngestionVersion != CurrentVersion {
		log.Errorf(
			"State verification expected version is %d but actual is: %d",
			StateVerifierExpectedIngestionVersion,
			CurrentVersion,
		)
		return nil
	}

	historyQ := s.historyQ.CloneIngestionQ()
	defer historyQ.Rollback()
	err := historyQ.BeginTx(s.ctx, &sql.TxOptions{
		Isolation: sql.LevelRepeatableRead,
		ReadOnly:  true,
	})
	if err != nil {
		return errors.Wrap(err, "Error starting transaction")
	}

	ctx := s.ctx
	if s.config.StateVerificationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(s.ctx, s.config.StateVerificationTimeout)
		defer cancel()
	}

	// Ensure the ledger is a checkpoint ledger
	ledgerSequence, err := historyQ.GetLastLedgerIngestNonBlocking(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.GetLastLedgerIngestNonBlocking")
	}

	localLog := log.WithFields(logpkg.F{
		"subservice": "state_verify",
		"sequence":   ledgerSequence,
	})

	if !s.runStateVerificationOnLedger(ledgerSequence) {
		localLog.Info("Current ledger is not eligible for state verification. Canceling...")
		return nil
	}

	if ledgerSequence != checkpointSequence {
		localLog.WithField("checkpointSequence", checkpointSequence).
			Info("Current ledger does not match checkpoint sequence. Canceling...")
		return nil
	}

	ok, err := historyQ.TryStateVerificationLock(ctx)
	if err != nil {
		return errors.Wrap(err, "Error acquiring state verification lock")
	}
	if !ok {
		localLog.Info("State verification is already in progress. Canceling...")
		return nil
	}

	localLog.Info("Starting state verification")

	if verifyAgainstLatestCheckpoint {
		retries := 0
		for {
			// Get root HAS to check if we're checking one of the latest ledgers or
			// Horizon is catching up. It doesn't make sense to verify old ledgers as
			// we want to check the latest state.
			var historyLatestSequence uint32
			historyLatestSequence, err = s.historyAdapter.GetLatestLedgerSequence()
			if err != nil {
				return errors.Wrap(err, "Error getting the latest ledger sequence")
			}

			if ledgerSequence < historyLatestSequence {
				localLog.Info("Current ledger is old. Canceling...")
				return nil
			}

			if ledgerSequence == historyLatestSequence {
				break
			}

			localLog.Info("Waiting for stellar-core to publish HAS...")
			select {
			case <-ctx.Done():
				localLog.Info("State verifier shut down...")
				return nil
			case <-time.After(5 * time.Second):
				// Wait for stellar-core to publish HAS
				retries++
				if retries == 12 {
					localLog.Info("Checkpoint not published. Canceling...")
					return nil
				}
			}
		}
	}

	totalByType := map[string]int64{}

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		if updateMetrics {
			// Don't update metrics if context canceled.
			if ctx.Err() != context.Canceled {
				s.Metrics().StateVerifyDuration.Observe(float64(duration))
				for typ, tot := range totalByType {
					s.Metrics().StateVerifyLedgerEntriesCount.
						With(prometheus.Labels{"type": typ}).Set(float64(tot))
				}
			}
		}

		localLog.WithField("duration", duration).Info("State verification finished")
	}()

	localLog.Info("Creating state reader...")

	stateReader, err := s.historyAdapter.GetState(ctx, ledgerSequence)
	if err != nil {
		return errors.Wrap(err, "Error running GetState")
	}
	defer stateReader.Close()
	if err = stateReader.VerifyBucketList(expectedBucketListHash); err != nil {
		return ingestsdk.NewStateError(err)
	}

	verifier := NewStateVerifier(stateReader, func(entry xdr.LedgerEntry) (bool, xdr.LedgerEntry) {
		entryType := entry.Data.Type
		// Won't be persisting protocol 20 ContractData ledger entries (except for Stellar Asset Contract
		// ledger entries) to the history db, therefore must not allow it
		// to be counted in history state-verifier accumulators.
		if entryType == xdr.LedgerEntryTypeConfigSetting || entryType == xdr.LedgerEntryTypeContractCode {
			return true, entry
		}

		return false, entry
	})

	assetStats := processors.NewAssetStatSet()
	createdExpirationEntries := map[xdr.Hash]uint32{}
	var contractDataEntries []xdr.LedgerEntry
	total := int64(0)
	for {
		var entries []xdr.LedgerEntry
		entries, err = verifier.GetLedgerEntries(verifyBatchSize)
		if err != nil {
			return errors.Wrap(err, "verifier.GetLedgerEntries")
		}

		if len(entries) == 0 {
			break
		}

		accounts := make([]string, 0, verifyBatchSize)
		data := make([]xdr.LedgerKeyData, 0, verifyBatchSize)
		offers := make([]int64, 0, verifyBatchSize)
		trustLines := make([]xdr.LedgerKeyTrustLine, 0, verifyBatchSize)
		cBalances := make([]xdr.ClaimableBalanceId, 0, verifyBatchSize)
		lPools := make([]xdr.PoolId, 0, verifyBatchSize)
		for _, entry := range entries {
			switch entry.Data.Type {
			case xdr.LedgerEntryTypeAccount:
				accounts = append(accounts, entry.Data.MustAccount().AccountId.Address())
				totalByType["accounts"]++
			case xdr.LedgerEntryTypeData:
				key, keyErr := entry.LedgerKey()
				if keyErr != nil {
					return errors.Wrap(keyErr, "entry.LedgerKey")
				}
				data = append(data, *key.Data)
				totalByType["data"]++
			case xdr.LedgerEntryTypeOffer:
				offers = append(offers, int64(entry.Data.MustOffer().OfferId))
				totalByType["offers"]++
			case xdr.LedgerEntryTypeTrustline:
				key, keyErr := entry.LedgerKey()
				if keyErr != nil {
					return errors.Wrap(keyErr, "TrustlineEntry.LedgerKey")
				}
				trustLines = append(trustLines, key.MustTrustLine())
				totalByType["trust_lines"]++
			case xdr.LedgerEntryTypeClaimableBalance:
				cBalances = append(cBalances, entry.Data.MustClaimableBalance().BalanceId)
				totalByType["claimable_balances"]++
			case xdr.LedgerEntryTypeLiquidityPool:
				lPools = append(lPools, entry.Data.MustLiquidityPool().LiquidityPoolId)
				totalByType["liquidity_pools"]++
			case xdr.LedgerEntryTypeContractData:
				// contract data is a special case.
				// we don't store contract data entries in the db,
				// however, we ingest contract data entries for asset stats.
				if err = verifier.Write(entry); err != nil {
					return err
				}
				contractDataEntries = append(contractDataEntries, entry)
				totalByType["contract_data"]++
			case xdr.LedgerEntryTypeTtl:
				// we don't store all expiration entries in the db,
				// we will only verify expiration of contract balances in the horizon db.
				if err = verifier.Write(entry); err != nil {
					return err
				}
				totalByType["ttl"]++
				ttl := entry.Data.MustTtl()
				createdExpirationEntries[ttl.KeyHash] = uint32(ttl.LiveUntilLedgerSeq)
				totalByType["expiration"]++
			default:
				return errors.New("GetLedgerEntries return unexpected type")
			}
		}

		err = addAccountsToStateVerifier(ctx, verifier, historyQ, accounts)
		if err != nil {
			return errors.Wrap(err, "addAccountsToStateVerifier failed")
		}

		err = addDataToStateVerifier(ctx, verifier, historyQ, data)
		if err != nil {
			return errors.Wrap(err, "addDataToStateVerifier failed")
		}

		err = addOffersToStateVerifier(ctx, verifier, historyQ, offers)
		if err != nil {
			return errors.Wrap(err, "addOffersToStateVerifier failed")
		}

		err = addTrustLinesToStateVerifier(ctx, verifier, assetStats, historyQ, trustLines)
		if err != nil {
			return errors.Wrap(err, "addTrustLinesToStateVerifier failed")
		}

		err = addClaimableBalanceToStateVerifier(ctx, verifier, assetStats, historyQ, cBalances)
		if err != nil {
			return errors.Wrap(err, "addClaimableBalanceToStateVerifier failed")
		}

		err = addLiquidityPoolsToStateVerifier(ctx, verifier, assetStats, historyQ, lPools)
		if err != nil {
			return errors.Wrap(err, "addLiquidityPoolsToStateVerifier failed")
		}

		total += int64(len(entries))
		localLog.WithField("total", total).Info("Batch added to StateVerifier")
	}

	contractAssetStatSet := processors.NewContractAssetStatSet(
		historyQ,
		s.config.NetworkPassphrase,
		map[xdr.Hash]uint32{},
		createdExpirationEntries,
		map[xdr.Hash][2]uint32{},
		ledgerSequence,
	)
	for i := range contractDataEntries {
		entry := contractDataEntries[i]
		if err = contractAssetStatSet.AddContractData(ingestsdk.Change{
			Type: xdr.LedgerEntryTypeContractData,
			Post: &entry,
		}); err != nil {
			return errors.Wrap(err, "Error ingesting contract data")
		}
	}

	localLog.WithField("total", total).Info("Finished writing to StateVerifier")

	countAccounts, err := historyQ.CountAccounts(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountAccounts")
	}

	countData, err := historyQ.CountAccountsData(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountData")
	}

	countOffers, err := historyQ.CountOffers(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountOffers")
	}

	countTrustLines, err := historyQ.CountTrustLines(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountTrustLines")
	}

	countClaimableBalances, err := historyQ.CountClaimableBalances(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountClaimableBalances")
	}

	countLiquidityPools, err := historyQ.CountLiquidityPools(ctx)
	if err != nil {
		return errors.Wrap(err, "Error running historyQ.CountLiquidityPools")
	}

	err = verifier.Verify(
		countAccounts + countData + countOffers + countTrustLines + countClaimableBalances +
			countLiquidityPools + int(totalByType["contract_data"]) + int(totalByType["ttl"]),
	)
	if err != nil {
		return errors.Wrap(err, "verifier.Verify failed")
	}

	err = checkAssetStats(ctx, assetStats, contractAssetStatSet, historyQ)
	if err != nil {
		return errors.Wrap(err, "checkAssetStats failed")
	}

	localLog.Info("State correct")
	updateMetrics = true
	return nil
}

func checkAssetStats(
	ctx context.Context,
	set processors.AssetStatSet,
	contractAssetStatSet *processors.ContractAssetStatSet,
	q history.IngestionQ,
) error {
	all, balances, err := extractAssetStatsAndBalances(set, contractAssetStatSet)
	if err != nil {
		return err
	}

	page := db2.PageQuery{
		Order: "asc",
		Limit: assetStatsBatchSize,
	}
	for {
		assetStats, err := q.GetAssetStats(ctx, "", "", page)
		if err != nil {
			return errors.Wrap(err, "could not fetch asset stats from db")
		}
		if len(assetStats) == 0 {
			break
		}

		for _, assetStat := range assetStats {
			key := assetStat.AssetCode + ":" + assetStat.AssetIssuer
			fromSet, ok := all[key]
			if !ok {
				return ingestsdk.NewStateError(
					fmt.Errorf(
						"db contains asset stat with code %s issuer %s which is missing from HAS",
						assetStat.AssetCode, assetStat.AssetIssuer,
					),
				)
			}
			delete(all, key)

			if !fromSet.Equals(assetStat) {
				return ingestsdk.NewStateError(
					fmt.Errorf(
						"db asset stat with code %s issuer %s does not match asset stat from HAS: expected=%v actual=%v",
						assetStat.AssetCode, assetStat.AssetIssuer, fromSet, assetStat,
					),
				)
			}
		}

		page.Cursor = assetStats[len(assetStats)-1].PagingToken()
	}

	if len(all) > 0 {
		return ingestsdk.NewStateError(
			fmt.Errorf(
				"HAS contains %d more asset stats than db",
				len(all),
			),
		)
	}

	if err := checkContractBalances(ctx, balances, q); err != nil {
		return err
	}
	return nil
}

func extractAssetStatsAndBalances(set processors.AssetStatSet, contractAssetStatSet *processors.ContractAssetStatSet) (map[string]history.AssetAndContractStat, []history.ContractAssetBalance, error) {
	all := map[string]history.AssetAndContractStat{}
	for _, assetStat := range set.All() {
		// no need to handle the native asset because asset stats only
		// include non-native assets.
		all[assetStat.AssetCode+":"+assetStat.AssetIssuer] = history.AssetAndContractStat{
			ExpAssetStat: assetStat,
			Contracts: history.ContractStat{
				ActiveBalance: "0",
				ActiveHolders: 0,
			},
		}
	}

	contractToStats := map[xdr.ContractId]history.ContractAssetStatRow{}
	for _, row := range contractAssetStatSet.GetContractStats() {
		var contractID xdr.ContractId
		copy(contractID[:], row.ContractID)
		contractToStats[contractID] = row
	}

	assetContracts, err := contractAssetStatSet.GetCreatedAssetContracts()
	if err != nil {
		return nil, nil, errors.Wrap(err, "Error getting created asset contracts")
	}
	for _, assetContract := range assetContracts {
		key := assetContract.AssetCode + ":" + assetContract.AssetIssuer
		entry, ok := all[key]
		if !ok {
			assetType := xdr.AssetTypeAssetTypeCreditAlphanum4
			if len(assetContract.AssetCode) > 4 {
				assetType = xdr.AssetTypeAssetTypeCreditAlphanum12
			}
			entry = history.AssetAndContractStat{
				ExpAssetStat: history.ExpAssetStat{
					AssetType:   assetType,
					AssetCode:   assetContract.AssetCode,
					AssetIssuer: assetContract.AssetIssuer,
					Accounts:    history.ExpAssetStatAccounts{},
					Balances: history.ExpAssetStatBalances{
						Authorized:                      "0",
						AuthorizedToMaintainLiabilities: "0",
						ClaimableBalances:               "0",
						LiquidityPools:                  "0",
						Unauthorized:                    "0",
					},
				},
			}
		}
		contractID := assetContract.ContractID
		entry.ContractID = &contractID
		var contractIDHash xdr.ContractId
		copy(contractIDHash[:], assetContract.ContractID)
		contractStats, ok := contractToStats[contractIDHash]
		if !ok {
			entry.Contracts = history.ContractStat{
				ActiveBalance: "0",
				ActiveHolders: 0,
			}
		} else {
			entry.Contracts = contractStats.Stat
		}
		all[key] = entry
	}

	// only check contract asset balances which belong to stellar asset contracts
	// because other balances may be forged.
	var filteredBalances []history.ContractAssetBalance
	for _, balance := range contractAssetStatSet.GetCreatedBalances() {
		var contractID xdr.ContractId
		copy(contractID[:], balance.ContractID)
		if _, ok := contractToStats[contractID]; ok {
			filteredBalances = append(filteredBalances, balance)
		}
	}
	return all, filteredBalances, nil
}

func checkContractBalances(
	ctx context.Context,
	balances []history.ContractAssetBalance,
	q history.IngestionQ,
) error {
	for i := 0; i < len(balances); {
		end := i + assetStatsBatchSize
		if end > len(balances) {
			end = len(balances)
		}

		subset := balances[i:end]
		var keys []xdr.Hash
		byKey := map[xdr.Hash]history.ContractAssetBalance{}
		for _, balance := range subset {
			var key xdr.Hash
			copy(key[:], balance.KeyHash)
			keys = append(keys, key)
			byKey[key] = balance
		}

		rows, err := q.GetContractAssetBalances(ctx, keys)
		if err != nil {
			return err
		}

		for _, row := range rows {
			var key xdr.Hash
			copy(key[:], row.KeyHash)
			expected := byKey[key]

			if !bytes.Equal(row.ContractID, expected.ContractID) {
				return ingestsdk.NewStateError(
					fmt.Errorf(
						"contract balance %v has contract %v in HAS but is %v in db",
						key,
						expected.ContractID,
						row.ContractID,
					),
				)
			}

			if row.ExpirationLedger != expected.ExpirationLedger {
				return ingestsdk.NewStateError(
					fmt.Errorf(
						"contract balance %v has expiration %v in HAS but is %v in db",
						key,
						expected.ExpirationLedger,
						row.ExpirationLedger,
					),
				)
			}

			if row.Amount != expected.Amount {
				return ingestsdk.NewStateError(
					fmt.Errorf(
						"contract balance %v has amount %v in HAS but is %v in db",
						key,
						expected.Amount,
						row.Amount,
					),
				)
			}
		}

		i = end
	}
	return nil
}

func addAccountsToStateVerifier(ctx context.Context, verifier *StateVerifier, q history.IngestionQ, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	accounts, err := q.GetAccountsByIDs(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetAccountsByIDs")
	}

	signers, err := q.SignersForAccounts(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.SignersForAccounts")
	}

	masterWeightMap := make(map[string]int32)
	signersMap := make(map[string][]xdr.Signer)
	// map[accountID]map[signerKey]sponsor
	sponsoringSignersMap := make(map[string]map[string]string)
	for _, row := range signers {
		if row.Account == row.Signer {
			masterWeightMap[row.Account] = row.Weight
		} else {
			signersMap[row.Account] = append(
				signersMap[row.Account],
				xdr.Signer{
					Key:    xdr.MustSigner(row.Signer),
					Weight: xdr.Uint32(row.Weight),
				},
			)
			if sponsoringSignersMap[row.Account] == nil {
				sponsoringSignersMap[row.Account] = make(map[string]string)
			}
			sponsoringSignersMap[row.Account][row.Signer] = row.Sponsor.String
		}
	}

	for _, row := range accounts {
		var inflationDest *xdr.AccountId
		if row.InflationDestination != "" {
			t := xdr.MustAddress(row.InflationDestination)
			inflationDest = &t
		}

		// Ensure master weight matches, if not it's a state error!
		if int32(row.MasterWeight) != masterWeightMap[row.AccountID] {
			return ingestsdk.NewStateError(
				fmt.Errorf(
					"Master key weight in account %s does not match (expected=%d, actual=%d)",
					row.AccountID,
					masterWeightMap[row.AccountID],
					int32(row.MasterWeight),
				),
			)
		}

		signers := xdr.SortSignersByKey(signersMap[row.AccountID])
		signerSponsoringIDs := make([]xdr.SponsorshipDescriptor, len(signers))
		for i, signer := range signers {
			sponsor := sponsoringSignersMap[row.AccountID][signer.Key.Address()]
			if sponsor != "" {
				signerSponsoringIDs[i] = xdr.MustAddressPtr(sponsor)
			}
		}

		// Accounts that haven't done anything since Protocol 19 will not have a
		// V3 extension, so we need to check whether or not this extension needs
		// to be filled out.
		v3extension := xdr.AccountEntryExtensionV2Ext{V: 0}
		if row.SequenceLedger.Valid && row.SequenceTime.Valid {
			v3extension.V = 3
			v3extension.V3 = &xdr.AccountEntryExtensionV3{
				SeqLedger: xdr.Uint32(row.SequenceLedger.Int64),
				SeqTime:   xdr.TimePoint(row.SequenceTime.Int64),
			}
		}

		account := &xdr.AccountEntry{
			AccountId:     xdr.MustAddress(row.AccountID),
			Balance:       xdr.Int64(row.Balance),
			SeqNum:        xdr.SequenceNumber(row.SequenceNumber),
			NumSubEntries: xdr.Uint32(row.NumSubEntries),
			InflationDest: inflationDest,
			Flags:         xdr.Uint32(row.Flags),
			HomeDomain:    xdr.String32(row.HomeDomain),
			Thresholds: xdr.Thresholds{
				row.MasterWeight,
				row.ThresholdLow,
				row.ThresholdMedium,
				row.ThresholdHigh,
			},
			Signers: signers,
			Ext: xdr.AccountEntryExt{
				V: 1,
				V1: &xdr.AccountEntryExtensionV1{
					Liabilities: xdr.Liabilities{
						Buying:  xdr.Int64(row.BuyingLiabilities),
						Selling: xdr.Int64(row.SellingLiabilities),
					},
					Ext: xdr.AccountEntryExtensionV1Ext{
						V: 2,
						V2: &xdr.AccountEntryExtensionV2{
							NumSponsored:        xdr.Uint32(row.NumSponsored),
							NumSponsoring:       xdr.Uint32(row.NumSponsoring),
							SignerSponsoringIDs: signerSponsoringIDs,
							Ext:                 v3extension,
						},
					},
				},
			},
		}

		entry := xdr.LedgerEntry{
			LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
			Data: xdr.LedgerEntryData{
				Type:    xdr.LedgerEntryTypeAccount,
				Account: account,
			},
		}
		addLedgerEntrySponsor(&entry, row.Sponsor)
		err = verifier.Write(entry)
		if err != nil {
			return err
		}
	}

	return nil
}

func addDataToStateVerifier(ctx context.Context, verifier *StateVerifier, q history.IngestionQ, lkeys []xdr.LedgerKeyData) error {
	if len(lkeys) == 0 {
		return nil
	}
	var keys []history.AccountDataKey
	for _, k := range lkeys {
		keys = append(keys, history.AccountDataKey{
			AccountID: k.AccountId.Address(),
			DataName:  string(k.DataName),
		})
	}
	data, err := q.GetAccountDataByKeys(ctx, keys)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetAccountDataByKeys")
	}

	for _, row := range data {
		entry := xdr.LedgerEntry{
			LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
			Data: xdr.LedgerEntryData{
				Type: xdr.LedgerEntryTypeData,
				Data: &xdr.DataEntry{
					AccountId: xdr.MustAddress(row.AccountID),
					DataName:  xdr.String64(row.Name),
					DataValue: xdr.DataValue(row.Value),
				},
			},
		}
		addLedgerEntrySponsor(&entry, row.Sponsor)
		err := verifier.Write(entry)
		if err != nil {
			return err
		}
	}

	return nil
}

func addOffersToStateVerifier(
	ctx context.Context,
	verifier *StateVerifier,
	q history.IngestionQ,
	ids []int64,
) error {
	if len(ids) == 0 {
		return nil
	}

	offers, err := q.GetOffersByIDs(ctx, ids)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetOfferByIDs")
	}

	for _, row := range offers {
		offerXDR := offerToXDR(row)
		entry := xdr.LedgerEntry{
			LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
			Data: xdr.LedgerEntryData{
				Type:  xdr.LedgerEntryTypeOffer,
				Offer: &offerXDR,
			},
		}
		addLedgerEntrySponsor(&entry, row.Sponsor)
		err := verifier.Write(entry)
		if err != nil {
			return err
		}
	}

	return nil
}

func offerToXDR(row history.Offer) xdr.OfferEntry {
	return xdr.OfferEntry{
		SellerId: xdr.MustAddress(row.SellerID),
		OfferId:  xdr.Int64(row.OfferID),
		Selling:  row.SellingAsset,
		Buying:   row.BuyingAsset,
		Amount:   xdr.Int64(row.Amount),
		Price: xdr.Price{
			N: xdr.Int32(row.Pricen),
			D: xdr.Int32(row.Priced),
		},
		Flags: xdr.Uint32(row.Flags),
	}
}

func addTrustLinesToStateVerifier(
	ctx context.Context,
	verifier *StateVerifier,
	assetStats processors.AssetStatSet,
	q history.IngestionQ,
	keys []xdr.LedgerKeyTrustLine,
) error {
	if len(keys) == 0 {
		return nil
	}

	var ledgerKeyStrings []string
	for _, key := range keys {
		var ledgerKey xdr.LedgerKey
		if err := ledgerKey.SetTrustline(key.AccountId, key.Asset); err != nil {
			return errors.Wrap(err, "Error running ledgerKey.SetTrustline")
		}
		b64, err := ledgerKey.MarshalBinaryBase64()
		if err != nil {
			return errors.Wrap(err, "Error running ledgerKey.MarshalBinaryBase64")
		}
		ledgerKeyStrings = append(ledgerKeyStrings, b64)
	}

	trustLines, err := q.GetTrustLinesByKeys(ctx, ledgerKeyStrings)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetTrustLinesByKeys")
	}

	for _, row := range trustLines {
		var entry xdr.LedgerEntry
		entry, err = trustLineToXDR(row)
		if err != nil {
			return err
		}

		if err = verifier.Write(entry); err != nil {
			return err
		}
		if err = assetStats.AddTrustline(
			ingestsdk.Change{
				Post: &entry,
			},
		); err != nil {
			return ingestsdk.NewStateError(
				errors.Wrap(err, "could not add trustline to asset stats"),
			)
		}
	}

	return nil
}

func trustLineToXDR(row history.TrustLine) (xdr.LedgerEntry, error) {
	var asset xdr.TrustLineAsset
	switch row.AssetType {
	case xdr.AssetTypeAssetTypePoolShare:
		asset = xdr.TrustLineAsset{
			Type:            xdr.AssetTypeAssetTypePoolShare,
			LiquidityPoolId: &xdr.PoolId{},
		}
		_, err := hex.Decode((*asset.LiquidityPoolId)[:], []byte(row.LiquidityPoolID))
		if err != nil {
			return xdr.LedgerEntry{}, errors.Wrap(err, "Error decoding liquidity pool id")
		}
	case xdr.AssetTypeAssetTypeNative:
		asset = xdr.MustNewNativeAsset().ToTrustLineAsset()
	default:
		creditAsset, err := xdr.NewCreditAsset(row.AssetCode, row.AssetIssuer)
		if err != nil {
			return xdr.LedgerEntry{}, errors.Wrap(err, "Error decoding credit asset")
		}
		asset = creditAsset.ToTrustLineAsset()
	}

	trustline := xdr.TrustLineEntry{
		AccountId: xdr.MustAddress(row.AccountID),
		Asset:     asset,
		Balance:   xdr.Int64(row.Balance),
		Limit:     xdr.Int64(row.Limit),
		Flags:     xdr.Uint32(row.Flags),
		Ext: xdr.TrustLineEntryExt{
			V: 1,
			V1: &xdr.TrustLineEntryV1{
				Liabilities: xdr.Liabilities{
					Buying:  xdr.Int64(row.BuyingLiabilities),
					Selling: xdr.Int64(row.SellingLiabilities),
				},
			},
		},
	}
	entry := xdr.LedgerEntry{
		LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
		Data: xdr.LedgerEntryData{
			Type:      xdr.LedgerEntryTypeTrustline,
			TrustLine: &trustline,
		},
	}
	addLedgerEntrySponsor(&entry, row.Sponsor)
	return entry, nil
}

func addClaimableBalanceToStateVerifier(
	ctx context.Context,
	verifier *StateVerifier,
	assetStats processors.AssetStatSet,
	q history.IngestionQ,
	ids []xdr.ClaimableBalanceId,
) error {
	if len(ids) == 0 {
		return nil
	}

	var idStrings []string
	e := xdr.NewEncodingBuffer()
	for _, id := range ids {
		idString, err := e.MarshalHex(id)
		if err != nil {
			return err
		}
		idStrings = append(idStrings, idString)
	}
	cBalances, err := q.GetClaimableBalancesByID(ctx, idStrings)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetClaimableBalancesByID")
	}

	cBalancesClaimants, err := q.GetClaimantsByClaimableBalances(ctx, idStrings)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetClaimantsByClaimableBalances")
	}

	for _, row := range cBalances {
		claimants := []xdr.Claimant{}
		for _, claimant := range row.Claimants {
			claimants = append(claimants, xdr.Claimant{
				Type: xdr.ClaimantTypeClaimantTypeV0,
				V0: &xdr.ClaimantV0{
					Destination: xdr.MustAddress(claimant.Destination),
					Predicate:   claimant.Predicate,
				},
			})
		}
		claimants = xdr.SortClaimantsByDestination(claimants)

		// Check if balances in claimable_balance_claimants table match.
		if len(claimants) != len(cBalancesClaimants[row.BalanceID]) {
			return ingestsdk.NewStateError(
				fmt.Errorf(
					"claimable_balance_claimants length (%d) for claimants doesn't match claimable_balance table (%d)",
					len(cBalancesClaimants[row.BalanceID]), len(claimants),
				),
			)
		}

		for i, claimant := range claimants {
			if claimant.MustV0().Destination.Address() != cBalancesClaimants[row.BalanceID][i].Destination ||
				row.LastModifiedLedger != cBalancesClaimants[row.BalanceID][i].LastModifiedLedger {
				return fmt.Errorf(
					"claimable_balance_claimants table for balance %s does not match. expectedDestination=%s actualDestination=%s, expectedLastModifiedLedger=%d actualLastModifiedLedger=%d",
					row.BalanceID,
					claimant.MustV0().Destination.Address(),
					cBalancesClaimants[row.BalanceID][i].Destination,
					row.LastModifiedLedger,
					cBalancesClaimants[row.BalanceID][i].LastModifiedLedger,
				)
			}
		}

		var balanceID xdr.ClaimableBalanceId
		if err := xdr.SafeUnmarshalHex(row.BalanceID, &balanceID); err != nil {
			return err
		}
		cBalance := xdr.ClaimableBalanceEntry{
			BalanceId: balanceID,
			Claimants: claimants,
			Asset:     row.Asset,
			Amount:    row.Amount,
		}
		if row.Flags != 0 {
			cBalance.Ext = xdr.ClaimableBalanceEntryExt{
				V: 1,
				V1: &xdr.ClaimableBalanceEntryExtensionV1{
					Flags: xdr.Uint32(row.Flags),
				},
			}
		}
		entry := xdr.LedgerEntry{
			LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
			Data: xdr.LedgerEntryData{
				Type:             xdr.LedgerEntryTypeClaimableBalance,
				ClaimableBalance: &cBalance,
			},
		}
		addLedgerEntrySponsor(&entry, row.Sponsor)
		if err := verifier.Write(entry); err != nil {
			return err
		}

		if err := assetStats.AddClaimableBalance(
			ingestsdk.Change{
				Post: &entry,
			},
		); err != nil {
			return ingestsdk.NewStateError(
				errors.Wrap(err, "could not add claimable balance to asset stats"),
			)
		}
	}

	return nil
}

func addLiquidityPoolsToStateVerifier(
	ctx context.Context,
	verifier *StateVerifier,
	assetStats processors.AssetStatSet,
	q history.IngestionQ,
	ids []xdr.PoolId,
) error {
	if len(ids) == 0 {
		return nil
	}
	var idsHex = make([]string, len(ids))
	for i, id := range ids {
		idsHex[i] = processors.PoolIDToString(id)

	}
	lPools, err := q.GetLiquidityPoolsByID(ctx, idsHex)
	if err != nil {
		return errors.Wrap(err, "Error running history.Q.GetLiquidityPoolsByID")
	}

	for _, row := range lPools {
		lPoolEntry, err := liquidityPoolToXDR(row)
		if err != nil {
			return errors.Wrap(err, "Invalid liquidity pool row")
		}

		entry := xdr.LedgerEntry{
			LastModifiedLedgerSeq: xdr.Uint32(row.LastModifiedLedger),
			Data: xdr.LedgerEntryData{
				Type:          xdr.LedgerEntryTypeLiquidityPool,
				LiquidityPool: &lPoolEntry,
			},
		}
		if err := verifier.Write(entry); err != nil {
			return err
		}

		if err := assetStats.AddLiquidityPool(
			ingestsdk.Change{
				Post: &entry,
			},
		); err != nil {
			return ingestsdk.NewStateError(
				errors.Wrap(err, "could not add claimable balance to asset stats"),
			)
		}
	}

	return nil
}

func liquidityPoolToXDR(row history.LiquidityPool) (xdr.LiquidityPoolEntry, error) {
	if len(row.AssetReserves) != 2 {
		return xdr.LiquidityPoolEntry{}, fmt.Errorf("unexpected number of asset reserves (%d), expected %d", len(row.AssetReserves), 2)
	}
	id, err := hex.DecodeString(row.PoolID)
	if err != nil {
		return xdr.LiquidityPoolEntry{}, errors.Wrap(err, "Error decoding pool ID")
	}
	var poolID xdr.PoolId
	if len(id) != len(poolID) {
		return xdr.LiquidityPoolEntry{}, fmt.Errorf("Error decoding pool ID, incorrect length (%d)", len(id))
	}
	copy(poolID[:], id)

	var lPoolEntry = xdr.LiquidityPoolEntry{
		LiquidityPoolId: poolID,
		Body: xdr.LiquidityPoolEntryBody{
			Type: row.Type,
			ConstantProduct: &xdr.LiquidityPoolEntryConstantProduct{
				Params: xdr.LiquidityPoolConstantProductParameters{
					AssetA: row.AssetReserves[0].Asset,
					AssetB: row.AssetReserves[1].Asset,
					Fee:    xdr.Int32(row.Fee),
				},
				ReserveA:                 xdr.Int64(row.AssetReserves[0].Reserve),
				ReserveB:                 xdr.Int64(row.AssetReserves[1].Reserve),
				TotalPoolShares:          xdr.Int64(row.ShareCount),
				PoolSharesTrustLineCount: xdr.Int64(row.TrustlineCount),
			},
		},
	}
	return lPoolEntry, nil
}

func addLedgerEntrySponsor(entry *xdr.LedgerEntry, sponsor null.String) {
	ledgerEntrySponsor := xdr.SponsorshipDescriptor(nil)

	if !sponsor.IsZero() {
		ledgerEntrySponsor = xdr.MustAddressPtr(sponsor.String)
	}
	entry.Ext = xdr.LedgerEntryExt{
		V: 1,
		V1: &xdr.LedgerEntryExtensionV1{
			SponsoringId: ledgerEntrySponsor,
		},
	}
}
