package cli

import (
	"github.com/esse/snapshot-tester/internal/config"
	dbpkg "github.com/esse/snapshot-tester/internal/db"
	"github.com/esse/snapshot-tester/internal/httpclient"
	"github.com/esse/snapshot-tester/internal/snapshot"
)

func newSnapshotterForUpdate(cfg *config.Config, connStr string) (dbpkg.Snapshotter, error) {
	return dbpkg.NewSnapshotter(cfg.Database.Type, connStr, cfg.Database.Tables, cfg.Database.Namespaces)
}

func fireRequestForUpdate(cfg *config.Config, req snapshot.Request) (*snapshot.Response, error) {
	return httpclient.FireRequest(cfg.Service.BaseURL, req, cfg.Replay.TimeoutMs)
}

func computeDiffForUpdate(before, after map[string][]map[string]any) map[string]snapshot.TableDiff {
	return dbpkg.ComputeDiff(before, after)
}
