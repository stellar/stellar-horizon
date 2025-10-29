package integration

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/ingest/ledgerbackend"
	"github.com/stellar/go/ingest/loadtest"
	horizoncmd "github.com/stellar/go/services/horizon/cmd"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	horizoningest "github.com/stellar/go/services/horizon/internal/ingest"
	"github.com/stellar/go/services/horizon/internal/test/integration"
	"github.com/stellar/go/support/db"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
)

func TestLoadTestLedgerBackend(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 23 {
		t.Skip("This test run does not support less than Protocol 23")
	}

	itest := integration.NewTest(t, integration.Config{
		NetworkPassphrase: loadTestNetworkPassphrase,
	})
	senderKP, senderAccount := itest.CreateAccount("10000000")
	recipientKP, _ := itest.CreateAccount("10000000")

	tx := itest.MustSubmitOperations(
		senderAccount,
		senderKP,
		&txnbuild.Payment{
			SourceAccount: senderKP.Address(),
			Destination:   recipientKP.Address(),
			Asset:         txnbuild.NativeAsset{},
			Amount:        "1000",
		},
	)
	require.True(t, tx.Successful)

	replayConfig := loadtest.LedgerBackendConfig{
		NetworkPassphrase:   "invalid passphrase",
		LedgersFilePath:     filepath.Join("testdata", fmt.Sprintf("load-test-ledgers-v%d.xdr.zstd", itest.Config().ProtocolVersion)),
		LedgerCloseDuration: 3 * time.Second / 2,
		LedgerBackend:       newCaptiveCore(itest),
	}
	var generatedLedgers []xdr.LedgerCloseMeta

	readFile(t, replayConfig.LedgersFilePath,
		func() *xdr.LedgerCloseMeta { return &xdr.LedgerCloseMeta{} },
		func(ledger *xdr.LedgerCloseMeta) {
			generatedLedgers = append(generatedLedgers, *ledger)
		},
	)

	startLedger := uint32(tx.Ledger - 1)
	endLedger := startLedger + uint32(len(generatedLedgers)-1)

	itest.WaitForLedgerInArchive(6*time.Minute, endLedger)

	loadTestBackend := loadtest.NewLedgerBackend(replayConfig)
	// PrepareRange() is expected to fail because of the invalid network passphrase which
	// is validated by the loadtest ledger backend
	require.ErrorContains(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)),
		"unknown tx hash in LedgerCloseMeta",
	)
	require.NoError(t, loadTestBackend.Close())

	// now, we recreate the loadtest ledger backend with the
	// correct network passphrase
	replayConfig.NetworkPassphrase = itest.Config().NetworkPassphrase
	replayConfig.LedgerBackend = newCaptiveCore(itest)
	loadTestBackend = loadtest.NewLedgerBackend(replayConfig)

	_, err := loadTestBackend.GetLatestLedgerSequence(context.Background())
	require.EqualError(t, err, "PrepareRange() must be called before GetLatestLedgerSequence()")

	prepared, err := loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	require.NoError(t, loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)))

	latest, err := loadTestBackend.GetLatestLedgerSequence(context.Background())
	require.NoError(t, err)
	require.Equal(t, endLedger, latest)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.True(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger+1, endLedger))
	require.NoError(t, err)
	require.True(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger-1, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(endLedger+1, endLedger+2))
	require.NoError(t, err)
	require.False(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.UnboundedRange(startLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	require.NoError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)),
	)
	require.NoError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger+1, endLedger)),
	)
	require.EqualError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.UnboundedRange(startLedger)),
		"PrepareRange() already called",
	)

	_, err = loadTestBackend.GetLedger(context.Background(), startLedger-1)
	require.EqualError(t, err,
		fmt.Sprintf(
			"sequence number %v is behind the ledger stream sequence %v",
			startLedger-1,
			startLedger,
		),
	)

	var ledgers []xdr.LedgerCloseMeta
	for cur := startLedger; cur <= endLedger; cur++ {
		startTime := time.Now()
		var ledger xdr.LedgerCloseMeta
		ledger, err = loadTestBackend.GetLedger(context.Background(), cur)
		duration := time.Since(startTime)
		require.NoError(t, err)
		ledgers = append(ledgers, ledger)
		require.WithinDuration(t, startTime.Add(replayConfig.LedgerCloseDuration), startTime.Add(duration), time.Millisecond*100)
		require.Equal(t, cur, ledger.LedgerSequence())
	}

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	_, err = loadTestBackend.GetLedger(context.Background(), endLedger+1)
	require.ErrorIs(t, err, loadtest.ErrLoadTestDone)

	require.NoError(t, loadTestBackend.Close())

	originalLedgers := getLedgers(itest, startLedger, endLedger)

	changes := extractChanges(t, itest.Config().NetworkPassphrase, ledgers[0:1])
	accountSet := map[string]bool{}
	for _, change := range changes {
		if change.Reason == ingest.LedgerEntryChangeReasonUpgrade && change.Type == xdr.LedgerEntryTypeAccount {
			require.Nil(t, change.Pre)
			accountSet[change.Post.Data.MustAccount().AccountId.Address()] = true
		}
	}
	checkLedgerSequenceInChanges(t, changes, startLedger)

	for cur := startLedger + 1; cur <= endLedger; cur++ {
		i := int(cur - startLedger)
		changes = extractChanges(t, itest.Config().NetworkPassphrase, ledgers[i:i+1])
		expectedChanges := extractChanges(
			t, itest.Config().NetworkPassphrase, []xdr.LedgerCloseMeta{originalLedgers[cur], generatedLedgers[i]},
		)
		checkLedgerSequenceInChanges(t, changes, cur)
		// a merge is valid if the ordered list of changes emitted by the merged ledger is equal to
		// the list of changes emitted by dst concatenated by the list of changes emitted by src, or
		// in other words:
		// extractChanges(merge(dst, src)) == concat(extractChanges(dst), extractChanges(src))
		requireChangesAreEqual(t, expectedChanges, changes)
		generatedChanges := extractChanges(t, itest.Config().NetworkPassphrase, []xdr.LedgerCloseMeta{generatedLedgers[i]})
		for _, change := range generatedChanges {
			if change.Type != xdr.LedgerEntryTypeAccount {
				continue
			}
			ledgerKey, err := change.LedgerKey()
			require.NoError(itest.CurrentTest(), err)
			require.True(itest.CurrentTest(), accountSet[ledgerKey.MustAccount().AccountId.Address()])
		}
	}
}

