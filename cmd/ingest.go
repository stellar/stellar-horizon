package cmd

import (
	"context"
	"fmt"
	"go/types"
	"net/http"
	_ "net/http/pprof"
	"time"

	"github.com/go-chi/chi"
	chimiddleware "github.com/go-chi/chi/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/stellar/go/historyarchive"
	horizon "github.com/stellar/go/services/horizon/internal"
	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/httpx"
	"github.com/stellar/go/services/horizon/internal/ingest"
	"github.com/stellar/go/support/config"
	support "github.com/stellar/go/support/config"
	"github.com/stellar/go/support/db"
	"github.com/stellar/go/support/log"
)

var ingestBuildStateSequence uint32
var ingestBuildStateSkipChecks bool
var ingestVerifyFrom, ingestVerifyTo uint32
var ingestVerifyState bool
var ingestVerifyStorageBackendConfigPath string
var ingestVerifyLedgerBackendType ingest.LedgerBackendType
var processVerifyRangeFn = processVerifyRange

var ingestBuildStateCmdOpts = []*support.ConfigOption{
	{
		Name:        "sequence",
		ConfigKey:   &ingestBuildStateSequence,
		OptType:     types.Uint32,
		Required:    true,
		FlagDefault: uint32(0),
		Usage:       "checkpoint ledger sequence",
	},
	{
		Name:        "skip-checks",
		ConfigKey:   &ingestBuildStateSkipChecks,
		OptType:     types.Bool,
		Required:    false,
		FlagDefault: false,
		Usage:       "[optional] set to skip protocol version and bucket list hash verification, can speed up the process because does not require a running Stellar-Core",
	},
}

var ingestVerifyRangeCmdOpts = support.ConfigOptions{
	{
		Name:        "from",
		ConfigKey:   &ingestVerifyFrom,
		OptType:     types.Uint32,
		Required:    true,
		FlagDefault: uint32(0),
		Usage:       "first ledger of the range to ingest",
	},
	{
		Name:        "to",
		ConfigKey:   &ingestVerifyTo,
		OptType:     types.Uint32,
		Required:    true,
		FlagDefault: uint32(0),
		Usage:       "last ledger of the range to ingest",
	},
	{
		Name:        "verify-state",
		ConfigKey:   &ingestVerifyState,
		OptType:     types.Bool,
		Required:    false,
		FlagDefault: false,
		Usage:       "[optional] verifies state at the last ledger of the range when true",
	},
	generateLedgerBackendOpt(&ingestVerifyLedgerBackendType),
	generateDatastoreConfigOpt(&ingestVerifyStorageBackendConfigPath),
}

var ingestionLoadTestLedgersPath string
var ingestionLoadTestCloseDuration time.Duration
var ingestionLoadTestMerge bool
var ingestLoadTestCmdOpts = support.ConfigOptions{
	{
		Name:        "ledgers-path",
		OptType:     types.String,
		FlagDefault: "",
		Required:    true,
		Usage:       "path to ledgers file which will be replayed in the ingestion load test.",
		ConfigKey:   &ingestionLoadTestLedgersPath,
	},
	{
		Name:        "merge",
		ConfigKey:   &ingestionLoadTestMerge,
		OptType:     types.Bool,
		Required:    false,
		FlagDefault: false,
		Usage:       "[optional] set to merge real ledgers with the synthetic ledgers in the ingestion load test.",
	},
	{
		Name:        "close-duration",
		OptType:     types.Float64,
		FlagDefault: 2.0,
		Required:    false,
		CustomSetValue: func(co *support.ConfigOption) error {
			*(co.ConfigKey.(*time.Duration)) = time.Duration(viper.GetFloat64(co.Name)) * time.Second
			return nil
		},
		Usage:     "the time (in seconds) it takes to close ledgers in the ingestion load test.",
		ConfigKey: &ingestionLoadTestCloseDuration,
	},
	generateLedgerBackendOpt(&ledgerBackendType),
	generateDatastoreConfigOpt(&storageBackendConfigPath),
}

var stressTestNumTransactions, stressTestChangesPerTransaction int

