package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func inspectEntries(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()

	rawEntries, ok := data["entries"].([]any)
	require.True(t, ok, "entries should be an array")

	entries := make([]map[string]any, 0, len(rawEntries))
	for _, rawEntry := range rawEntries {
		entry, ok := rawEntry.(map[string]any)
		require.True(t, ok, "entry should be an object")
		entries = append(entries, entry)
	}

	return entries
}

func inspectEntryByBaseName(t *testing.T, data map[string]any, baseName string) map[string]any {
	t.Helper()

	for _, entry := range inspectEntries(t, data) {
		source, ok := entry["source"].(map[string]any)
		require.True(t, ok, "entry.source should be an object")
		rawPath, ok := source["raw"].(string)
		require.True(t, ok, "entry.source.raw should be a string")
		if filepath.Base(rawPath) == baseName {
			return entry
		}
	}

	t.Fatalf("entry with base name %q not found", baseName)
	return nil
}

func inspectFindings(t *testing.T, entry map[string]any) []map[string]any {
	t.Helper()

	rawFindings, ok := entry["findings"].([]any)
	require.True(t, ok, "findings should be an array")

	findings := make([]map[string]any, 0, len(rawFindings))
	for _, rawFinding := range rawFindings {
		finding, ok := rawFinding.(map[string]any)
		require.True(t, ok, "finding should be an object")
		findings = append(findings, finding)
	}

	return findings
}
