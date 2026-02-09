package cli

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/esse/snapshot-tester/internal/asserter"
	"github.com/esse/snapshot-tester/internal/config"
	"github.com/esse/snapshot-tester/internal/logger"
	"github.com/esse/snapshot-tester/internal/recorder"
	"github.com/esse/snapshot-tester/internal/replayer"
	"github.com/esse/snapshot-tester/internal/reporter"
	"github.com/esse/snapshot-tester/internal/security"
	"github.com/esse/snapshot-tester/internal/snapshot"
	"github.com/spf13/cobra"
)

// Execute runs the CLI.
func Execute() {
	var logLevel string

	root := &cobra.Command{
		Use:   "snapshot-tester",
		Short: "Record and replay service interactions for deterministic integration testing",
		Long: `Service Snapshot Tester records the full lifecycle of HTTP requests —
including database state before and after — and uses these snapshots to
verify that your service behaves consistently over time.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			logger.Setup(logLevel)
		},
	}

	root.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Log level: debug, info, warn, error")

	root.AddCommand(
		newRecordCmd(),
		newReplayCmd(),
		newListCmd(),
		newDiffCmd(),
		newUpdateCmd(),
		newProxyCmd(),
	)

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}

func newRecordCmd() *cobra.Command {
	var (
		configPath string
		tags       []string
	)

	cmd := &cobra.Command{
		Use:   "record",
		Short: "Start the recording proxy to capture snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate config path for security
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}
			
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			rec, err := recorder.New(cfg, tags)
			if err != nil {
				return fmt.Errorf("creating recorder: %w", err)
			}
			defer rec.Close()

			return rec.Start()
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")
	cmd.Flags().StringSliceVarP(&tags, "tag", "t", nil, "Tags to apply to recorded snapshots")

	return cmd
}

func newReplayCmd() *cobra.Command {
	var (
		configPath   string
		snapshotPath string
		tag          string
		ci           bool
		outputFormat string
	)

	cmd := &cobra.Command{
		Use:   "replay",
		Short: "Replay snapshots against the service and verify behavior",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate config path for security
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}
			
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			store := snapshot.NewStore(cfg.Recording.SnapshotDir, cfg.Recording.Format)

			var snapshots []*snapshot.Snapshot
			var paths []string

			if snapshotPath != "" {
				// Validate snapshot path for security
				if err := security.ValidateSnapshotPath(snapshotPath, cfg.Recording.SnapshotDir); err != nil {
					return fmt.Errorf("invalid snapshot path: %w", err)
				}
				
				// Replay single snapshot
				snap, err := store.Load(snapshotPath)
				if err != nil {
					return fmt.Errorf("loading snapshot: %w", err)
				}
				snapshots = []*snapshot.Snapshot{snap}
				paths = []string{snapshotPath}
			} else if tag != "" {
				// Replay by tag
				snapshots, paths, err = store.LoadByTag(strings.Split(tag, ","))
				if err != nil {
					return fmt.Errorf("loading snapshots by tag: %w", err)
				}
			} else {
				// Replay all
				snapshots, paths, err = store.LoadAll()
				if err != nil {
					return fmt.Errorf("loading snapshots: %w", err)
				}
			}

			if len(snapshots) == 0 {
				fmt.Println("No snapshots found.")
				return nil
			}

			fmt.Printf("Replaying %d snapshot(s)...\n\n", len(snapshots))

			rep, err := replayer.New(cfg)
			if err != nil {
				return fmt.Errorf("creating replayer: %w", err)
			}
			defer rep.Close()

			results := rep.ReplayAll(snapshots, paths)

			// Determine output format
			format := reporter.FormatText
			if ci {
				format = reporter.FormatJUnit
			}
			if outputFormat != "" {
				switch reporter.Format(outputFormat) {
				case reporter.FormatJUnit:
					format = reporter.FormatJUnit
				case reporter.FormatTAP:
					format = reporter.FormatTAP
				case reporter.FormatJSON:
					format = reporter.FormatJSON
				default:
					format = reporter.FormatText
				}
			}

			output, err := reporter.Report(results, format)
			if err != nil {
				return fmt.Errorf("generating report: %w", err)
			}

			fmt.Print(output)

			// Exit with error code if any tests failed
			for _, r := range results {
				if !r.Passed || r.Error != "" {
					if cfg.Replay.StrictMode {
						os.Exit(1)
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")
	cmd.Flags().StringVarP(&snapshotPath, "snapshot", "s", "", "Path to a specific snapshot file")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Replay snapshots with this tag (comma-separated)")
	cmd.Flags().BoolVar(&ci, "ci", false, "Output in CI-friendly format (JUnit XML)")
	cmd.Flags().StringVarP(&outputFormat, "format", "f", "", "Output format: text, junit, tap, json")

	return cmd
}

func newListCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all recorded snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate config path for security
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}
			
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			store := snapshot.NewStore(cfg.Recording.SnapshotDir, cfg.Recording.Format)
			infos, err := store.List()
			if err != nil {
				return fmt.Errorf("listing snapshots: %w", err)
			}

			if len(infos) == 0 {
				fmt.Println("No snapshots found.")
				return nil
			}

			fmt.Printf("%-12s %-8s %-30s %-6s %s\n", "ID", "METHOD", "URL", "STATUS", "TAGS")
			fmt.Println(strings.Repeat("-", 80))
			for _, info := range infos {
				tags := strings.Join(info.Tags, ", ")
				fmt.Printf("%-12s %-8s %-30s %-6d %s\n",
					info.ID, info.Method, info.URL, info.Status, tags)
			}
			fmt.Printf("\nTotal: %d snapshot(s)\n", len(infos))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")

	return cmd
}

func newDiffCmd() *cobra.Command {
	var (
		configPath   string
		snapshotPath string
	)

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Show the diff for a snapshot replay",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate config path for security
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}
			
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Validate snapshot path for security
			if err := security.ValidateSnapshotPath(snapshotPath, cfg.Recording.SnapshotDir); err != nil {
				return fmt.Errorf("invalid snapshot path: %w", err)
			}

			store := snapshot.NewStore(cfg.Recording.SnapshotDir, cfg.Recording.Format)
			snap, err := store.Load(snapshotPath)
			if err != nil {
				return fmt.Errorf("loading snapshot: %w", err)
			}

			rep, err := replayer.New(cfg)
			if err != nil {
				return fmt.Errorf("creating replayer: %w", err)
			}
			defer rep.Close()

			result := rep.ReplayOne(snap, snapshotPath)

			if result.Error != "" {
				fmt.Printf("ERROR: %s\n", result.Error)
				return nil
			}

			if result.Passed {
				fmt.Println("No differences found. Snapshot matches current behavior.")
			} else {
				fmt.Println(asserter.FormatDiffs(result.Diffs))
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")
	cmd.Flags().StringVarP(&snapshotPath, "snapshot", "s", "", "Path to snapshot file")
	cmd.MarkFlagRequired("snapshot")

	return cmd
}

func newUpdateCmd() *cobra.Command {
	var (
		configPath   string
		snapshotPath string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a snapshot with the current service behavior",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validate config path for security
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}
			
			cfg, err := config.Load(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			// Validate snapshot path for security
			if err := security.ValidateSnapshotPath(snapshotPath, cfg.Recording.SnapshotDir); err != nil {
				return fmt.Errorf("invalid snapshot path: %w", err)
			}

			store := snapshot.NewStore(cfg.Recording.SnapshotDir, cfg.Recording.Format)
			snap, err := store.Load(snapshotPath)
			if err != nil {
				return fmt.Errorf("loading snapshot: %w", err)
			}

			rep, err := replayer.New(cfg)
			if err != nil {
				return fmt.Errorf("creating replayer: %w", err)
			}
			defer rep.Close()

			// Restore DB, fire request, capture new response and DB state
			result := rep.ReplayOne(snap, snapshotPath)
			if result.Error != "" {
				return fmt.Errorf("replay failed: %s", result.Error)
			}

			if result.Passed {
				fmt.Println("Snapshot already matches current behavior. No update needed.")
				return nil
			}

			// Re-run to capture actual state for update
			// We need the actual response and DB state, so we do a fresh capture
			connStr := cfg.Database.ConnectionString
			if cfg.Replay.TestDatabase.ConnectionString != "" {
				connStr = cfg.Replay.TestDatabase.ConnectionString
			}

			snapshotter, err := newSnapshotterForUpdate(cfg, connStr)
			if err != nil {
				return err
			}
			defer snapshotter.Close()

			// Restore, fire, capture
			if err := snapshotter.RestoreAll(snap.DBStateBefore); err != nil {
				return fmt.Errorf("restoring DB: %w", err)
			}

			actualResp, err := fireRequestForUpdate(cfg, snap.Request)
			if err != nil {
				return fmt.Errorf("firing request: %w", err)
			}

			actualDBAfter, err := snapshotter.SnapshotAll()
			if err != nil {
				return fmt.Errorf("snapshotting DB: %w", err)
			}

			// Update snapshot
			snap.Response = *actualResp
			snap.DBStateAfter = actualDBAfter
			snap.DBDiff = computeDiffForUpdate(snap.DBStateBefore, actualDBAfter)

			if err := store.Update(snapshotPath, snap); err != nil {
				return fmt.Errorf("updating snapshot: %w", err)
			}

			fmt.Printf("Updated snapshot: %s\n", snapshotPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")
	cmd.Flags().StringVarP(&snapshotPath, "snapshot", "s", "", "Path to snapshot file")
	cmd.MarkFlagRequired("snapshot")

	return cmd
}

func newProxyCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Start a passthrough proxy without recording snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := security.ValidateConfigPath(configPath); err != nil {
				return fmt.Errorf("invalid config path: %w", err)
			}

			cfg, err := config.LoadForProxy(configPath)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			target, err := url.Parse(cfg.Service.BaseURL)
			if err != nil {
				return fmt.Errorf("parsing service base URL: %w", err)
			}

			proxy := httputil.NewSingleHostReverseProxy(target)

			addr := fmt.Sprintf(":%d", cfg.Recording.ProxyPort)
			slog.Info("passthrough proxy started", "addr", addr, "target", cfg.Service.BaseURL)

			return http.ListenAndServe(addr, proxy)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "snapshot-tester.yml", "Path to config file")

	return cmd
}
