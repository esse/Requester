package reporter

import (
	"strings"
	"testing"
	"time"

	"github.com/esse/snapshot-tester/internal/asserter"
	"github.com/esse/snapshot-tester/internal/replayer"
)

func sampleResults() []replayer.TestResult {
	return []replayer.TestResult{
		{
			SnapshotID:   "pass1",
			SnapshotPath: "snapshots/svc/GET_users/001.snapshot.json",
			Passed:       true,
			Duration:     50 * time.Millisecond,
		},
		{
			SnapshotID:   "fail1",
			SnapshotPath: "snapshots/svc/POST_users/001.snapshot.json",
			Passed:       false,
			Duration:     100 * time.Millisecond,
			Diffs: []asserter.Diff{
				{Path: "response.status", Expected: 201, Actual: 500, Message: "Status code mismatch"},
			},
		},
		{
			SnapshotID:   "err1",
			SnapshotPath: "snapshots/svc/DELETE_users/001.snapshot.json",
			Duration:     10 * time.Millisecond,
			Error:        "connection refused",
		},
	}
}

func TestReportText(t *testing.T) {
	output, err := Report(sampleResults(), FormatText)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "PASS") {
		t.Error("expected PASS in text output")
	}
	if !strings.Contains(output, "FAIL") {
		t.Error("expected FAIL in text output")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("expected ERROR in text output")
	}
	if !strings.Contains(output, "1 passed") {
		t.Error("expected '1 passed' in summary")
	}
}

func TestReportJUnit(t *testing.T) {
	output, err := Report(sampleResults(), FormatJUnit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "<?xml") {
		t.Error("expected XML header")
	}
	if !strings.Contains(output, "testsuites") {
		t.Error("expected testsuites element")
	}
	if !strings.Contains(output, "failure") {
		t.Error("expected failure element")
	}
}

func TestReportTAP(t *testing.T) {
	output, err := Report(sampleResults(), FormatTAP)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "TAP version 13") {
		t.Error("expected TAP version header")
	}
	if !strings.Contains(output, "1..3") {
		t.Error("expected test plan 1..3")
	}
	if !strings.Contains(output, "ok 1") {
		t.Error("expected ok 1")
	}
	if !strings.Contains(output, "not ok 2") {
		t.Error("expected not ok 2")
	}
}

func TestReportJSON(t *testing.T) {
	output, err := Report(sampleResults(), FormatJSON)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, `"SnapshotID"`) && !strings.Contains(output, `"snapshotID"`) {
		// Just check it's valid JSON-ish
		if !strings.HasPrefix(output, "[") {
			t.Error("expected JSON array output")
		}
	}
}
