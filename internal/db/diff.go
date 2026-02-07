package db

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/esse/snapshot-tester/internal/snapshot"
)

// ComputeDiff computes the diff between two database states.
func ComputeDiff(before, after map[string][]map[string]any) map[string]snapshot.TableDiff {
	diffs := make(map[string]snapshot.TableDiff)

	// Process all tables in after state
	allTables := make(map[string]bool)
	for t := range before {
		allTables[t] = true
	}
	for t := range after {
		allTables[t] = true
	}

	for table := range allTables {
		beforeRows := before[table]
		afterRows := after[table]
		diffs[table] = diffTable(beforeRows, afterRows)
	}

	return diffs
}

func diffTable(before, after []map[string]any) snapshot.TableDiff {
	diff := snapshot.TableDiff{
		Added:    []map[string]any{},
		Removed:  []map[string]any{},
		Modified: []snapshot.ModifiedRow{},
	}

	beforeByHash := make(map[string]map[string]any)
	afterByHash := make(map[string]map[string]any)

	// Try to match by primary key first (look for "id" column)
	beforeByID := indexByID(before)
	afterByID := indexByID(after)

	if beforeByID != nil && afterByID != nil {
		// Match by ID
		for id, beforeRow := range beforeByID {
			if afterRow, exists := afterByID[id]; exists {
				if !rowsEqual(beforeRow, afterRow) {
					diff.Modified = append(diff.Modified, snapshot.ModifiedRow{
						Before: beforeRow,
						After:  afterRow,
					})
				}
			} else {
				diff.Removed = append(diff.Removed, beforeRow)
			}
		}
		for id, afterRow := range afterByID {
			if _, exists := beforeByID[id]; !exists {
				diff.Added = append(diff.Added, afterRow)
			}
		}
	} else {
		// Fall back to hash-based matching
		for _, row := range before {
			h := hashRow(row)
			beforeByHash[h] = row
		}
		for _, row := range after {
			h := hashRow(row)
			afterByHash[h] = row
		}

		for h, row := range beforeByHash {
			if _, exists := afterByHash[h]; !exists {
				diff.Removed = append(diff.Removed, row)
			}
		}
		for h, row := range afterByHash {
			if _, exists := beforeByHash[h]; !exists {
				diff.Added = append(diff.Added, row)
			}
		}
	}

	return diff
}

func indexByID(rows []map[string]any) map[string]map[string]any {
	idx := make(map[string]map[string]any)
	for _, row := range rows {
		id, ok := row["id"]
		if !ok {
			return nil // Not all rows have "id"
		}
		key := fmt.Sprintf("%v", id)
		idx[key] = row
	}
	return idx
}

func rowsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if fmt.Sprintf("%v", v) != fmt.Sprintf("%v", bv) {
			return false
		}
	}
	return true
}

func hashRow(row map[string]any) string {
	data, _ := json.Marshal(row)
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h)
}