func TestLoadTestLedgerBackendWithoutMerge(t *testing.T) {
	replayConfig := loadtest.LedgerBackendConfig{
		NetworkPassphrase:   "invalid passphrase",
		LedgersFilePath:     filepath.Join("testdata", fmt.Sprintf("load-test-ledgers-v%d.xdr.zstd", horizoningest.MaxSupportedProtocolVersion)),
		LedgerCloseDuration: 3 * time.Second / 2,
	}
	var generatedLedgers []xdr.LedgerCloseMeta

	readFile(t, replayConfig.LedgersFilePath,
		func() *xdr.LedgerCloseMeta { return &xdr.LedgerCloseMeta{} },
		func(ledger *xdr.LedgerCloseMeta) {
			generatedLedgers = append(generatedLedgers, *ledger)
		},
	)

	startLedger := uint32(100_000_005)
	endLedger := startLedger + uint32(len(generatedLedgers)-1)

	loadTestBackend := loadtest.NewLedgerBackend(replayConfig)
	// PrepareRange() is expected to fail because of the invalid network passphrase which
	// is validated by the loadtest ledger backend
	require.ErrorContains(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)),
		"unknown tx hash in LedgerCloseMeta",
	)
	require.NoError(t, loadTestBackend.Close())

	// now, we recreate the loadtest ledger backend with the
	// correct network passphrase
	replayConfig.NetworkPassphrase = loadTestNetworkPassphrase
	loadTestBackend = loadtest.NewLedgerBackend(replayConfig)

	_, err := loadTestBackend.GetLatestLedgerSequence(context.Background())
	require.EqualError(t, err, "PrepareRange() must be called before GetLatestLedgerSequence()")

	prepared, err := loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	require.NoError(t, loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)))

	latest, err := loadTestBackend.GetLatestLedgerSequence(context.Background())
	require.NoError(t, err)
	require.Equal(t, endLedger, latest)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.True(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger+1, endLedger))
	require.NoError(t, err)
	require.True(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger-1, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(endLedger+1, endLedger+2))
	require.NoError(t, err)
	require.False(t, prepared)

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.UnboundedRange(startLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	require.NoError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger)),
	)
	require.NoError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.BoundedRange(startLedger+1, endLedger)),
	)
	require.EqualError(
		t,
		loadTestBackend.PrepareRange(context.Background(), ledgerbackend.UnboundedRange(startLedger)),
		"PrepareRange() already called",
	)

	_, err = loadTestBackend.GetLedger(context.Background(), startLedger-1)
	require.EqualError(t, err,
		fmt.Sprintf(
			"sequence number %v is behind the ledger stream sequence %v",
			startLedger-1,
			startLedger,
		),
	)

	var ledgers []xdr.LedgerCloseMeta
	for cur := startLedger; cur <= endLedger; cur++ {
		startTime := time.Now()
		var ledger xdr.LedgerCloseMeta
		ledger, err = loadTestBackend.GetLedger(context.Background(), cur)
		duration := time.Since(startTime)
		require.NoError(t, err)
		ledgers = append(ledgers, ledger)
		require.WithinDuration(t, startTime.Add(replayConfig.LedgerCloseDuration), startTime.Add(duration), time.Millisecond*100)
		require.Equal(t, cur, ledger.LedgerSequence())
	}

	prepared, err = loadTestBackend.IsPrepared(context.Background(), ledgerbackend.BoundedRange(startLedger, endLedger))
	require.NoError(t, err)
	require.False(t, prepared)

	_, err = loadTestBackend.GetLedger(context.Background(), endLedger+1)
	require.ErrorIs(t, err, loadtest.ErrLoadTestDone)

	require.NoError(t, loadTestBackend.Close())

	changes := extractChanges(t, loadTestNetworkPassphrase, ledgers[0:1])
	accountSet := map[string]bool{}
	for _, change := range changes {
		if change.Reason == ingest.LedgerEntryChangeReasonUpgrade && change.Type == xdr.LedgerEntryTypeAccount {
			require.Nil(t, change.Pre)
			accountSet[change.Post.Data.MustAccount().AccountId.Address()] = true
		}
	}
	checkLedgerSequenceInChanges(t, changes, startLedger)

	for cur := startLedger + 1; cur <= endLedger; cur++ {
		i := int(cur - startLedger)
		changes = extractChanges(t, loadTestNetworkPassphrase, ledgers[i:i+1])
		expectedChanges := extractChanges(
			t, loadTestNetworkPassphrase, []xdr.LedgerCloseMeta{generatedLedgers[i]},
		)
		checkLedgerSequenceInChanges(t, changes, cur)
		// a merge is valid if the ordered list of changes emitted by the merged ledger is equal to
		// the list of changes emitted by dst concatenated by the list of changes emitted by src, or
		// in other words:
		// extractChanges(merge(dst, src)) == concat(extractChanges(dst), extractChanges(src))
		requireChangesAreEqual(t, expectedChanges, changes)
		for _, change := range changes {
			if change.Type != xdr.LedgerEntryTypeAccount {
				continue
			}
			ledgerKey, err := change.LedgerKey()
			require.NoError(t, err)
			require.True(t, accountSet[ledgerKey.MustAccount().AccountId.Address()])
		}
	}
}

