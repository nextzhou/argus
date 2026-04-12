package main

import (
	"path/filepath"
	"testing"
)

func inspectEntries(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()

	rawEntries := mustJSONArray(t, data["entries"])

	entries := make([]map[string]any, 0, len(rawEntries))
	for _, rawEntry := range rawEntries {
		entry := mustJSONObject(t, rawEntry)
		entries = append(entries, entry)
	}

	return entries
}

func inspectEntryByBaseName(t *testing.T, data map[string]any, baseName string) map[string]any {
	t.Helper()

	for _, entry := range inspectEntries(t, data) {
		source := mustJSONObject(t, entry["source"])
		rawPath := mustJSONString(t, source["raw"])
		if filepath.Base(rawPath) == baseName {
			return entry
		}
	}

	t.Fatalf("entry with base name %q not found", baseName)
	return nil
}

func inspectFindings(t *testing.T, entry map[string]any) []map[string]any {
	t.Helper()

	rawFindings := mustJSONArray(t, entry["findings"])

	findings := make([]map[string]any, 0, len(rawFindings))
	for _, rawFinding := range rawFindings {
		finding := mustJSONObject(t, rawFinding)
		findings = append(findings, finding)
	}

	return findings
}