var stressTestCmdOpts = []*support.ConfigOption{
	{
		Name:        "transactions",
		ConfigKey:   &stressTestNumTransactions,
		OptType:     types.Int,
		Required:    false,
		FlagDefault: int(1000),
		Usage:       "total number of transactions to ingest (at most 1000)",
	},
	{
		Name:        "changes",
		ConfigKey:   &stressTestChangesPerTransaction,
		OptType:     types.Int,
		Required:    false,
		FlagDefault: int(4000),
		Usage:       "changes per transaction to ingest (at most 4000)",
	},
}

func DefineIngestCommands(rootCmd *cobra.Command, horizonConfig *horizon.Config, horizonFlags config.ConfigOptions) {
	var ingestCmd = &cobra.Command{
		Use:   "ingest",
		Short: "ingestion related commands",
	}

	var ingestVerifyRangeCmd = &cobra.Command{
		Use:   "verify-range",
		Short: "runs ingestion pipeline within a range. warning! requires clean DB.",
		Long:  "runs ingestion pipeline between X and Y sequence number (inclusive)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ingestVerifyRangeCmdOpts.RequireE(); err != nil {
				return err
			}
			if err := ingestVerifyRangeCmdOpts.SetValues(); err != nil {
				return err
			}

			mngr := historyarchive.NewCheckpointManager(horizonConfig.CheckpointFrequency)
			if !mngr.IsCheckpoint(ingestVerifyFrom) && ingestVerifyFrom != 1 {
				return fmt.Errorf("`--from` must be a checkpoint ledger")
			}

			if ingestVerifyState && !mngr.IsCheckpoint(ingestVerifyTo) {
				return fmt.Errorf("`--to` must be a checkpoint ledger when `--verify-state` is set")
			}

			storageBackendConfig := ingest.StorageBackendConfig{}
			noCaptiveCore := false
			if ingestVerifyLedgerBackendType == ingest.BufferedStorageBackend {
				if ingestVerifyStorageBackendConfigPath == "" {
					return fmt.Errorf("datastore-config file path is required with datastore backend")
				}
				var err error
				if storageBackendConfig, err = loadStorageBackendConfig(ingestVerifyStorageBackendConfigPath); err != nil {
					return err
				}
				noCaptiveCore = true
			}

			if err := horizon.ApplyFlags(horizonConfig, horizonFlags, horizon.ApplyOptions{NoCaptiveCore: noCaptiveCore, RequireCaptiveCoreFullConfig: false}); err != nil {
				return err
			}
			err := processVerifyRangeFn(horizonConfig, horizonFlags, storageBackendConfig)
			if err != nil {
				return err
			}

			log.Info("Range run successfully!")
			return nil
		},
	}

	var ingestStressTestCmd = &cobra.Command{
		Use:   "stress-test",
		Short: "runs ingestion pipeline on a ledger with many changes. warning! requires clean DB.",
		Long:  "runs ingestion pipeline on a ledger with many changes. warning! requires clean DB.",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, co := range stressTestCmdOpts {
				if err := co.RequireE(); err != nil {
					return err
				}
				co.SetValue()
			}

			if err := horizon.ApplyFlags(horizonConfig, horizonFlags, horizon.ApplyOptions{RequireCaptiveCoreFullConfig: false}); err != nil {
				return err
			}

			horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
			if err != nil {
				return fmt.Errorf("cannot open Horizon DB: %v", err)
			}

			if stressTestNumTransactions <= 0 {
				return fmt.Errorf("`--transactions` must be positive")
			}

			if stressTestChangesPerTransaction <= 0 {
				return fmt.Errorf("`--changes` must be positive")
			}

			ingestConfig := ingest.Config{
				NetworkPassphrase:      horizonConfig.NetworkPassphrase,
				HistorySession:         horizonSession,
				HistoryArchiveURLs:     horizonConfig.HistoryArchiveURLs,
				HistoryArchiveCaching:  horizonConfig.HistoryArchiveCaching,
				RoundingSlippageFilter: horizonConfig.RoundingSlippageFilter,
				CaptiveCoreBinaryPath:  horizonConfig.CaptiveCoreBinaryPath,
			}

			system, err := ingest.NewSystem(ingestConfig)
			if err != nil {
				return err
			}
			return runWithMetrics(horizonConfig.AdminPort, system, func() error {
				return system.StressTest(
					stressTestNumTransactions,
					stressTestChangesPerTransaction,
				)
			})
		},
	}

	var ingestTriggerStateRebuildCmd = &cobra.Command{
		Use:   "trigger-state-rebuild",
		Short: "updates a database to trigger state rebuild, state will be rebuilt by a running Horizon instance, DO NOT RUN production DB, some endpoints will be unavailable until state is rebuilt",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			if err := horizon.ApplyFlags(horizonConfig, horizonFlags, horizon.ApplyOptions{RequireCaptiveCoreFullConfig: false}); err != nil {
				return err
			}

			horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
			if err != nil {
				return fmt.Errorf("cannot open Horizon DB: %v", err)
			}

			historyQ := &history.Q{SessionInterface: horizonSession}
			if err := historyQ.UpdateIngestVersion(ctx, 0); err != nil {
				return fmt.Errorf("cannot trigger state rebuild: %v", err)
			}

			log.Info("Triggered state rebuild")
			return nil
		},
	}

	var ingestLoadTestRestoreCmd = &cobra.Command{
		Use:   "load-test-restore",
		Short: "restores the horizon db if it is in a dirty state after an interrupted load test",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAndSetFlags(horizonFlags, horizon.DatabaseURLFlagName); err != nil {
				return err
			}

			horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
			if err != nil {
				return fmt.Errorf("cannot open Horizon DB: %v", err)
			}
			defer horizonSession.Close()

			historyQ := &history.Q{SessionInterface: horizonSession}
			if err := ingest.RestoreSnapshot(context.Background(), historyQ); err != nil {
				return fmt.Errorf("cannot restore snapshot: %v", err)
			}

			log.Info("Horizon DB restored")
			return nil
		},
	}

	var ingestBuildStateCmd = &cobra.Command{
		Use:   "build-state",
		Short: "builds state at a given checkpoint. warning! requires clean DB.",
		Long:  "useful for debugging or starting Horizon at specific checkpoint.",
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, co := range ingestBuildStateCmdOpts {
				if err := co.RequireE(); err != nil {
					return err
				}
				co.SetValue()
			}

			if err := horizon.ApplyFlags(horizonConfig, horizonFlags, horizon.ApplyOptions{RequireCaptiveCoreFullConfig: false}); err != nil {
				return err
			}

			horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
			if err != nil {
				return fmt.Errorf("cannot open Horizon DB: %v", err)
			}

			historyQ := &history.Q{SessionInterface: horizonSession}

			lastIngestedLedger, err := historyQ.GetLastLedgerIngestNonBlocking(context.Background())
			if err != nil {
				return fmt.Errorf("cannot get last ledger value: %v", err)
			}

			if lastIngestedLedger != 0 {
				return fmt.Errorf("cannot run on non-empty DB")
			}

			mngr := historyarchive.NewCheckpointManager(globalConfig.CheckpointFrequency)
			if !mngr.IsCheckpoint(ingestBuildStateSequence) {
				return fmt.Errorf("`--sequence` must be a checkpoint ledger")
			}

			ingestConfig := ingest.Config{
				NetworkPassphrase:      horizonConfig.NetworkPassphrase,
				HistorySession:         horizonSession,
				HistoryArchiveURLs:     horizonConfig.HistoryArchiveURLs,
				HistoryArchiveCaching:  horizonConfig.HistoryArchiveCaching,
				CaptiveCoreBinaryPath:  horizonConfig.CaptiveCoreBinaryPath,
				CheckpointFrequency:    horizonConfig.CheckpointFrequency,
				CaptiveCoreToml:        horizonConfig.CaptiveCoreToml,
				CaptiveCoreStoragePath: horizonConfig.CaptiveCoreStoragePath,
				RoundingSlippageFilter: horizonConfig.RoundingSlippageFilter,
			}

			system, err := ingest.NewSystem(ingestConfig)
			if err != nil {
				return err
			}

			return runWithMetrics(horizonConfig.AdminPort, system, func() error {
				return system.BuildState(
					ingestBuildStateSequence,
					ingestBuildStateSkipChecks,
				)
			})
		},
	}

	var ingestLoadTestCmd = &cobra.Command{
		Use:   "load-test",
		Short: "runs an ingestion load test.",
		Long:  "useful for analyzing ingestion performance at configurable transactions per second.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ingestLoadTestCmdOpts.RequireE(); err != nil {
				return err
			}
			if err := ingestLoadTestCmdOpts.SetValues(); err != nil {
				return err
			}

			var err error
			var storageBackendConfig ingest.StorageBackendConfig
			options := horizon.ApplyOptions{RequireCaptiveCoreFullConfig: true}
			if ledgerBackendType == ingest.BufferedStorageBackend {
				if storageBackendConfig, err = loadStorageBackendConfig(storageBackendConfigPath); err != nil {
					return err
				}
				options.NoCaptiveCore = true
			}

			if err = horizon.ApplyFlags(horizonConfig, horizonFlags, options); err != nil {
				return err
			}

			horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
			if err != nil {
				return fmt.Errorf("cannot open Horizon DB: %v", err)
			}
			defer horizonSession.Close()

			if !horizonConfig.IngestDisableStateVerification {
				log.Info("Overriding state verification to be disabled")
			}

			ingestConfig := ingest.Config{
				CaptiveCoreBinaryPath:                horizonConfig.CaptiveCoreBinaryPath,
				CaptiveCoreStoragePath:               horizonConfig.CaptiveCoreStoragePath,
				CaptiveCoreToml:                      horizonConfig.CaptiveCoreToml,
				NetworkPassphrase:                    horizonConfig.NetworkPassphrase,
				HistorySession:                       horizonSession,
				HistoryArchiveURLs:                   horizonConfig.HistoryArchiveURLs,
				HistoryArchiveCaching:                horizonConfig.HistoryArchiveCaching,
				DisableStateVerification:             true,
				ReapLookupTables:                     horizonConfig.ReapLookupTables,
				EnableExtendedLogLedgerStats:         horizonConfig.IngestEnableExtendedLogLedgerStats,
				CheckpointFrequency:                  horizonConfig.CheckpointFrequency,
				StateVerificationCheckpointFrequency: uint32(horizonConfig.IngestStateVerificationCheckpointFrequency),
				StateVerificationTimeout:             horizonConfig.IngestStateVerificationTimeout,
				RoundingSlippageFilter:               horizonConfig.RoundingSlippageFilter,
				SkipTxmeta:                           horizonConfig.SkipTxmeta,
				ReapConfig: ingest.ReapConfig{
					Frequency:      horizonConfig.ReapFrequency,
					RetentionCount: uint32(horizonConfig.HistoryRetentionCount),
					BatchSize:      uint32(horizonConfig.HistoryRetentionReapCount),
				},
				LedgerBackendType:    ledgerBackendType,
				StorageBackendConfig: storageBackendConfig,
			}

			system, err := ingest.NewSystem(ingestConfig)
			if err != nil {
				return err
			}

			return runWithMetrics(horizonConfig.AdminPort, system, func() error {
				return system.LoadTest(
					ingestionLoadTestLedgersPath,
					ingestionLoadTestMerge,
					ingestionLoadTestCloseDuration,
				)
			})
		},
	}

	for _, co := range ingestVerifyRangeCmdOpts {
		err := co.Init(ingestVerifyRangeCmd)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	for _, co := range stressTestCmdOpts {
		err := co.Init(ingestStressTestCmd)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	for _, co := range ingestLoadTestCmdOpts {
		err := co.Init(ingestLoadTestCmd)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	for _, co := range ingestBuildStateCmdOpts {
		err := co.Init(ingestBuildStateCmd)
		if err != nil {
			log.Fatal(err.Error())
		}
	}

	viper.BindPFlags(ingestVerifyRangeCmd.PersistentFlags())
	viper.BindPFlags(ingestBuildStateCmd.PersistentFlags())
	viper.BindPFlags(ingestLoadTestCmd.PersistentFlags())
	viper.BindPFlags(ingestStressTestCmd.PersistentFlags())

	rootCmd.AddCommand(ingestCmd)
	ingestCmd.AddCommand(
		ingestVerifyRangeCmd,
		ingestStressTestCmd,
		ingestTriggerStateRebuildCmd,
		ingestBuildStateCmd,
		ingestLoadTestCmd,
		ingestLoadTestRestoreCmd,
	)
}

func runWithMetrics(metricsPort uint, system ingest.System, f func() error) error {
	if metricsPort != 0 {
		log.Infof("Starting metrics server at: %d", metricsPort)
		mux := chi.NewMux()
		mux.Use(chimiddleware.StripSlashes)
		mux.Use(chimiddleware.RequestID)
		mux.Use(chimiddleware.RequestLogger(&chimiddleware.DefaultLogFormatter{
			Logger:  log.DefaultLogger,
			NoColor: true,
		}))
		registry := prometheus.NewRegistry()
		system.RegisterMetrics(registry)
		httpx.AddMetricRoutes(mux, registry)
		metricsServer := &http.Server{
			Addr:        fmt.Sprintf(":%d", metricsPort),
			Handler:     mux,
			ReadTimeout: 5 * time.Second,
		}
		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("error running metrics server: %v", err)
			}
		}()
		defer func() {
			log.Info("Waiting for metrics to be flushed")
			// by default, the scrape_interval for prometheus is 1 minute
			// so if we sleep for 1.5 minutes we ensure that all remaining metrics
			// will be picked up by the prometheus scraper
			time.Sleep(time.Minute + time.Second*30)
			log.Info("Shutting down metrics server...")
			if err := metricsServer.Shutdown(context.Background()); err != nil {
				log.Warnf("error shutting down metrics server: %v", err)
			}
		}()
	} else {
		log.Info("Metrics server disabled")
	}
	return f()
}

