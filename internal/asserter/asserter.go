package asserter

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Result represents the outcome of comparing expected vs actual.
type Result struct {
	Passed     bool
	Diffs      []Diff
	SnapshotID string
	Path       string
}

// Diff describes a single difference.
type Diff struct {
	Path     string `json:"path"`
	Expected any    `json:"expected"`
	Actual   any    `json:"actual"`
	Message  string `json:"message"`
}

// Options configures assertion behavior.
type Options struct {
	IgnoreFields    []string
	OrderInsensitive map[string]bool // table/field paths where array order doesn't matter
}

// AssertResponse compares expected and actual HTTP responses.
func AssertResponse(expected, actual map[string]any, opts *Options) []Diff {
	var diffs []Diff

	// Compare status
	if fmt.Sprintf("%v", expected["status"]) != fmt.Sprintf("%v", actual["status"]) {
		diffs = append(diffs, Diff{
			Path:     "response.status",
			Expected: expected["status"],
			Actual:   actual["status"],
			Message:  "Status code mismatch",
		})
	}

	// Compare body
	bodyDiffs := compareValues("response.body", expected["body"], actual["body"], opts)
	diffs = append(diffs, bodyDiffs...)

	return diffs
}

// AssertDBState compares expected and actual database states.
func AssertDBState(expected, actual map[string][]map[string]any, opts *Options) []Diff {
	var diffs []Diff

	allTables := make(map[string]bool)
	for t := range expected {
		allTables[t] = true
	}
	for t := range actual {
		allTables[t] = true
	}

	for table := range allTables {
		expectedRows := expected[table]
		actualRows := actual[table]

		if expectedRows == nil {
			diffs = append(diffs, Diff{
				Path:    fmt.Sprintf("db.%s", table),
				Actual:  actualRows,
				Message: "Unexpected table in actual DB state",
			})
			continue
		}
		if actualRows == nil {
			diffs = append(diffs, Diff{
				Path:     fmt.Sprintf("db.%s", table),
				Expected: expectedRows,
				Message:  "Table missing from actual DB state",
			})
			continue
		}

		if len(expectedRows) != len(actualRows) {
			diffs = append(diffs, Diff{
				Path:     fmt.Sprintf("db.%s.length", table),
				Expected: len(expectedRows),
				Actual:   len(actualRows),
				Message:  fmt.Sprintf("Row count mismatch in table %s", table),
			})
		}

		// Compare row by row (try to match by ID first)
		orderInsensitive := opts != nil && opts.OrderInsensitive != nil && opts.OrderInsensitive[table]
		tableDiffs := compareRowSets(fmt.Sprintf("db.%s", table), expectedRows, actualRows, orderInsensitive, opts)
		diffs = append(diffs, tableDiffs...)
	}

	return diffs
}

func compareRowSets(basePath string, expected, actual []map[string]any, orderInsensitive bool, opts *Options) []Diff {
	var diffs []Diff

	if orderInsensitive {
		// Match by best effort (try ID-based matching)
		expectedByID := indexRows(expected)
		actualByID := indexRows(actual)

		if expectedByID != nil && actualByID != nil {
			for id, eRow := range expectedByID {
				aRow, ok := actualByID[id]
				if !ok {
					diffs = append(diffs, Diff{
						Path:     fmt.Sprintf("%s[id=%s]", basePath, id),
						Expected: eRow,
						Message:  "Row missing from actual",
					})
					continue
				}
				rowDiffs := compareRow(fmt.Sprintf("%s[id=%s]", basePath, id), eRow, aRow, opts)
				diffs = append(diffs, rowDiffs...)
			}
			for id := range actualByID {
				if _, ok := expectedByID[id]; !ok {
					diffs = append(diffs, Diff{
						Path:   fmt.Sprintf("%s[id=%s]", basePath, id),
						Actual: actualByID[id],
						Message: "Unexpected row in actual",
					})
				}
			}
			return diffs
		}
	}

	// Positional comparison
	maxLen := len(expected)
	if len(actual) > maxLen {
		maxLen = len(actual)
	}
	for i := 0; i < maxLen; i++ {
		path := fmt.Sprintf("%s[%d]", basePath, i)
		if i >= len(expected) {
			diffs = append(diffs, Diff{
				Path:    path,
				Actual:  actual[i],
				Message: "Extra row in actual",
			})
		} else if i >= len(actual) {
			diffs = append(diffs, Diff{
				Path:     path,
				Expected: expected[i],
				Message:  "Missing row in actual",
			})
		} else {
			rowDiffs := compareRow(path, expected[i], actual[i], opts)
			diffs = append(diffs, rowDiffs...)
		}
	}
	return diffs
}

