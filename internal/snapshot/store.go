package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Store handles reading and writing snapshots to disk.
type Store struct {
	BaseDir string
	Format  string // "json" or "yaml"
}

// NewStore creates a new Store.
func NewStore(baseDir, format string) *Store {
	return &Store{BaseDir: baseDir, Format: format}
}

// Save writes a snapshot to disk, organized by service and endpoint.
func (s *Store) Save(snap *Snapshot) (string, error) {
	dir := s.dirForSnapshot(snap)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating snapshot directory: %w", err)
	}

	// Determine next sequence number
	seq, err := s.nextSeqNumber(dir)
	if err != nil {
		return "", err
	}

	ext := s.extension()
	slug := sanitizeForFilename(snap.ID)
	filename := fmt.Sprintf("%03d_%s.snapshot.%s", seq, slug, ext)
	path := filepath.Join(dir, filename)

	data, err := s.marshal(snap)
	if err != nil {
		return "", fmt.Errorf("marshaling snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("writing snapshot file: %w", err)
	}

	return path, nil
}

// Load reads a snapshot from a specific file path.
func (s *Store) Load(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot file: %w", err)
	}

	snap := &Snapshot{}
	if err := s.unmarshal(data, snap); err != nil {
		return nil, fmt.Errorf("parsing snapshot file: %w", err)
	}

	return snap, nil
}

// LoadAll reads all snapshots under the base directory.
func (s *Store) LoadAll() ([]*Snapshot, []string, error) {
	var snapshots []*Snapshot
	var paths []string

	err := filepath.Walk(s.BaseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".snapshot.json") && !strings.HasSuffix(path, ".snapshot.yaml") && !strings.HasSuffix(path, ".snapshot.yml") {
			return nil
		}

		snap, err := s.Load(path)
		if err != nil {
			return fmt.Errorf("loading %s: %w", path, err)
		}
		snapshots = append(snapshots, snap)
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return snapshots, paths, nil
}

// LoadByTag loads all snapshots that have at least one of the given tags.
func (s *Store) LoadByTag(tags []string) ([]*Snapshot, []string, error) {
	all, allPaths, err := s.LoadAll()
	if err != nil {
		return nil, nil, err
	}

	tagSet := make(map[string]bool)
	for _, t := range tags {
		tagSet[t] = true
	}

	var filtered []*Snapshot
	var filteredPaths []string
	for i, snap := range all {
		for _, t := range snap.Tags {
			if tagSet[t] {
				filtered = append(filtered, snap)
				filteredPaths = append(filteredPaths, allPaths[i])
				break
			}
		}
	}
	return filtered, filteredPaths, nil
}

// Update replaces a snapshot file with an updated snapshot.
func (s *Store) Update(path string, snap *Snapshot) error {
	data, err := s.marshal(snap)
	if err != nil {
		return fmt.Errorf("marshaling snapshot: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// List returns metadata about all snapshots.
func (s *Store) List() ([]SnapshotInfo, error) {
	all, paths, err := s.LoadAll()
	if err != nil {
		return nil, err
	}

	infos := make([]SnapshotInfo, len(all))
	for i, snap := range all {
		infos[i] = SnapshotInfo{
			ID:        snap.ID,
			Path:      paths[i],
			Service:   snap.Service,
			Method:    snap.Request.Method,
			URL:       snap.Request.URL,
			Status:    snap.Response.Status,
			Tags:      snap.Tags,
			Timestamp: snap.Timestamp,
		}
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Path < infos[j].Path
	})

	return infos, nil
}

// SnapshotInfo is a summary of a snapshot for listing.
type SnapshotInfo struct {
	ID        string   `json:"id"`
	Path      string   `json:"path"`
	Service   string   `json:"service"`
	Method    string   `json:"method"`
	URL       string   `json:"url"`
	Status    int      `json:"status"`
	Tags      []string `json:"tags"`
	Timestamp interface{}
}

func (s *Store) dirForSnapshot(snap *Snapshot) string {
	endpoint := fmt.Sprintf("%s_%s", snap.Request.Method, sanitizeForFilename(snap.Request.URL))
	return filepath.Join(s.BaseDir, sanitizeForFilename(snap.Service), endpoint)
}

func (s *Store) nextSeqNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1, nil
	}
	max := 0
	for _, e := range entries {
		name := e.Name()
		if len(name) >= 3 {
			var n int
			if _, err := fmt.Sscanf(name, "%03d_", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return max + 1, nil
}

func (s *Store) extension() string {
	if s.Format == "yaml" || s.Format == "yml" {
		return "yaml"
	}
	return "json"
}

func (s *Store) marshal(snap *Snapshot) ([]byte, error) {
	if s.Format == "yaml" || s.Format == "yml" {
		return yaml.Marshal(snap)
	}
	return json.MarshalIndent(snap, "", "  ")
}

func (s *Store) unmarshal(data []byte, snap *Snapshot) error {
	// Try JSON first, then YAML
	if err := json.Unmarshal(data, snap); err == nil {
		return nil
	}
	return yaml.Unmarshal(data, snap)
}

func sanitizeForFilename(s string) string {
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, ":", "_")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.TrimLeft(s, "_")
	if s == "" {
		s = "root"
	}
	return s
}