func init() {
	DefineIngestCommands(RootCmd, globalConfig, globalFlags)
}

func processVerifyRange(horizonConfig *horizon.Config, horizonFlags config.ConfigOptions, backendConfig ingest.StorageBackendConfig) error {
	horizonSession, err := db.Open("postgres", horizonConfig.DatabaseURL)
	if err != nil {
		return fmt.Errorf("cannot open Horizon DB: %v", err)
	}

	ingestConfig := ingest.Config{
		NetworkPassphrase:      horizonConfig.NetworkPassphrase,
		HistorySession:         horizonSession,
		HistoryArchiveURLs:     horizonConfig.HistoryArchiveURLs,
		HistoryArchiveCaching:  horizonConfig.HistoryArchiveCaching,
		CaptiveCoreBinaryPath:  horizonConfig.CaptiveCoreBinaryPath,
		CheckpointFrequency:    horizonConfig.CheckpointFrequency,
		CaptiveCoreToml:        horizonConfig.CaptiveCoreToml,
		CaptiveCoreStoragePath: horizonConfig.CaptiveCoreStoragePath,
		RoundingSlippageFilter: horizonConfig.RoundingSlippageFilter,
		LedgerBackendType:      ingestVerifyLedgerBackendType,
		StorageBackendConfig:   backendConfig,
	}

	system, err := ingest.NewSystem(ingestConfig)
	if err != nil {
		return err
	}

	return runWithMetrics(horizonConfig.AdminPort, system, func() error {
		return system.VerifyRange(
			ingestVerifyFrom,
			ingestVerifyTo,
			ingestVerifyState,
		)
	})
}

// generateDatastoreConfigOpt returns a *support.ConfigOption for the datastore-config flag
func generateDatastoreConfigOpt(configKey *string) *support.ConfigOption {
	return &support.ConfigOption{
		Name:      "datastore-config",
		ConfigKey: configKey,
		OptType:   types.String,
		Required:  false,
		Usage:     "[optional] Specify the path to the datastore config file (required for datastore backend)",
	}
}