func compareRow(basePath string, expected, actual map[string]any, opts *Options) []Diff {
	var diffs []Diff
	allKeys := make(map[string]bool)
	for k := range expected {
		allKeys[k] = true
	}
	for k := range actual {
		allKeys[k] = true
	}

	for key := range allKeys {
		path := fmt.Sprintf("%s.%s", basePath, key)
		if opts != nil && isIgnored(path, opts.IgnoreFields) {
			continue
		}

		ev, eOk := expected[key]
		av, aOk := actual[key]
		if !eOk {
			diffs = append(diffs, Diff{
				Path:    path,
				Actual:  av,
				Message: "Unexpected field",
			})
			continue
		}
		if !aOk {
			diffs = append(diffs, Diff{
				Path:     path,
				Expected: ev,
				Message:  "Missing field",
			})
			continue
		}

		fieldDiffs := compareValues(path, ev, av, opts)
		diffs = append(diffs, fieldDiffs...)
	}
	return diffs
}

func compareValues(path string, expected, actual any, opts *Options) []Diff {
	if opts != nil && isIgnored(path, opts.IgnoreFields) {
		return nil
	}

	// Check dynamic matchers
	if s, ok := expected.(string); ok {
		if matchesDynamic(s, actual) {
			return nil
		}
	}

	// Normalize for comparison
	eNorm := normalize(expected)
	aNorm := normalize(actual)

	switch ev := eNorm.(type) {
	case map[string]any:
		av, ok := aNorm.(map[string]any)
		if !ok {
			return []Diff{{Path: path, Expected: expected, Actual: actual, Message: "Type mismatch"}}
		}
		return compareRow(path, ev, av, opts)

	case []any:
		av, ok := aNorm.([]any)
		if !ok {
			return []Diff{{Path: path, Expected: expected, Actual: actual, Message: "Type mismatch"}}
		}
		var diffs []Diff
		maxLen := len(ev)
		if len(av) > maxLen {
			maxLen = len(av)
		}
		if len(ev) != len(av) {
			diffs = append(diffs, Diff{
				Path:     path + ".length",
				Expected: len(ev),
				Actual:   len(av),
				Message:  "Array length mismatch",
			})
		}
		for i := 0; i < maxLen; i++ {
			elemPath := fmt.Sprintf("%s[%d]", path, i)
			if i >= len(ev) {
				diffs = append(diffs, Diff{Path: elemPath, Actual: av[i], Message: "Extra element"})
			} else if i >= len(av) {
				diffs = append(diffs, Diff{Path: elemPath, Expected: ev[i], Message: "Missing element"})
			} else {
				diffs = append(diffs, compareValues(elemPath, ev[i], av[i], opts)...)
			}
		}
		return diffs

	default:
		if fmt.Sprintf("%v", eNorm) != fmt.Sprintf("%v", aNorm) {
			return []Diff{{Path: path, Expected: expected, Actual: actual, Message: "Value mismatch"}}
		}
		return nil
	}
}

// matchesDynamic checks if a value matches a dynamic matcher pattern.
func matchesDynamic(pattern string, actual any) bool {
	switch pattern {
	case "__ANY__":
		return true
	case "__UUID__":
		s, ok := actual.(string)
		if !ok {
			return false
		}
		uuidRegex := regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
		return uuidRegex.MatchString(s)
	case "__ISO_DATE__":
		s, ok := actual.(string)
		if !ok {
			return false
		}
		isoRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2})?`)
		return isoRegex.MatchString(s)
	}
	return false
}

// isIgnored checks if a field path matches any ignore pattern.
func isIgnored(path string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, path) {
			return true
		}
	}
	return false
}

// matchGlob provides simple glob matching where * matches any segment.
func matchGlob(pattern, path string) bool {
	if pattern == path {
		return true
	}
	// Handle *.field pattern (matches any prefix)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // .field
		return strings.HasSuffix(path, suffix)
	}
	// Handle simple wildcard
	if strings.Contains(pattern, "*") {
		regexStr := "^" + strings.ReplaceAll(regexp.QuoteMeta(pattern), `\*`, `.*`) + "$"
		if matched, err := regexp.MatchString(regexStr, path); err == nil {
			return matched
		}
	}
	return false
}

func indexRows(rows []map[string]any) map[string]map[string]any {
	idx := make(map[string]map[string]any)
	for _, row := range rows {
		id, ok := row["id"]
		if !ok {
			return nil
		}
		idx[fmt.Sprintf("%v", id)] = row
	}
	return idx
}

// normalize converts a value to a comparable form by round-tripping through JSON.
func normalize(v any) any {
	if v == nil {
		return nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return v
	}
	return out
}

// FormatDiffs produces a human-readable diff report.
func FormatDiffs(diffs []Diff) string {
	if len(diffs) == 0 {
		return "No differences found."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d difference(s):\n\n", len(diffs)))
	for i, d := range diffs {
		sb.WriteString(fmt.Sprintf("  %d) %s\n", i+1, d.Path))
		sb.WriteString(fmt.Sprintf("     %s\n", d.Message))
		if d.Expected != nil {
			sb.WriteString(fmt.Sprintf("     expected: %v\n", formatValue(d.Expected)))
		}
		if d.Actual != nil {
			sb.WriteString(fmt.Sprintf("     actual:   %v\n", formatValue(d.Actual)))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatValue(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
