package invariant

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCatalog_SortsValidInvariantsByOrder(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFixture(t, dir, "later.yaml", `version: v0.1.0
id: later
order: 20
check:
  - shell: "true"
prompt: "Fix it"
`)
	writeCatalogFixture(t, dir, "earlier.yaml", `version: v0.1.0
id: earlier
order: 10
check:
  - shell: "true"
prompt: "Fix it"
`)

	catalog, err := LoadCatalog(dir, false)
	require.NoError(t, err)
	require.Len(t, catalog.Invariants, 2)
	assert.Equal(t, "earlier", catalog.Invariants[0].ID)
	assert.Equal(t, 10, catalog.Invariants[0].Order)
	assert.Equal(t, "later", catalog.Invariants[1].ID)
	assert.Empty(t, catalog.Issues)
}

func TestLoadCatalog_DuplicateOrderRemovesConflictingEntries(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFixture(t, dir, "first.yaml", `version: v0.1.0
id: first
order: 10
check:
  - shell: "true"
prompt: "Fix it"
`)
	writeCatalogFixture(t, dir, "second.yaml", `version: v0.1.0
id: second
order: 10
check:
  - shell: "true"
prompt: "Fix it"
`)
	writeCatalogFixture(t, dir, "third.yaml", `version: v0.1.0
id: third
order: 20
check:
  - shell: "true"
prompt: "Fix it"
`)

	catalog, err := LoadCatalog(dir, false)
	require.NoError(t, err)
	require.Len(t, catalog.Invariants, 1)
	assert.Equal(t, "third", catalog.Invariants[0].ID)
	require.Len(t, catalog.Issues, 2)
	assert.Equal(t, issueKindDuplicateOrder, catalog.Issues[0].Kind)
	assert.Equal(t, issueKindDuplicateOrder, catalog.Issues[1].Kind)
	require.Len(t, catalog.IssuesForID("first"), 1)
	require.Len(t, catalog.IssuesForID("second"), 1)
}

func TestLoadCatalog_IgnoreUnderscoreMatchesRuntimeBehavior(t *testing.T) {
	dir := t.TempDir()
	writeCatalogFixture(t, dir, "_ignored.yaml", `version: v0.1.0
id: ignored
check:
  - shell: "true"
prompt: "Fix it"
`)
	writeCatalogFixture(t, dir, "valid.yaml", `version: v0.1.0
id: valid
order: 10
check:
  - shell: "true"
prompt: "Fix it"
`)

	catalog, err := LoadCatalog(dir, true)
	require.NoError(t, err)
	require.Len(t, catalog.Invariants, 1)
	assert.Equal(t, "valid", catalog.Invariants[0].ID)
	assert.Empty(t, catalog.Issues)
}

func writeCatalogFixture(t *testing.T, dir string, name string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
}
