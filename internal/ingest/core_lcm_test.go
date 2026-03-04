package ingest

import (
	"context"
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
		readErr := stream.ReadOne(&lcm)
		if readErr == io.EOF {
			break
		}
		require.NoError(t, readErr, "failed to decode LedgerCloseMeta from %s", path)
		ledgers = append(ledgers, lcm)
	}
	return ledgers
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
			// t.Parallel()

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
				t.Logf("ingesting ledger %d through all processors", lcm.LedgerSequence())

				require.NoError(t, historyQ.Begin(ctx),
					"failed to begin transaction for ledger index %d", i)
				defer historyQ.Rollback()

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
