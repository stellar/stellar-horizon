package ingest

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/db2/schema"
	"github.com/stellar/stellar-horizon/internal/ingest/filters"

	supportdb "github.com/stellar/go-stellar-sdk/support/db"
	dbtest "github.com/stellar/go-stellar-sdk/support/db/dbtest"
)

const (
	// coreTestLCMDir is the directory containing XDR-encoded LedgerCloseMeta
	// files produced by stellar-core's InvokeHostFunction tests.
	coreTestLCMDir = "./testdata/test-lcms/InvokeHostFunctionTests"
	// coreTestNetworkPassphrase is the network passphrase used by
	// stellar-core's unit tests.
	coreTestNetworkPassphrase = "(V) (;,,;) (V)"
)

// readLedgerCloseMetasFromFile reads all framed XDR LedgerCloseMeta records
// from the given file path.
func readLedgerCloseMetasFromFile(t *testing.T, path string) []xdr.LedgerCloseMeta {
	t.Helper()
	file, err := os.Open(path)
	require.NoError(t, err)
	defer file.Close()

	stream := xdr.NewStream(file)
	var ledgers []xdr.LedgerCloseMeta
	for {
		var lcm xdr.LedgerCloseMeta
		if err := stream.ReadOne(&lcm); err == io.EOF {
			break
		}
		require.NoError(t, err, "failed to decode LedgerCloseMeta from %s", path)
		ledgers = append(ledgers, lcm)
	}
	return ledgers
}

// setLedgerSequence overwrites the ledger sequence number, ledger hash,
// and previous-ledger hash inside a LedgerCloseMeta regardless of its
// version (V0, V1, or V2).  Deterministic unique hashes are derived from
// seq so that DB unique-index constraints are satisfied.  The previous
// ledger hash is derived from seq-1 to form a consistent chain.
func setLedgerSequence(lcm *xdr.LedgerCloseMeta, seq uint32) {
	hash := seqToHash(seq)
	prevHash := seqToHash(seq - 1)

	switch lcm.V {
	case 0:
		lcm.V0.LedgerHeader.Header.LedgerSeq = xdr.Uint32(seq)
		lcm.V0.LedgerHeader.Header.PreviousLedgerHash = prevHash
		lcm.V0.LedgerHeader.Hash = hash
	case 1:
		lcm.V1.LedgerHeader.Header.LedgerSeq = xdr.Uint32(seq)
		lcm.V1.LedgerHeader.Header.PreviousLedgerHash = prevHash
		lcm.V1.LedgerHeader.Hash = hash
	case 2:
		lcm.V2.LedgerHeader.Header.LedgerSeq = xdr.Uint32(seq)
		lcm.V2.LedgerHeader.Header.PreviousLedgerHash = prevHash
		lcm.V2.LedgerHeader.Hash = hash
	default:
		panic(fmt.Sprintf("unsupported LedgerCloseMeta version: %d", lcm.V))
	}
}

// seqToHash returns a deterministic xdr.Hash derived from a ledger sequence
// number so that each ledger gets a globally unique hash.
func seqToHash(seq uint32) xdr.Hash {
	var h xdr.Hash
	binary.BigEndian.PutUint32(h[:4], seq)
	return h
}

// TestCoreLCMIngestion reads every XDR file produced by stellar-core's
// InvokeHostFunction test suite, decodes each framed LedgerCloseMeta, and
// runs Horizon's ingestion processors against a real test database to verify
// that ingestion succeeds without errors.
//
// The test exercises:
//   - XDR decoding of LedgerCloseMeta streams
//   - Extraction of ledger entry changes (via change readers)
//   - Extraction of transactions (via transaction readers)
func TestCoreLCMIngestion(t *testing.T) {
	entries, err := os.ReadDir(coreTestLCMDir)
	require.NoError(t, err, "cannot read LCM test directory %s", coreTestLCMDir)
	require.NotEmpty(t, entries, "no files found in %s", coreTestLCMDir)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".xdr" {
			continue
		}

		t.Run(entry.Name(), func(t *testing.T) {
			t.Parallel()

			// Each parallel sub-test gets its own isolated database so
			// there are no conflicts between concurrent DB transactions
			// or migration resets.
			testDB := dbtest.Postgres(t)
			defer testDB.Close()

			dbConn := testDB.Open()
			defer dbConn.Close()

			_, err := schema.Migrate(dbConn.DB, schema.MigrateUp, 0)
			require.NoError(t, err, "failed to run migrations")

			historyQ := &history.Q{SessionInterface: &supportdb.Session{DB: dbConn}}

			path := filepath.Join(coreTestLCMDir, entry.Name())
			ledgers := readLedgerCloseMetasFromFile(t, path)
			require.NotEmpty(t, ledgers, "expected at least one LedgerCloseMeta in %s", path)
			t.Logf("decoded %d LedgerCloseMeta(s)", len(ledgers))

			// The test LCMs from stellar-core have LedgerSequence==0
			// because core's test harness doesn't populate real sequence
			// numbers.  Inject sequential values starting at 2 (ledger 1
			// is the genesis ledger and some processors treat it
			// specially).
			for i := range ledgers {
				setLedgerSequence(&ledgers[i], uint32(i+2))
			}

			ctx := context.Background()
			runner := ProcessorRunner{
				ctx: ctx,
				config: Config{
					NetworkPassphrase:        coreTestNetworkPassphrase,
					SkipProtocolVersionCheck: true,
				},
				historyQ: historyQ,
				session:  historyQ,
				filters:  filters.NewFilters(),
			}

			// Run the full pipeline (change + transaction processors) on
			// each ledger sequentially, inside a DB transaction.
			for i, lcm := range ledgers {
				require.NoError(t, historyQ.Begin(ctx),
					"failed to begin transaction for ledger index %d", i)
				defer func() { _ = historyQ.Rollback() }()

				_, err := runner.RunAllProcessorsOnLedger(lcm)
				require.NoError(t, err,
					"RunAllProcessorsOnLedger failed on ledger %d (index %d)",
					lcm.LedgerSequence(), i)

				require.NoError(t, historyQ.Commit(),
					"failed to commit transaction for ledger index %d", i)
			}
			t.Logf("successfully ingested %d ledgers through all processors", len(ledgers))
		})
	}
}
