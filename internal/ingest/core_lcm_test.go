package ingest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/protocols/horizon/effects"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/db2/schema"
	"github.com/stellar/stellar-horizon/internal/ingest/filters"

	supportdb "github.com/stellar/go-stellar-sdk/support/db"
	dbtest "github.com/stellar/go-stellar-sdk/support/db/dbtest"
)

const (
	// coreTestLCMDir is the parent directory whose child directories each
	// contain XDR-encoded LedgerCloseMeta files produced by stellar-core's
	// tests. Each child directory becomes its own top-level sub-test.
	coreTestLCMDir = "./testdata/test-lcms/"
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

// ingestCoreLCMFile runs every LedgerCloseMeta in path through all ingestion
// processors against its own isolated test database, so parallel sub-tests
// don't conflict on DB transactions or migration resets.
func ingestCoreLCMFile(t *testing.T, path string) *history.Q {
	t.Helper()
	testDB := dbtest.Postgres(t)
	t.Cleanup(testDB.Close)

	dbConn := testDB.Open()
	t.Cleanup(func() { _ = dbConn.Close() })

	_, err := schema.Migrate(dbConn.DB, schema.MigrateUp, 0)
	require.NoError(t, err, "failed to run migrations")

	historyQ := &history.Q{SessionInterface: &supportdb.Session{DB: dbConn}}

	ledgers := readLedgerCloseMetasFromFile(t, path)
	if len(ledgers) == 0 {
		t.Skipf("no LedgerCloseMeta records in %s, skipping", path)
	}
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
	return historyQ
}

// TestCoreLCMIngestion walks every child directory of coreTestLCMDir, reads
// every .xdr file in each directory, decodes framed LedgerCloseMeta records,
// and runs Horizon's ingestion processors against an isolated test database
// to verify that ingestion succeeds without errors.
//
// The test exercises:
//   - XDR decoding of LedgerCloseMeta streams
//   - Extraction of ledger entry changes (via change readers)
//   - Extraction of transactions (via transaction readers)
func TestCoreLCMIngestion(t *testing.T) {
	// Walk every child directory under coreTestLCMDir.  Each child
	// becomes a top-level sub-test, and every .xdr file inside it
	// becomes a nested sub-test.
	topEntries, err := os.ReadDir(coreTestLCMDir)
	require.NoError(t, err, "cannot read LCM test directory %s", coreTestLCMDir)
	require.NotEmpty(t, topEntries, "no entries found in %s", coreTestLCMDir)

	for _, dirEntry := range topEntries {
		if !dirEntry.IsDir() {
			continue
		}

		dirName := dirEntry.Name()
		dirPath := filepath.Join(coreTestLCMDir, dirName)

		t.Run(dirName, func(t *testing.T) {
			files, err := os.ReadDir(dirPath)
			require.NoError(t, err, "cannot read directory %s", dirPath)

			for _, fileEntry := range files {
				if fileEntry.IsDir() || filepath.Ext(fileEntry.Name()) != ".xdr" {
					continue
				}

				t.Run(fileEntry.Name(), func(t *testing.T) {
					t.Parallel()
					ingestCoreLCMFile(t, filepath.Join(dirPath, fileEntry.Name()))
				})
			}
		})
	}
}

// TestCoreLCMMuxedContractDestination checks that SAC transfers and mints to a
// muxed contract address (CAP-0084) keep the destination's muxed id, both in
// the operation's asset_balance_changes and on the contract_credited effect.
func TestCoreLCMMuxedContractDestination(t *testing.T) {
	const (
		sender   = "GCE4HENKZ3ZIHQY4VEYCVX5ZE5LNDIN3FH4MHZCWFKXQZGQIOGAO77CN"
		contract = "CAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWA3Y"
	)
	type balanceChange struct {
		ledger        int32
		changeType    string
		from          string
		muxedID       string
		muxedContract string
	}
	transfer := balanceChange{23, "transfer", sender, "987654321987654321",
		"WAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWDNU3JPX55ASWEDNS"}
	mint := balanceChange{25, "mint", "", "111222333444555666",
		"WAA3QKIP2SNVXUJTB4HKOGF55JTSSMQGED3FZYNHMNSXYV3DRRMAWAMLEQPXYDMDSLJLI"}
	for file, expected := range map[string][]balanceChange{
		"8fe0b7272f3a072f.xdr": {transfer, mint},
		"938fb779c48ac3af.xdr": {transfer},
	} {
		t.Run(file, func(t *testing.T) {
			historyQ := ingestCoreLCMFile(t, filepath.Join(coreTestLCMDir, "InvokeHostFunctionTests", file))
			ctx := context.Background()
			for _, want := range expected {
				ops, _, err := historyQ.Operations().ForLedger(ctx, want.ledger).Fetch(ctx)
				require.NoError(t, err)
				require.Len(t, ops, 1)

				var details struct {
					AssetBalanceChanges []map[string]interface{} `json:"asset_balance_changes"`
				}
				require.NoError(t, ops[0].UnmarshalDetails(&details))
				require.Len(t, details.AssetBalanceChanges, 1)
				change := details.AssetBalanceChanges[0]
				require.Equal(t, want.changeType, change["type"])
				require.Equal(t, contract, change["to"])
				require.Equal(t, want.muxedID, change["destination_muxed_id"])
				if want.from == "" {
					require.NotContains(t, change, "from")
				} else {
					require.Equal(t, want.from, change["from"])
				}

				ledgerEffects, err := historyQ.EffectsForLedger(ctx, want.ledger,
					db2.PageQuery{Order: db2.OrderAscending, Limit: 10})
				require.NoError(t, err)
				var credits []history.Effect
				for _, effect := range ledgerEffects {
					if effect.Type == history.EffectContractCredited {
						credits = append(credits, effect)
					}
				}
				require.Len(t, credits, 1)
				// Decode the way the /effects endpoint does.
				var credit effects.ContractCredited
				require.NoError(t, credits[0].UnmarshalDetails(&credit))
				require.Equal(t, contract, credit.Contract)
				require.Equal(t, want.muxedContract, credit.ContractMuxed)
				require.Equal(t, want.muxedID, strconv.FormatUint(credit.ContractMuxedID, 10))
			}
		})
	}
}
