package integration

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/historyarchive"
	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/xdr"

	horizoningest "github.com/stellar/stellar-horizon/internal/ingest"
)

// TestGenerateLedgers generates ledgers using stellar-core's apply-load command.
// The generated ledgers can be written to a compressed XDR file for use in load testing.
// It also extracts ledger entry fixtures from the pre-benchmark checkpoint.
//
// Required env vars:
//   - HORIZON_INTEGRATION_TESTS_ENABLED=true
//
// Optional env vars:
//   - HORIZON_INTEGRATION_TESTS_CAPTIVE_CORE_BIN: Path to stellar-core (version >= 24.1.1-2881)
//     (default: looks for "stellar-core" in PATH)
//   - LOADTEST_CORE_CONFIG_PATH: Path to custom apply-load config file
//     (default: testdata/apply-load.cfg)
//   - LOADTEST_OUTPUT_PATH: Destination path for compressed ledger XDR output
//     (default: empty, no file written)
//   - LOADTEST_FIXTURES_PATH: Destination path for compressed fixtures XDR output
//     (default: empty, no file written)
func TestGenerateLedgers(t *testing.T) {
	if os.Getenv("HORIZON_INTEGRATION_TESTS_ENABLED") != "true" {
		t.Skip("HORIZON_INTEGRATION_TESTS_ENABLED not set")
	}

	coreBinaryPath := os.Getenv("HORIZON_INTEGRATION_TESTS_CAPTIVE_CORE_BIN")
	if coreBinaryPath == "" {
		var err error
		coreBinaryPath, err = exec.LookPath("stellar-core")
		require.NoError(t, err)
	}

	// Use custom config if provided, otherwise use default
	configPath := os.Getenv("LOADTEST_CORE_CONFIG_PATH")
	if configPath == "" {
		configPath = "testdata/apply-load.cfg"
	}

	outputPath := os.Getenv("LOADTEST_OUTPUT_PATH")
	fixturesPath := os.Getenv("LOADTEST_FIXTURES_PATH")
	// both output paths need to be set or empty, it doesn't make sense
	// to only have 1 or the other
	require.Equal(t, len(outputPath) == 0, len(fixturesPath) == 0)

	t.Logf("Using stellar-core: %s", coreBinaryPath)
	t.Logf("Using config: %s", configPath)

	cfg := parseConfig(t, configPath)

	// Run apply-load
	workDir, preBenchmarkCheckpoint := runApplyLoad(t, coreBinaryPath, configPath, cfg)

	t.Log("Verifying fixtures completeness...")
	metadataPath := filepath.Join(workDir, cfg.MetadataOutputStream)
	verifyFixturesCompleteness(t, workDir, metadataPath, cfg.NetworkPassphrase, preBenchmarkCheckpoint)

	// Stream ledgers to output file, or just count them for verification
	// Only include benchmark ledgers (after the pre-benchmark checkpoint),
	// not setup ledgers which would conflict with the fixtures.
	if outputPath != "" {
		t.Logf("Streaming ledgers to %s", outputPath)
		count := streamLedgersToFile(t, metadataPath, outputPath, preBenchmarkCheckpoint)
		t.Logf("Wrote %d ledgers", count)
		require.NotZero(t, count, "apply-load should generate at least one ledger")
	}

	// Stream fixtures to output file, or just count them for verification
	if fixturesPath != "" {
		t.Logf("Streaming fixtures to %s", fixturesPath)
		count := streamFixturesToFile(t, workDir, cfg.NetworkPassphrase, preBenchmarkCheckpoint, fixturesPath)
		t.Logf("Wrote %d ledger entry fixtures", count)
	}
}

