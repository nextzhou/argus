package lifecycle

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ChangeSet groups actual filesystem mutations by operation type.
type ChangeSet struct {
	Created []string `json:"created"`
	Updated []string `json:"updated"`
	Removed []string `json:"removed"`
}

// Report describes actual mutations together with the managed path summaries
// that a lifecycle command is responsible for.
type Report struct {
	Changes       ChangeSet `json:"changes"`
	AffectedPaths []string  `json:"affected_paths"`
}

// ProjectOperationResult captures project-scoped lifecycle output data.
type ProjectOperationResult struct {
	Root   string
	Report Report
}

// WorkspaceOperationResult captures workspace-scoped lifecycle output data.
type WorkspaceOperationResult struct {
	Path                    string
	AlreadyRegistered       bool
	ToreDownGlobalResources bool
	Report                  Report
}

// WorkspaceSetupPreview contains the information needed to decide whether an
// setup confirmation prompt is required.
type WorkspaceSetupPreview struct {
	Path              string
	AlreadyRegistered bool
}

// WorkspaceTeardownPreview contains the information needed to decide which
// teardown confirmation prompt to show.
type WorkspaceTeardownPreview struct {
	Path   string
	IsLast bool
}

type mutationTracker struct {
	created map[string]struct{}
	updated map[string]struct{}
	removed map[string]struct{}
}

type summaryRule struct {
	summary string
	match   func(string) bool
}

// summaryProfile defines the user-facing path summaries for one lifecycle
// command family.
//
// These strings are intentionally hard-coded because they are part of the
// public CLI/reporting contract, not just internal implementation details. The
// lifecycle output must expose stable merged summaries such as
// ".argus/{...}/" and ".agents/skills/argus-*", not raw per-file writes.
//
// That summary cannot be derived mechanically from the mutation stream:
//   - multiple concrete writes collapse into one semantic summary
//   - the same concrete path can map to different summaries in different
//     commands
//   - some summaries are intentionally phrased as directories/patterns even
//     when the mutation happened on individual files
//
// Centralizing the hard-coded summaries here keeps the output contract
// explicit, reviewable, and synchronized with docs/tests instead of scattering
// ad-hoc formatting logic across setup/teardown code paths.
type summaryProfile struct {
	affectedPaths []string
	rules         []summaryRule
	fallback      func(string) string
}

func newMutationTracker() *mutationTracker {
	return &mutationTracker{
		created: make(map[string]struct{}),
		updated: make(map[string]struct{}),
		removed: make(map[string]struct{}),
	}
}

func (t *mutationTracker) recordCreated(path string) {
	if t == nil || path == "" {
		return
	}
	delete(t.updated, path)
	delete(t.removed, path)
	t.created[path] = struct{}{}
}

func (t *mutationTracker) recordUpdated(path string) {
	if t == nil || path == "" {
		return
	}
	if _, created := t.created[path]; created {
		return
	}
	delete(t.removed, path)
	t.updated[path] = struct{}{}
}

func (t *mutationTracker) recordRemoved(path string) {
	if t == nil || path == "" {
		return
	}
	delete(t.created, path)
	delete(t.updated, path)
	t.removed[path] = struct{}{}
}

func (t *mutationTracker) buildReport(summary func(string) string, affectedPaths []string) Report {
	if t == nil {
		t = newMutationTracker()
	}

	return Report{
		Changes: ChangeSet{
			Created: summarizeMutations(t.created, summary),
			Updated: summarizeMutations(t.updated, summary),
			Removed: summarizeMutations(t.removed, summary),
		},
		AffectedPaths: sortedUniqueStrings(affectedPaths),
	}
}

func (p summaryProfile) summarize(path string) string {
	for _, rule := range p.rules {
		if rule.match(path) {
			return rule.summary
		}
	}

	if p.fallback == nil {
		return path
	}
	return p.fallback(path)
}

func (p summaryProfile) buildReport(tracker *mutationTracker) Report {
	return tracker.buildReport(p.summarize, p.affectedPaths)
}

func summarizeMutations(paths map[string]struct{}, summary func(string) string) []string {
	if len(paths) == 0 {
		return []string{}
	}

	summaries := make(map[string]struct{}, len(paths))
	for path := range paths {
		s := summary(path)
		if s == "" {
			continue
		}
		summaries[s] = struct{}{}
	}

	return sortedKeys(summaries)
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		seen[value] = struct{}{}
	}

	return sortedKeys(seen)
}

func sortedKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return []string{}
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func pathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func displayPath(projectRoot, homeDir, path string) string {
	if projectRoot != "" && pathWithin(projectRoot, path) {
		rel, err := filepath.Rel(projectRoot, path)
		if err == nil {
			return filepath.Clean(rel)
		}
	}

	if homeDir != "" && pathWithin(homeDir, path) {
		rel, err := filepath.Rel(homeDir, path)
		if err == nil {
			if rel == "." {
				return "~"
			}
			return filepath.Join("~", rel)
		}
	}

	return path
}

// buildProjectSetupReport declares the success-output summaries for project
// lifecycle. The literals below are intentionally explicit because this function
// defines the user-visible contract for "affected_paths" and summarized
// changes; hiding them behind inference would make review harder and would blur
// the difference between concrete filesystem writes and the merged semantics we
// promise to users.
func buildProjectSetupReport(projectRoot, homeDir string, tracker *mutationTracker) Report {
	return summaryProfile{
		affectedPaths: []string{
			".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/",
			".agents/skills/argus-*/SKILL.md",
			".claude/skills/argus-*/SKILL.md",
			".claude/settings.json",
			".codex/hooks.json",
			".opencode/plugins/argus.ts",
			"~/.codex/config.toml",
		},
		// These rules define the public output summaries for project lifecycle.
		rules: []summaryRule{
			prefixRule(filepath.Join(projectRoot, ".argus"), ".argus/{workflows,invariants,rules,pipelines,logs,data,tmp}/"),
			prefixRule(filepath.Join(projectRoot, ".agents", "skills"), ".agents/skills/argus-*/SKILL.md"),
			prefixRule(filepath.Join(projectRoot, ".claude", "skills"), ".claude/skills/argus-*/SKILL.md"),
			exactRule(filepath.Join(projectRoot, ".claude", "settings.json"), ".claude/settings.json"),
			exactRule(filepath.Join(projectRoot, ".codex", "hooks.json"), ".codex/hooks.json"),
			exactRule(filepath.Join(projectRoot, ".opencode", "plugins", "argus.ts"), ".opencode/plugins/argus.ts"),
			exactRule(filepath.Join(homeDir, ".codex", "config.toml"), "~/.codex/config.toml"),
		},
		fallback: func(path string) string { return displayPath(projectRoot, homeDir, path) },
	}.buildReport(tracker)
}

func buildProjectTeardownReport(projectRoot, homeDir string, tracker *mutationTracker) Report {
	return summaryProfile{
		affectedPaths: []string{
			".argus/",
			".agents/skills/argus-*",
			".claude/skills/argus-*",
			".claude/settings.json",
			".codex/hooks.json",
			".opencode/plugins/argus.ts",
		},
		// Teardown uses a different summary contract from setup for some paths,
		// notably skill directories and the top-level .argus removal.
		rules: []summaryRule{
			prefixRule(filepath.Join(projectRoot, ".argus"), ".argus/"),
			exactRule(filepath.Join(projectRoot, ".argus"), ".argus/"),
			prefixRule(filepath.Join(projectRoot, ".agents", "skills"), ".agents/skills/argus-*"),
			prefixRule(filepath.Join(projectRoot, ".claude", "skills"), ".claude/skills/argus-*"),
			exactRule(filepath.Join(projectRoot, ".claude", "settings.json"), ".claude/settings.json"),
			exactRule(filepath.Join(projectRoot, ".codex", "hooks.json"), ".codex/hooks.json"),
			exactRule(filepath.Join(projectRoot, ".opencode", "plugins", "argus.ts"), ".opencode/plugins/argus.ts"),
		},
		fallback: func(path string) string { return displayPath(projectRoot, homeDir, path) },
	}.buildReport(tracker)
}

