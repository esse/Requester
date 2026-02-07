package asserter

import (
	"testing"
)

func TestAssertResponse_Match(t *testing.T) {
	expected := map[string]any{
		"status": 200,
		"body":   map[string]any{"id": float64(1), "name": "Alice"},
	}
	actual := map[string]any{
		"status": 200,
		"body":   map[string]any{"id": float64(1), "name": "Alice"},
	}

	diffs := AssertResponse(expected, actual, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d: %v", len(diffs), diffs)
	}
}

func TestAssertResponse_StatusMismatch(t *testing.T) {
	expected := map[string]any{"status": 200, "body": nil}
	actual := map[string]any{"status": 404, "body": nil}

	diffs := AssertResponse(expected, actual, nil)
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].Path != "response.status" {
		t.Errorf("expected path 'response.status', got %q", diffs[0].Path)
	}
}

func TestAssertResponse_BodyMismatch(t *testing.T) {
	expected := map[string]any{
		"status": 200,
		"body":   map[string]any{"name": "Alice"},
	}
	actual := map[string]any{
		"status": 200,
		"body":   map[string]any{"name": "Bob"},
	}

	diffs := AssertResponse(expected, actual, nil)
	if len(diffs) == 0 {
		t.Fatal("expected diffs for body mismatch")
	}
}

func TestAssertResponse_IgnoreFields(t *testing.T) {
	expected := map[string]any{
		"status": 200,
		"body":   map[string]any{"id": float64(1), "created_at": "2024-01-01"},
	}
	actual := map[string]any{
		"status": 200,
		"body":   map[string]any{"id": float64(1), "created_at": "2024-06-01"},
	}

	opts := &Options{IgnoreFields: []string{"*.created_at"}}
	diffs := AssertResponse(expected, actual, opts)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs with ignored field, got %d: %v", len(diffs), diffs)
	}
}

func TestDynamicMatcher_ANY(t *testing.T) {
	diffs := compareValues("test", "__ANY__", "literally anything", nil)
	if len(diffs) != 0 {
		t.Errorf("__ANY__ should match any value, got %d diffs", len(diffs))
	}
}

func TestDynamicMatcher_UUID(t *testing.T) {
	diffs := compareValues("test", "__UUID__", "550e8400-e29b-41d4-a716-446655440000", nil)
	if len(diffs) != 0 {
		t.Errorf("__UUID__ should match valid UUID, got %d diffs", len(diffs))
	}

	diffs = compareValues("test", "__UUID__", "not-a-uuid", nil)
	if len(diffs) == 0 {
		t.Error("__UUID__ should not match invalid UUID")
	}
}

func TestDynamicMatcher_ISODate(t *testing.T) {
	diffs := compareValues("test", "__ISO_DATE__", "2024-01-15T10:30:00Z", nil)
	if len(diffs) != 0 {
		t.Errorf("__ISO_DATE__ should match ISO date, got %d diffs", len(diffs))
	}

	diffs = compareValues("test", "__ISO_DATE__", "not-a-date", nil)
	if len(diffs) == 0 {
		t.Error("__ISO_DATE__ should not match non-date string")
	}
}

func TestAssertDBState_Match(t *testing.T) {
	state := map[string][]map[string]any{
		"users": {
			{"id": float64(1), "name": "Alice"},
		},
	}

	diffs := AssertDBState(state, state, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs, got %d", len(diffs))
	}
}

func TestAssertDBState_RowCountMismatch(t *testing.T) {
	expected := map[string][]map[string]any{
		"users": {
			{"id": float64(1), "name": "Alice"},
			{"id": float64(2), "name": "Bob"},
		},
	}
	actual := map[string][]map[string]any{
		"users": {
			{"id": float64(1), "name": "Alice"},
		},
	}

	diffs := AssertDBState(expected, actual, nil)
	if len(diffs) == 0 {
		t.Fatal("expected diffs for row count mismatch")
	}
}

func TestAssertDBState_MissingTable(t *testing.T) {
	expected := map[string][]map[string]any{
		"users":  {{"id": float64(1)}},
		"orders": {{"id": float64(1)}},
	}
	actual := map[string][]map[string]any{
		"users": {{"id": float64(1)}},
	}

	diffs := AssertDBState(expected, actual, nil)
	if len(diffs) == 0 {
		t.Fatal("expected diffs for missing table")
	}
}

func TestFormatDiffs(t *testing.T) {
	diffs := []Diff{
		{Path: "response.status", Expected: 200, Actual: 404, Message: "Status code mismatch"},
	}

	output := FormatDiffs(diffs)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if len(output) < 20 {
		t.Errorf("expected detailed output, got %q", output)
	}
}

func TestFormatDiffs_Empty(t *testing.T) {
	output := FormatDiffs(nil)
	if output != "No differences found." {
		t.Errorf("expected 'No differences found.', got %q", output)
	}
}

func TestIsIgnored(t *testing.T) {
	patterns := []string{"*.created_at", "*.updated_at", "response.headers.Date"}

	tests := []struct {
		path    string
		ignored bool
	}{
		{"db.users[0].created_at", true},
		{"response.body.created_at", true},
		{"db.users[0].updated_at", true},
		{"response.headers.Date", true},
		{"db.users[0].name", false},
		{"response.body.id", false},
	}

	for _, tt := range tests {
		got := isIgnored(tt.path, patterns)
		if got != tt.ignored {
			t.Errorf("isIgnored(%q) = %v, want %v", tt.path, got, tt.ignored)
		}
	}
}
