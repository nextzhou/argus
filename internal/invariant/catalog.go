package invariant

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nextzhou/argus/internal/core"
)

const (
	issueKindParseError       = "parse_error"
	issueKindSchemaError      = "schema_error"
	issueKindFilenameMismatch = "filename_mismatch"
	issueKindDuplicateID      = "duplicate_id"
	issueKindDuplicateOrder   = "duplicate_order"
)

// Issue describes one invalid invariant definition discovered while scanning a directory.
type Issue struct {
	File    string `json:"file"`
	ID      string `json:"id,omitempty"`
	Path    string `json:"path,omitempty"`
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// String renders the issue in the same file:path message format used by doctor output.
func (i Issue) String() string {
	if i.Path == "" {
		return fmt.Sprintf("%s: %s", i.File, i.Message)
	}
	return fmt.Sprintf("%s:%s %s", i.File, i.Path, i.Message)
}

// Catalog contains the valid invariants in a scope plus configuration issues for invalid ones.
type Catalog struct {
	Invariants []*Invariant `json:"-"`
	Issues     []Issue      `json:"issues"`

	byID       map[string]*Invariant
	issuesByID map[string][]Issue
}

// EmptyCatalog returns an initialized catalog with no invariants or issues.
func EmptyCatalog() *Catalog {
	return &Catalog{
		Invariants: []*Invariant{},
		Issues:     []Issue{},
		byID:       map[string]*Invariant{},
		issuesByID: map[string][]Issue{},
	}
}

// FindByID returns a valid invariant by ID.
func (c *Catalog) FindByID(id string) (*Invariant, bool) {
	if c == nil {
		return nil, false
	}
	inv, ok := c.byID[id]
	return inv, ok
}

// IssuesForID returns all catalog issues associated with the given invariant ID.
func (c *Catalog) IssuesForID(id string) []Issue {
	if c == nil {
		return nil
	}
	issues := c.issuesByID[id]
	if len(issues) == 0 {
		return nil
	}
	return append([]Issue(nil), issues...)
}

// HasIssues reports whether the catalog contains any invalid invariant definitions.
func (c *Catalog) HasIssues() bool {
	return c != nil && len(c.Issues) > 0
}

type scanOptions struct {
	ignoreUnderscore bool
}

type scannedEntry struct {
	file   string
	inv    *Invariant
	issues []Issue
}

type scannedDirectory struct {
	entries []*scannedEntry
}

// LoadCatalog scans the invariant directory and returns valid ordered invariants plus issues.
func LoadCatalog(dir string, ignoreUnderscore bool) (*Catalog, error) {
	scanned, err := scanInvariantDirectory(dir, scanOptions{ignoreUnderscore: ignoreUnderscore})
	if err != nil {
		return nil, err
	}
	return buildCatalog(scanned), nil
}

func scanInvariantDirectory(dir string, options scanOptions) (*scannedDirectory, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading invariant directory %q: %w", dir, err)
	}

	scanned := &scannedDirectory{
		entries: make([]*scannedEntry, 0, len(entries)),
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".yaml") {
			continue
		}
		if options.ignoreUnderscore && strings.HasPrefix(name, "_") {
			continue
		}

		scannedEntry := &scannedEntry{file: name}
		scanned.entries = append(scanned.entries, scannedEntry)

		fullPath := filepath.Join(dir, name)
		//nolint:gosec // fullPath is built from os.ReadDir results within the requested invariant directory.
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			addIssue(scannedEntry, Issue{
				File:    name,
				Kind:    issueKindParseError,
				Message: fmt.Sprintf("reading invariant file: %v", readErr),
			})
			continue
		}

		inv, decodeErr := decodeInvariant(strings.NewReader(string(data)))
		if decodeErr != nil {
			addIssue(scannedEntry, Issue{
				File:    name,
				Kind:    issueKindParseError,
				Message: decodeErr.Error(),
			})
			continue
		}

		scannedEntry.inv = inv
		for _, fieldErr := range validationErrors(inv) {
			addIssue(scannedEntry, Issue{
				File:    name,
				ID:      inv.ID,
				Path:    fieldErr.Path,
				Kind:    issueKindSchemaError,
				Message: fieldErr.Message,
			})
		}
	}

	applyCrossFileInvariantIssues(scanned)
	return scanned, nil
}