func buildWorkspaceSetupReport(homeDir string, tracker *mutationTracker) Report {
	return summaryProfile{
		affectedPaths: []string{
			"~/.config/argus/{invariants,workflows,pipelines,logs}/",
			"~/.config/argus/config.yaml",
			"~/.claude/settings.json",
			"~/.codex/{hooks.json,config.toml}",
			"~/.config/opencode/plugins/argus.ts",
			"~/.claude/skills/argus-*/SKILL.md",
			"~/.agents/skills/argus-*/SKILL.md",
			"~/.config/opencode/skills/argus-*/SKILL.md",
		},
		// Workspace setup reports user-scoped config, hook, skill, and global
		// artifact summaries.
		rules: []summaryRule{
			anyOfRule([]string{
				filepath.Join(homeDir, ".config", "argus", "invariants"),
				filepath.Join(homeDir, ".config", "argus", "workflows"),
				filepath.Join(homeDir, ".config", "argus", "pipelines"),
				filepath.Join(homeDir, ".config", "argus", "logs"),
			}, "~/.config/argus/{invariants,workflows,pipelines,logs}/"),
			prefixRule(filepath.Join(homeDir, ".config", "argus", "invariants"), "~/.config/argus/{invariants,workflows,pipelines,logs}/"),
			prefixRule(filepath.Join(homeDir, ".config", "argus", "workflows"), "~/.config/argus/{invariants,workflows,pipelines,logs}/"),
			prefixRule(filepath.Join(homeDir, ".config", "argus", "pipelines"), "~/.config/argus/{invariants,workflows,pipelines,logs}/"),
			prefixRule(filepath.Join(homeDir, ".config", "argus", "logs"), "~/.config/argus/{invariants,workflows,pipelines,logs}/"),
			exactRule(userConfigPathForHome(homeDir), "~/.config/argus/config.yaml"),
			exactRule(filepath.Join(homeDir, claudeSettingsRelativePath), "~/.claude/settings.json"),
			anyOfRule([]string{
				filepath.Join(homeDir, codexHooksRelativePath),
				filepath.Join(homeDir, codexConfigRelativePath),
			}, "~/.codex/{hooks.json,config.toml}"),
			exactRule(globalOpenCodePluginPathForHome(homeDir), "~/.config/opencode/plugins/argus.ts"),
			prefixRule(filepath.Join(homeDir, ".claude", "skills"), "~/.claude/skills/argus-*/SKILL.md"),
			prefixRule(filepath.Join(homeDir, ".agents", "skills"), "~/.agents/skills/argus-*/SKILL.md"),
			prefixRule(filepath.Join(homeDir, ".config", "opencode", "skills"), "~/.config/opencode/skills/argus-*/SKILL.md"),
		},
		fallback: func(path string) string { return displayPath("", homeDir, path) },
	}.buildReport(tracker)
}

func buildWorkspaceTeardownReport(homeDir string, tracker *mutationTracker, toreDownGlobalResources bool) Report {
	affectedPaths := []string{
		"~/.config/argus/config.yaml",
	}
	if toreDownGlobalResources {
		affectedPaths = append(affectedPaths,
			"~/.config/argus/",
			"~/.claude/settings.json",
			"~/.codex/hooks.json",
			"~/.config/opencode/plugins/argus.ts",
			"~/.claude/skills/argus-*",
			"~/.agents/skills/argus-*",
			"~/.config/opencode/skills/argus-*",
		)
	}

	return summaryProfile{
		affectedPaths: affectedPaths,
		// Workspace teardown only reports global hook/skill/artifact summaries
		// when the last workspace registration is removed.
		rules: []summaryRule{
			exactRule(filepath.Join(homeDir, ".config", "argus"), "~/.config/argus/"),
			prefixRule(filepath.Join(homeDir, ".config", "argus"), "~/.config/argus/"),
			exactRule(userConfigPathForHome(homeDir), "~/.config/argus/config.yaml"),
			exactRule(filepath.Join(homeDir, claudeSettingsRelativePath), "~/.claude/settings.json"),
			exactRule(filepath.Join(homeDir, codexHooksRelativePath), "~/.codex/hooks.json"),
			exactRule(globalOpenCodePluginPathForHome(homeDir), "~/.config/opencode/plugins/argus.ts"),
			prefixRule(filepath.Join(homeDir, ".claude", "skills"), "~/.claude/skills/argus-*"),
			prefixRule(filepath.Join(homeDir, ".agents", "skills"), "~/.agents/skills/argus-*"),
			prefixRule(filepath.Join(homeDir, ".config", "opencode", "skills"), "~/.config/opencode/skills/argus-*"),
		},
		fallback: func(path string) string { return displayPath("", homeDir, path) },
	}.buildReport(tracker)
}

func exactRule(path, summary string) summaryRule {
	return summaryRule{
		summary: summary,
		match: func(candidate string) bool {
			return candidate == path
		},
	}
}

func anyOfRule(paths []string, summary string) summaryRule {
	return summaryRule{
		summary: summary,
		match: func(candidate string) bool {
			for _, path := range paths {
				if candidate == path {
					return true
				}
			}
			return false
		},
	}
}

func prefixRule(pathPrefix, summary string) summaryRule {
	return summaryRule{
		summary: summary,
		match: func(candidate string) bool {
			return pathWithin(pathPrefix, candidate)
		},
	}
}