func TestIngestLoadTestCmd(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 23 {
		t.Skip("This test run does not support less than Protocol 23")
	}
	itest := integration.NewTest(t, integration.Config{
		NetworkPassphrase: loadTestNetworkPassphrase,
	})

	ledgersFilePath := filepath.Join("testdata", fmt.Sprintf("load-test-ledgers-v%d.xdr.zstd", itest.Config().ProtocolVersion))
	var generatedLedgers []xdr.LedgerCloseMeta

	readFile(t, ledgersFilePath,
		func() *xdr.LedgerCloseMeta { return &xdr.LedgerCloseMeta{} },
		func(ledger *xdr.LedgerCloseMeta) {
			generatedLedgers = append(generatedLedgers, *ledger)
		},
	)

	session := &db.Session{DB: itest.GetTestDB().Open()}
	t.Cleanup(func() { session.Close() })
	q := &history.Q{session}

	var oldestLedger uint32
	require.NoError(itest.CurrentTest(), q.ElderLedger(context.Background(), &oldestLedger))

	horizoncmd.RootCmd.SetArgs([]string{
		"ingest", "load-test",
		"--db-url=" + itest.GetTestDB().DSN,
		"--stellar-core-binary-path=" + itest.CoreBinaryPath(),
		"--captive-core-config-path=" + itest.WriteCaptiveCoreConfig(),
		"--captive-core-storage-path=" + t.TempDir(),
		"--captive-core-http-port=0",
		"--network-passphrase=" + itest.Config().NetworkPassphrase,
		"--history-archive-urls=" + integration.HistoryArchiveUrl,
		"--ledgers-path=" + ledgersFilePath,
		"--close-duration=0.1",
		"--skip-txmeta=false",
	})
	var restoreLedger uint32
	var runID string
	var err error
	originalRestore := horizoningest.RestoreSnapshot
	t.Cleanup(func() { horizoningest.RestoreSnapshot = originalRestore })
	horizoningest.RestoreSnapshot = func(ctx context.Context, historyQ history.IngestionQ) error {
		runID, restoreLedger, err = q.GetLoadTestRestoreState(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, runID)
		expectedCurrentLedger := restoreLedger + uint32(len(generatedLedgers))
		var curLedger, curHistoryLedger uint32
		curLedger, err = q.GetLastLedgerIngestNonBlocking(context.Background())
		require.NoError(t, err)
		require.Equal(t, expectedCurrentLedger, curLedger)
		curHistoryLedger, err = q.GetLatestHistoryLedger(context.Background())
		require.NoError(t, err)
		require.Equal(t, curLedger, curHistoryLedger)

		sequence := int(restoreLedger) + 1
		for _, ledger := range generatedLedgers {
			checkLedgerIngested(itest, q, ledger, sequence)
			sequence++
		}

		require.NoError(t, originalRestore(ctx, historyQ))

		curHistoryLedger, err = q.GetLatestHistoryLedger(context.Background())
		require.NoError(t, err)
		require.Equal(t, restoreLedger, curHistoryLedger)
		var version int
		version, err = q.GetIngestVersion(ctx)
		require.NoError(t, err)
		require.Zero(t, version)
		return nil
	}
	require.NoError(t, horizoncmd.RootCmd.Execute())

	_, _, err = q.GetLoadTestRestoreState(context.Background())
	require.ErrorIs(t, err, sql.ErrNoRows)

	// check that all ledgers ingested are correct (including ledgers beyond
	// what was ingested during the load test)
	endLedger := restoreLedger + uint32(len(generatedLedgers)+1)
	require.Eventually(t, func() bool {
		var latestLedger, latestHistoryLedger uint32
		latestLedger, err = q.GetLastLedgerIngestNonBlocking(context.Background())
		require.NoError(t, err)
		latestHistoryLedger, err = q.GetLatestHistoryLedger(context.Background())
		require.NoError(t, err)
		return latestLedger >= endLedger && latestHistoryLedger >= endLedger
	}, time.Minute*5, time.Second)

	realLedgers := getLedgers(itest, oldestLedger, endLedger)
	for _, ledger := range realLedgers {
		checkLedgerIngested(itest, q, ledger, int(ledger.LedgerSequence()))
	}

	// restoring is a no-op if there is no load test which is active
	horizoningest.RestoreSnapshot = originalRestore
	horizoncmd.RootCmd.SetArgs([]string{
		"ingest", "load-test-restore",
		"--db-url=" + itest.GetTestDB().DSN,
	})
	require.NoError(t, horizoncmd.RootCmd.Execute())

	_, _, err = q.GetLoadTestRestoreState(context.Background())
	require.ErrorIs(t, err, sql.ErrNoRows)

	var version int
	version, err = q.GetIngestVersion(context.Background())
	require.NoError(t, err)
	require.Positive(t, version)

	for _, ledger := range realLedgers {
		checkLedgerIngested(itest, q, ledger, int(ledger.LedgerSequence()))
	}
}

