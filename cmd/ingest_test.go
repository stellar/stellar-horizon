package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	horizon "github.com/stellar/go/services/horizon/internal"
	"github.com/stellar/go/services/horizon/internal/ingest"
	"github.com/stellar/go/support/config"
	"github.com/stellar/go/support/db/dbtest"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IngestCommandsTestSuite struct {
	suite.Suite
	db *dbtest.DB
}

func TestIngestCommandsTestSuite(t *testing.T) {
	ingestCmdSuite := &IngestCommandsTestSuite{}
	suite.Run(t, ingestCmdSuite)
}

func (s *IngestCommandsTestSuite) SetupSuite() {
	// stub out the ingest command execution body,
	// just test the cmd argument parsing and validation
	processVerifyRangeFn = func(*horizon.Config, config.ConfigOptions, ingest.StorageBackendConfig) error {
		return nil
	}
	s.db = dbtest.Postgres(s.T())
	RootCmd.SetArgs([]string{
		"db", "migrate", "up", "--db-url", s.db.DSN})
	require.NoError(s.T(), RootCmd.Execute())
}

func (s *IngestCommandsTestSuite) TearDownSuite() {
	s.db.Close()
}

func newIngestCmd() *cobra.Command {
	rootCmd, horizonConfig, horizonFlags := newRootBaseCmd()
	DefineIngestCommands(rootCmd, horizonConfig, horizonFlags)
	return rootCmd
}

func (s *IngestCommandsTestSuite) TestIngestVerifyRangeCmd() {
	tests := []struct {
		name         string
		args         []string
		expectError  bool
		errorMessage string
	}{
		{
			name: "datastore backend without config",
			args: []string{
				"--from", "1", "--to", "10",
				"--network", "testnet",
				"--ledgerbackend", "datastore",
			},
			expectError:  true,
			errorMessage: "datastore-config file path is required with datastore backend",
		},
		{
			name: "invalid ledgerbackend type",
			args: []string{
				"--from", "1", "--to", "10",
				"--network", "testnet",
				"--ledgerbackend", "invalid-backend",
			},
			expectError:  true,
			errorMessage: "invalid ledger backend: invalid-backend, must be 'captive-core' or 'datastore'",
		},
		{
			name: "datastore with config",
			args: []string{
				"--from", "1", "--to", "10",
				"--network", "testnet",
				"--datastore-config", "../internal/ingest/testdata/config.storagebackend.toml",
				"--ledgerbackend", "datastore",
			},
			expectError: false,
		},
		{
			name: "datastore backend with missing config file",
			args: []string{
				"--from", "1", "--to", "10",
				"--network", "testnet",
				"--ledgerbackend", "datastore",
				"--datastore-config", "nonexistent-config.toml",
			},
			expectError:  true,
			errorMessage: "failed to load datastore ledgerbackend config file",
		},
	}

	for _, tt := range tests {
		s.T().Run(tt.name, func(t *testing.T) {
			rootCmd := newIngestCmd()
			args := append([]string{"ingest", "verify-range"}, tt.args...)
			rootCmd.SetArgs(append([]string{
				"--db-url", s.db.DSN,
				"--stellar-core-binary-path", "/test/core/bin/path",
			}, args...))

			if tt.expectError {
				err := rootCmd.Execute()
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errorMessage)
			} else {
				require.NoError(t, rootCmd.Execute())
			}
		})
	}
}