func applyCrossFileInvariantIssues(scanned *scannedDirectory) {
	if scanned == nil {
		return
	}

	idToEntries := make(map[string][]*scannedEntry)
	orderToEntries := make(map[int][]*scannedEntry)

	for _, entry := range scanned.entries {
		if entry == nil || entry.inv == nil {
			continue
		}

		if !core.DefinitionFileNameMatchesID(entry.file, entry.inv.ID) {
			addIssue(entry, Issue{
				File:    entry.file,
				ID:      entry.inv.ID,
				Path:    "id",
				Kind:    issueKindFilenameMismatch,
				Message: core.DefinitionFileNameMismatchMessage("invariant", entry.file, entry.inv.ID),
			})
		}

		if entry.inv.ID != "" {
			idToEntries[entry.inv.ID] = append(idToEntries[entry.inv.ID], entry)
		}
		if entry.inv.Order > 0 {
			orderToEntries[entry.inv.Order] = append(orderToEntries[entry.inv.Order], entry)
		}
	}

	for id, entries := range idToEntries {
		if len(entries) < 2 {
			continue
		}

		files := filesForEntries(entries)
		for _, entry := range entries {
			addIssue(entry, Issue{
				File:    entry.file,
				ID:      id,
				Path:    "id",
				Kind:    issueKindDuplicateID,
				Message: fmt.Sprintf("duplicate invariant ID %q found in files: %s", id, strings.Join(files, ", ")),
			})
		}
	}

	for order, entries := range orderToEntries {
		if len(entries) < 2 {
			continue
		}

		files := filesForEntries(entries)
		for _, entry := range entries {
			addIssue(entry, Issue{
				File:    entry.file,
				ID:      entry.inv.ID,
				Path:    "order",
				Kind:    issueKindDuplicateOrder,
				Message: fmt.Sprintf("duplicate order %d found in files: %s", order, strings.Join(files, ", ")),
			})
		}
	}
}

func buildCatalog(scanned *scannedDirectory) *Catalog {
	catalog := EmptyCatalog()
	if scanned == nil {
		return catalog
	}

	validEntries := make([]*scannedEntry, 0, len(scanned.entries))
	for _, entry := range scanned.entries {
		if entry == nil {
			continue
		}

		sortIssues(entry.issues)
		catalog.Issues = append(catalog.Issues, entry.issues...)
		if entry.inv != nil && len(entry.issues) == 0 {
			validEntries = append(validEntries, entry)
		}
	}

	slices.SortFunc(validEntries, func(a, b *scannedEntry) int {
		if cmp := compareInt(a.inv.Order, b.inv.Order); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.inv.ID, b.inv.ID); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.file, b.file)
	})

	for _, entry := range validEntries {
		catalog.Invariants = append(catalog.Invariants, entry.inv)
		catalog.byID[entry.inv.ID] = entry.inv
	}

	sortIssues(catalog.Issues)
	catalog.issuesByID = buildIssuesByID(catalog.Issues)
	return catalog
}

func buildIssuesByID(issues []Issue) map[string][]Issue {
	issuesByID := make(map[string][]Issue)
	for _, issue := range issues {
		if issue.ID == "" {
			continue
		}
		issuesByID[issue.ID] = append(issuesByID[issue.ID], issue)
	}

	for id := range issuesByID {
		sortIssues(issuesByID[id])
	}

	return issuesByID
}

func addIssue(entry *scannedEntry, issue Issue) {
	if entry == nil {
		return
	}
	entry.issues = append(entry.issues, issue)
}

func filesForEntries(entries []*scannedEntry) []string {
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.file)
	}
	slices.Sort(files)
	return files
}

func sortIssues(issues []Issue) {
	slices.SortFunc(issues, func(a, b Issue) int {
		if cmp := strings.Compare(a.File, b.File); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Path, b.Path); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Kind, b.Kind); cmp != 0 {
			return cmp
		}
		if cmp := strings.Compare(a.Message, b.Message); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
