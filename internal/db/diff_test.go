package db

import (
	"testing"
)

func TestComputeDiff_AddedRow(t *testing.T) {
	before := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
		},
	}
	after := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		},
	}

	diffs := ComputeDiff(before, after)

	userDiff, ok := diffs["users"]
	if !ok {
		t.Fatal("expected users diff")
	}
	if len(userDiff.Added) != 1 {
		t.Errorf("expected 1 added row, got %d", len(userDiff.Added))
	}
	if len(userDiff.Removed) != 0 {
		t.Errorf("expected 0 removed rows, got %d", len(userDiff.Removed))
	}
}

func TestComputeDiff_RemovedRow(t *testing.T) {
	before := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
			{"id": 2, "name": "Bob"},
		},
	}
	after := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
		},
	}

	diffs := ComputeDiff(before, after)

	userDiff := diffs["users"]
	if len(userDiff.Removed) != 1 {
		t.Errorf("expected 1 removed row, got %d", len(userDiff.Removed))
	}
}

func TestComputeDiff_ModifiedRow(t *testing.T) {
	before := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
		},
	}
	after := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice Updated"},
		},
	}

	diffs := ComputeDiff(before, after)

	userDiff := diffs["users"]
	if len(userDiff.Modified) != 1 {
		t.Errorf("expected 1 modified row, got %d", len(userDiff.Modified))
	}
}

func TestComputeDiff_NoChanges(t *testing.T) {
	state := map[string][]map[string]any{
		"users": {
			{"id": 1, "name": "Alice"},
		},
	}

	diffs := ComputeDiff(state, state)

	userDiff := diffs["users"]
	if len(userDiff.Added) != 0 || len(userDiff.Removed) != 0 || len(userDiff.Modified) != 0 {
		t.Error("expected no changes")
	}
}

func TestComputeDiff_NewTable(t *testing.T) {
	before := map[string][]map[string]any{}
	after := map[string][]map[string]any{
		"orders": {
			{"id": 1, "total": 99.99},
		},
	}

	diffs := ComputeDiff(before, after)

	orderDiff, ok := diffs["orders"]
	if !ok {
		t.Fatal("expected orders diff")
	}
	if len(orderDiff.Added) != 1 {
		t.Errorf("expected 1 added row in new table, got %d", len(orderDiff.Added))
	}
}

func TestComputeDiff_EmptyStates(t *testing.T) {
	before := map[string][]map[string]any{
		"users": {},
	}
	after := map[string][]map[string]any{
		"users": {},
	}

	diffs := ComputeDiff(before, after)

	userDiff := diffs["users"]
	if len(userDiff.Added) != 0 || len(userDiff.Removed) != 0 {
		t.Error("expected no diffs for empty tables")
	}
}