func TestIngestLoadTestRestoreCmd(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 23 {
		t.Skip("This test run does not support less than Protocol 23")
	}
	itest := integration.NewTest(t, integration.Config{
		NetworkPassphrase: loadTestNetworkPassphrase,
	})

	ledgersFilePath := filepath.Join("testdata", fmt.Sprintf("load-test-ledgers-v%d.xdr.zstd", itest.Config().ProtocolVersion))
	var generatedLedgers []xdr.LedgerCloseMeta

	readFile(t, ledgersFilePath,
		func() *xdr.LedgerCloseMeta { return &xdr.LedgerCloseMeta{} },
		func(ledger *xdr.LedgerCloseMeta) {
			generatedLedgers = append(generatedLedgers, *ledger)
		},
	)

	session := &db.Session{DB: itest.GetTestDB().Open()}
	t.Cleanup(func() { session.Close() })
	q := &history.Q{session}

	var oldestLedger uint32
	require.NoError(itest.CurrentTest(), q.ElderLedger(context.Background(), &oldestLedger))
	itest.StopHorizon()

	horizoncmd.RootCmd.SetArgs([]string{
		"ingest", "load-test",
		"--db-url=" + itest.GetTestDB().DSN,
		"--stellar-core-binary-path=" + itest.CoreBinaryPath(),
		"--captive-core-config-path=" + itest.WriteCaptiveCoreConfig(),
		"--captive-core-storage-path=" + t.TempDir(),
		"--captive-core-http-port=0",
		"--network-passphrase=" + itest.Config().NetworkPassphrase,
		"--history-archive-urls=" + integration.HistoryArchiveUrl,
		"--ledgers-path=" + ledgersFilePath,
		"--close-duration=0.1",
		"--skip-txmeta=false",
	})
	var restoreLedger uint32
	var runID string
	var err error
	originalRestore := horizoningest.RestoreSnapshot
	t.Cleanup(func() { horizoningest.RestoreSnapshot = originalRestore })
	horizoningest.RestoreSnapshot = func(ctx context.Context, historyQ history.IngestionQ) error {
		return fmt.Errorf("transient error")
	}
	require.ErrorContains(t, horizoncmd.RootCmd.Execute(), "transient error")

	runID, restoreLedger, err = q.GetLoadTestRestoreState(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, runID)
	expectedCurrentLedger := restoreLedger + uint32(len(generatedLedgers))
	var curLedger, curHistoryLedger uint32
	curLedger, err = q.GetLastLedgerIngestNonBlocking(context.Background())
	require.NoError(t, err)
	require.Equal(t, expectedCurrentLedger, curLedger)
	curHistoryLedger, err = q.GetLatestHistoryLedger(context.Background())
	require.NoError(t, err)
	require.Equal(t, curLedger, curHistoryLedger)

	horizoningest.RestoreSnapshot = originalRestore
	horizoncmd.RootCmd.SetArgs([]string{
		"ingest", "load-test-restore",
		"--db-url=" + itest.GetTestDB().DSN,
	})
	require.NoError(t, horizoncmd.RootCmd.Execute())

	_, _, err = q.GetLoadTestRestoreState(context.Background())
	require.ErrorIs(t, err, sql.ErrNoRows)

	curHistoryLedger, err = q.GetLatestHistoryLedger(context.Background())
	require.NoError(t, err)
	require.Equal(t, restoreLedger, curHistoryLedger)
	var version int
	version, err = q.GetIngestVersion(context.Background())
	require.NoError(t, err)
	require.Zero(t, version)
}

