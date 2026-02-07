package reporter

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/esse/snapshot-tester/internal/asserter"
	"github.com/esse/snapshot-tester/internal/replayer"
)

// Format represents an output format.
type Format string

const (
	FormatText  Format = "text"
	FormatJUnit Format = "junit"
	FormatTAP   Format = "tap"
	FormatJSON  Format = "json"
)

// Report generates a test report in the specified format.
func Report(results []replayer.TestResult, format Format) (string, error) {
	switch format {
	case FormatText:
		return reportText(results), nil
	case FormatJUnit:
		return reportJUnit(results)
	case FormatTAP:
		return reportTAP(results), nil
	case FormatJSON:
		return reportJSON(results)
	default:
		return reportText(results), nil
	}
}

func reportText(results []replayer.TestResult) string {
	var sb strings.Builder
	passed, failed, errored := 0, 0, 0

	for _, r := range results {
		if r.Error != "" {
			errored++
			sb.WriteString(fmt.Sprintf("ERROR %s (%s)\n", r.SnapshotPath, r.Duration))
			sb.WriteString(fmt.Sprintf("  %s\n\n", r.Error))
		} else if r.Passed {
			passed++
			sb.WriteString(fmt.Sprintf("PASS  %s (%s)\n", r.SnapshotPath, r.Duration))
		} else {
			failed++
			sb.WriteString(fmt.Sprintf("FAIL  %s (%s)\n", r.SnapshotPath, r.Duration))
			sb.WriteString(asserter.FormatDiffs(r.Diffs))
			sb.WriteString("\n")
		}
	}

	sb.WriteString(fmt.Sprintf("\nResults: %d passed, %d failed, %d errors, %d total\n",
		passed, failed, errored, len(results)))

	return sb.String()
}

// JUnit XML types
type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName  xml.Name        `xml:"testsuite"`
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitError   `xml:"error,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func reportJUnit(results []replayer.TestResult) (string, error) {
	failures, errors := 0, 0
	var cases []junitTestCase

	for _, r := range results {
		tc := junitTestCase{
			Name: r.SnapshotPath,
			Time: fmt.Sprintf("%.3f", r.Duration.Seconds()),
		}

		if r.Error != "" {
			errors++
			tc.Error = &junitError{
				Message: r.Error,
				Body:    r.Error,
			}
		} else if !r.Passed {
			failures++
			tc.Failure = &junitFailure{
				Message: fmt.Sprintf("%d differences found", len(r.Diffs)),
				Body:    asserter.FormatDiffs(r.Diffs),
			}
		}

		cases = append(cases, tc)
	}

	suites := junitTestSuites{
		Suites: []junitTestSuite{
			{
				Name:     "snapshot-tests",
				Tests:    len(results),
				Failures: failures,
				Errors:   errors,
				Cases:    cases,
			},
		},
	}

	data, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return "", err
	}

	return xml.Header + string(data), nil
}

func reportTAP(results []replayer.TestResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("TAP version 13\n1..%d\n", len(results)))

	for i, r := range results {
		num := i + 1
		if r.Error != "" {
			sb.WriteString(fmt.Sprintf("not ok %d - %s\n", num, r.SnapshotPath))
			sb.WriteString(fmt.Sprintf("  ---\n  error: %s\n  ...\n", r.Error))
		} else if r.Passed {
			sb.WriteString(fmt.Sprintf("ok %d - %s\n", num, r.SnapshotPath))
		} else {
			sb.WriteString(fmt.Sprintf("not ok %d - %s\n", num, r.SnapshotPath))
			sb.WriteString("  ---\n")
			for _, d := range r.Diffs {
				sb.WriteString(fmt.Sprintf("  - path: %s\n    message: %s\n", d.Path, d.Message))
			}
			sb.WriteString("  ...\n")
		}
	}

	return sb.String()
}

func reportJSON(results []replayer.TestResult) (string, error) {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