func runApplyLoad(t *testing.T, coreBinaryPath, configPath string, cfg applyLoadConfig) (string, uint32) {
	workDir := t.TempDir()

	// Copy config to work dir (apply-load writes files relative to config location)
	destConfigPath := filepath.Join(workDir, "apply-load.cfg")
	copyFile(t, configPath, destConfigPath)

	// Step 1: Initialize history archive
	newHistCmd := exec.Command(coreBinaryPath, "new-hist", cfg.HistoryArchiveName, "--conf", destConfigPath)
	newHistCmd.Dir = workDir
	output, err := newHistCmd.CombinedOutput()
	require.NoError(t, err)
	t.Logf("Initialized history archive: %s", cfg.HistoryArchiveName)

	// Step 2: Execute stellar-core apply-load
	applyLoadCmd := exec.Command(coreBinaryPath, "apply-load", "--conf", destConfigPath)
	applyLoadCmd.Dir = workDir
	output, err = applyLoadCmd.CombinedOutput()
	require.NoError(t, err)
	t.Logf("apply-load completed")

	// Parse pre-benchmark checkpoint from stellar-core output
	// The config sets LOG_FILE_PATH="" so logs go to stdout
	checkpoint := parsePreBenchmarkCheckpoint(t, string(output))
	t.Logf("Pre-benchmark checkpoint: ledger %d", checkpoint)

	return workDir, checkpoint
}

func parsePreBenchmarkCheckpoint(t *testing.T, output string) uint32 {
	re := regexp.MustCompile(`Published final checkpoint before benchmark: ledger (\d+)`)
	matches := re.FindStringSubmatch(output)
	require.NotNil(t, matches, "could not find 'Published final checkpoint before benchmark' in stellar-core output")

	ledger, err := strconv.ParseUint(matches[1], 10, 32)
	require.NoError(t, err)

	return uint32(ledger)
}

type applyLoadConfig struct {
	NetworkPassphrase    string
	MetadataOutputStream string
	HistoryArchiveName   string
}

func parseConfig(t *testing.T, configPath string) applyLoadConfig {
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	var raw map[string]any
	err = toml.Unmarshal(data, &raw)
	require.NoError(t, err)

	passphrase, ok := raw["NETWORK_PASSPHRASE"].(string)
	require.True(t, ok, "NETWORK_PASSPHRASE not found in config")

	metadataStream, ok := raw["METADATA_OUTPUT_STREAM"].(string)
	require.True(t, ok, "METADATA_OUTPUT_STREAM not found in config")

	history, ok := raw["HISTORY"].(map[string]any)
	require.True(t, ok, "HISTORY section not found in config")
	require.Len(t, history, 1, "expected exactly one history archive in config")

	var archiveName string
	for name := range history {
		archiveName = name
	}

	return applyLoadConfig{
		NetworkPassphrase:    passphrase,
		MetadataOutputStream: metadataStream,
		HistoryArchiveName:   archiveName,
	}
}

func streamLedgersToFile(t *testing.T, metadataPath, outputPath string, preBenchmarkCheckpoint uint32) int {
	inFile, err := os.Open(metadataPath)
	require.NoError(t, err)

	// Note: xdr.Stream closes the underlying reader when ReadOne hits EOF or error
	stream := xdr.NewStream(inFile)

	outFile, err := os.Create(outputPath)
	require.NoError(t, err)
	defer outFile.Close()

	writer, err := zstd.NewWriter(outFile)
	require.NoError(t, err)
	defer writer.Close()

	count := 0
	skipped := 0

	for {
		var ledger xdr.LedgerCloseMeta
		if err := stream.ReadOne(&ledger); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}

		// Skip setup ledgers (ledgers up to and including the pre-benchmark checkpoint).
		// Only include benchmark ledgers which operate on the fixture state.
		if ledger.LedgerSequence() <= preBenchmarkCheckpoint {
			skipped++
			continue
		}

		require.NoError(t, xdr.MarshalFramed(writer, ledger))
		count++
	}

	t.Logf("Wrote %d benchmark ledgers, skipped %d setup ledgers", count, skipped)
	return count
}

func openCheckpointReader(t *testing.T, workDir, networkPassphrase string, checkpointLedger uint32) ingest.ChangeReader {
	archivePath := filepath.Join(workDir, "history")
	archive, err := historyarchive.Connect(
		"file://"+archivePath,
		historyarchive.ArchiveOptions{
			NetworkPassphrase: networkPassphrase,
		},
	)
	require.NoError(t, err)

	ctx := context.Background()
	checkpointReader, err := ingest.NewCheckpointChangeReader(ctx, archive, checkpointLedger)
	require.NoError(t, err)

	return checkpointReader
}