func checkLedgerIngested(itest *integration.Test, historyQ *history.Q, ledger xdr.LedgerCloseMeta, sequence int) {
	txReader, err := ingest.NewLedgerTransactionReaderFromLedgerCloseMeta(itest.Config().NetworkPassphrase, ledger)
	require.NoError(itest.CurrentTest(), err)
	txCount := 0
	for {
		var tx ingest.LedgerTransaction
		tx, err = txReader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(itest.CurrentTest(), err)
		txCount++

		var ingestedTx history.Transaction
		err = historyQ.TransactionByHash(context.Background(), &ingestedTx, hex.EncodeToString(tx.Hash[:]))
		require.NoError(itest.CurrentTest(), err)
		var expectedEnvelope string
		expectedEnvelope, err = xdr.MarshalBase64(tx.Envelope)
		require.NoError(itest.CurrentTest(), err)
		require.Equal(itest.CurrentTest(), expectedEnvelope, ingestedTx.TxEnvelope)
	}
	var ingestedLedger history.Ledger
	err = historyQ.LedgerBySequence(context.Background(), &ingestedLedger, int32(sequence))
	require.NoError(itest.CurrentTest(), err)
	require.Equal(itest.CurrentTest(), txCount, int(ingestedLedger.TransactionCount))
}

func newCaptiveCore(itest *integration.Test) *ledgerbackend.CaptiveStellarCore {
	ccConfig, err := itest.CreateCaptiveCoreConfig()
	require.NoError(itest.CurrentTest(), err)

	captiveCore, err := ledgerbackend.NewCaptive(ccConfig)
	require.NoError(itest.CurrentTest(), err)
	return captiveCore
}

func checkLedgerSequenceInChanges(t *testing.T, changes []ingest.Change, curLedger uint32) {
	for _, change := range changes {
		if change.Pre != nil {
			require.LessOrEqual(t, change.Pre.LastModifiedLedgerSeq, curLedger)
		}
		if change.Post != nil {
			require.Equal(t, uint32(change.Post.LastModifiedLedgerSeq), curLedger)
			if change.Post.Data.Type == xdr.LedgerEntryTypeTtl {
				require.GreaterOrEqual(t, change.Post.Data.Ttl.LiveUntilLedgerSeq, curLedger)
			}
		}
	}
}