func streamFixturesToFile(t *testing.T, workDir, networkPassphrase string, checkpointLedger uint32, outputPath string) int {
	checkpointReader := openCheckpointReader(t, workDir, networkPassphrase, checkpointLedger)
	defer checkpointReader.Close()

	// Compute root account to filter it out (exists in any network with this passphrase)
	rootAccountID := keypair.Root(networkPassphrase).Address()

	outFile, err := os.Create(outputPath)
	require.NoError(t, err)
	defer outFile.Close()

	writer, err := zstd.NewWriter(outFile)
	require.NoError(t, err)
	defer writer.Close()

	count := 0
	skipped := 0
	for {
		change, err := checkpointReader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)

		if change.Post != nil {
			entry := change.Post

			// Skip protocol-level entries that would conflict with existing DB state:
			// 1. Config settings - exist in any network
			// 2. Root account - derived from network passphrase, created at genesis
			if entry.Data.Type == xdr.LedgerEntryTypeConfigSetting {
				skipped++
				continue
			}
			if entry.Data.Type == xdr.LedgerEntryTypeAccount {
				if entry.Data.Account.AccountId.Address() == rootAccountID {
					skipped++
					continue
				}
			}

			require.NoError(t, xdr.MarshalFramed(writer, entry))
			count++
		}
	}

	t.Logf("Wrote %d entries, skipped %d protocol entries", count, skipped)
	return count
}

func copyFile(t *testing.T, src, dst string) {
	data, err := os.ReadFile(src)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, data, 0644))
}

func verifyFixturesCompleteness(t *testing.T, workDir, metadataPath, networkPassphrase string, checkpointLedger uint32) {
	// Step 1: Load all ledger entry keys from fixtures into a set
	knownKeys := make(map[string]bool)
	checkpointReader := openCheckpointReader(t, workDir, networkPassphrase, checkpointLedger)

	fixtureCount := 0
	for {
		change, err := checkpointReader.Read()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if change.Post != nil {
			key, err := change.Post.LedgerKey()
			require.NoError(t, err)
			keyB64, err := key.MarshalBinaryBase64()
			require.NoError(t, err)
			knownKeys[keyB64] = true
			fixtureCount++
		}
	}
	require.NoError(t, checkpointReader.Close())
	t.Logf("Loaded %d fixture keys into verification set", fixtureCount)

	// Step 2: Stream through generated ledgers and verify each change
	file, err := os.Open(metadataPath)
	require.NoError(t, err)
	stream := xdr.NewStream(file)

	ledgerCount := 0
	changeCount := 0
	for {
		var ledger xdr.LedgerCloseMeta
		if err := stream.ReadOne(&ledger); err == io.EOF {
			break
		} else {
			require.NoError(t, err)
		}
		require.True(t, ledger.ProtocolVersion() == horizoningest.MaxSupportedProtocolVersion)
		ledgerCount++

		// Extract changes from this ledger
		changeReader, err := ingest.NewLedgerChangeReaderFromLedgerCloseMeta(networkPassphrase, ledger)
		require.NoError(t, err)

		for {
			change, err := changeReader.Read()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			changeCount++

			// If the change has a Pre state, the entry must already exist in our known set
			if change.Pre != nil {
				key, err := change.Pre.LedgerKey()
				require.NoError(t, err)
				keyB64, err := key.MarshalBinaryBase64()
				require.NoError(t, err)
				require.True(t, knownKeys[keyB64])
			}

			// Update our known set based on the Post state
			if change.Post != nil {
				// Entry exists after this change - add/keep in set
				key, err := change.Post.LedgerKey()
				require.NoError(t, err)
				keyB64, err := key.MarshalBinaryBase64()
				require.NoError(t, err)
				knownKeys[keyB64] = true
			} else if change.Pre != nil {
				// Entry was deleted - remove from set
				key, err := change.Pre.LedgerKey()
				require.NoError(t, err)
				keyB64, err := key.MarshalBinaryBase64()
				require.NoError(t, err)
				delete(knownKeys, keyB64)
			}
		}
		require.NoError(t, changeReader.Close())
	}
}
