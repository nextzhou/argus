package lifecycle

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

const (
	agentClaudeCode = "claude-code"
	agentCodex      = "codex"
	agentOpenCode   = "opencode"

	claudeSettingsRelativePath = ".claude/settings.json"
	codexHooksRelativePath     = ".codex/hooks.json"
	opencodePluginRelativePath = ".opencode/plugins/argus.ts"
	codexConfigRelativePath    = ".codex/config.toml"
)

// SetupHooks sets up Argus-managed hook files for the requested agents.
func SetupHooks(projectRoot string, agents []string) error {
	return setupHooks(projectRoot, agents, nil)
}

func setupHooks(projectRoot string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := setupHooksForAgent(projectRoot, agent, tracker); err != nil {
			return fmt.Errorf("setting up %s hooks: %w", agent, err)
		}
	}

	return nil
}

// TeardownHooks removes only Argus-managed hook files for the requested agents.
func TeardownHooks(projectRoot string, agents []string) error {
	return teardownHooks(projectRoot, agents, nil)
}

func teardownHooks(projectRoot string, agents []string, tracker *mutationTracker) error {
	for _, agent := range agents {
		if err := teardownHooksForAgent(projectRoot, agent, tracker); err != nil {
			return fmt.Errorf("tearing down %s hooks: %w", agent, err)
		}
	}

	return nil
}

func setupHooksForAgent(projectRoot string, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return setupClaudeCodeHooks(projectRoot, tracker)
	case agentCodex:
		return setupCodexHooks(projectRoot, tracker)
	case agentOpenCode:
		return setupOpenCodeHooks(projectRoot, tracker)
	default:
		_, err := RenderHookTemplate(agent, false)
		return err
	}
}

func teardownHooksForAgent(projectRoot string, agent string, tracker *mutationTracker) error {
	switch agent {
	case agentClaudeCode:
		return teardownClaudeCodeHooks(projectRoot, tracker)
	case agentCodex:
		return removeIfExistsTracked(filepath.Join(projectRoot, codexHooksRelativePath), tracker)
	case agentOpenCode:
		return removeIfExistsTracked(filepath.Join(projectRoot, opencodePluginRelativePath), tracker)
	default:
		_, err := RenderHookTemplate(agent, false)
		return err
	}
}

func setupClaudeCodeHooks(projectRoot string, tracker *mutationTracker) error {
	return setupClaudeCodeHooksAt(filepath.Join(projectRoot, claudeSettingsRelativePath), false, tracker)
}

func setupClaudeCodeHooksAt(settingsPath string, global bool, tracker *mutationTracker) error {
	settings, err := loadJSONObject(settingsPath)
	if err != nil {
		return fmt.Errorf("parsing claude code settings: %w", err)
	}

	desiredEvents, err := loadTemplateHookEvents(agentClaudeCode, global)
	if err != nil {
		return err
	}

	hooks, err := ensureObject(settings, "hooks")
	if err != nil {
		return fmt.Errorf("reading claude code hooks: %w", err)
	}

	for _, event := range managedClaudeCodeHookEvents() {
		existingEntries, err := getArray(hooks, event)
		if err != nil {
			return fmt.Errorf("reading claude code %s hooks: %w", event, err)
		}

		cleanedEntries, err := removeArgusEntries(existingEntries)
		if err != nil {
			return fmt.Errorf("cleaning claude code %s hooks: %w", event, err)
		}

		cleanedEntries = append(cleanedEntries, desiredEvents[event]...)
		if len(cleanedEntries) == 0 {
			delete(hooks, event)
			continue
		}

		hooks[event] = cleanedEntries
	}

	return writeJSONObjectTracked(settingsPath, settings, tracker)
}

func teardownClaudeCodeHooks(projectRoot string, tracker *mutationTracker) error {
	settingsPath := filepath.Join(projectRoot, claudeSettingsRelativePath)
	return teardownClaudeCodeHooksAt(settingsPath, tracker)
}

func teardownClaudeCodeHooksAt(settingsPath string, tracker *mutationTracker) error {
	settings, err := loadJSONObjectIfExists(settingsPath)
	if err != nil {
		return fmt.Errorf("parsing claude code settings: %w", err)
	}
	if settings == nil {
		return nil
	}

	hooksValue, ok := settings["hooks"]
	if !ok {
		return nil
	}

	hooks, ok := hooksValue.(map[string]any)
	if !ok {
		return fmt.Errorf("reading claude code hooks: hooks must be an object")
	}

	for _, event := range managedClaudeCodeHookEvents() {
		existingEntries, err := getArray(hooks, event)
		if err != nil {
			return fmt.Errorf("reading claude code %s hooks: %w", event, err)
		}

		cleanedEntries, err := removeArgusEntries(existingEntries)
		if err != nil {
			return fmt.Errorf("cleaning claude code %s hooks: %w", event, err)
		}

		if len(cleanedEntries) == 0 {
			delete(hooks, event)
			continue
		}

		hooks[event] = cleanedEntries
	}

	if len(hooks) == 0 {
		delete(settings, "hooks")
	}

	return writeJSONObjectTracked(settingsPath, settings, tracker)
}

func setupCodexHooks(projectRoot string, tracker *mutationTracker) error {
	return setupCodexHooksAt(filepath.Join(projectRoot, codexHooksRelativePath), false, tracker)
}

func setupCodexHooksAt(hooksPath string, global bool, tracker *mutationTracker) error {
	rendered, err := RenderHookTemplate(agentCodex, global)
	if err != nil {
		return err
	}

	if err := writeFileTracked(hooksPath, rendered, tracker); err != nil {
		return fmt.Errorf("writing codex hooks: %w", err)
	}

	if err := ensureCodexHooksEnabled(tracker); err != nil {
		return err
	}

	return nil
}

func setupOpenCodeHooks(projectRoot string, tracker *mutationTracker) error {
	return setupOpenCodeHooksAt(filepath.Join(projectRoot, opencodePluginRelativePath), false, tracker)
}

func setupOpenCodeHooksAt(pluginPath string, global bool, tracker *mutationTracker) error {
	rendered, err := RenderHookTemplate(agentOpenCode, global)
	if err != nil {
		return err
	}

	if err := writeFileTracked(pluginPath, rendered, tracker); err != nil {
		return fmt.Errorf("writing opencode plugin: %w", err)
	}

	return nil
}

func ensureCodexHooksEnabled(tracker *mutationTracker) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, codexConfigRelativePath)
	config := map[string]any{}

	//nolint:gosec // The Codex config path is derived from the resolved user home directory and a fixed relative path.
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("reading codex config: %w", err)
		}
	} else if err := toml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parsing codex config: %w", err)
	}

	features, err := ensureObject(config, "features")
	if err != nil {
		return fmt.Errorf("reading codex features config: %w", err)
	}
	features["codex_hooks"] = true

	rendered, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("encoding codex config: %w", err)
	}

	if err := writeFileTracked(configPath, rendered, tracker); err != nil {
		return fmt.Errorf("writing codex config: %w", err)
	}

	return nil
}

func loadTemplateHookEvents(agent string, global bool) (map[string][]any, error) {
	rendered, err := RenderHookTemplate(agent, global)
	if err != nil {
		return nil, err
	}

	var templateData map[string]any
	decoder := json.NewDecoder(bytes.NewReader(rendered))
	if err := decoder.Decode(&templateData); err != nil {
		return nil, fmt.Errorf("parsing %s hook template: %w", agent, err)
	}

	hooks, err := requireObject(templateData, "hooks")
	if err != nil {
		return nil, fmt.Errorf("reading %s hook template: %w", agent, err)
	}

	managedEvents := managedClaudeCodeHookEvents()
	events := make(map[string][]any, len(managedEvents))
	for _, event := range managedEvents {
		entries, err := getArray(hooks, event)
		if err != nil {
			return nil, fmt.Errorf("reading %s %s template hooks: %w", agent, event, err)
		}
		events[event] = append([]any(nil), entries...)
	}

	return events, nil
}

func loadJSONObject(path string) (map[string]any, error) {
	//nolint:gosec // loadJSONObject intentionally reads the exact JSON file path selected by the caller.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var parsed map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	if parsed == nil {
		return map[string]any{}, nil
	}

	return parsed, nil
}

func loadJSONObjectIfExists(path string) (map[string]any, error) {
	//nolint:gosec // loadJSONObjectIfExists intentionally reads the exact JSON file path selected by the caller.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var parsed map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	if parsed == nil {
		return map[string]any{}, nil
	}

	return parsed, nil
}

func writeJSONObjectTracked(path string, value map[string]any, tracker *mutationTracker) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}

	return writeFileTracked(path, buf.Bytes(), tracker)
}

func writeFileTracked(path string, content []byte, tracker *mutationTracker) error {
	existed := false
	//nolint:gosec // writeFileTracked intentionally compares and rewrites the exact path selected by the caller.
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		existed = true
		if bytes.Equal(existing, content) {
			return nil
		}
	case errors.Is(err, os.ErrNotExist):
		// The file will be created below.
	default:
		return fmt.Errorf("reading %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	if err := os.WriteFile(path, content, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	if !existed {
		tracker.recordCreated(path)
		return nil
	}

	tracker.recordUpdated(path)
	return nil
}

func removeIfExistsTracked(path string, tracker *mutationTracker) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("removing %s: %w", path, err)
	}

	tracker.recordRemoved(path)
	return nil
}

func managedClaudeCodeHookEvents() []string {
	return []string{"UserPromptSubmit", "PreToolUse"}
}

func removeAllIfExists(path string, tracker *mutationTracker) error {
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("stating %s: %w", path, err)
	}

	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removing %s: %w", path, err)
	}

	tracker.recordRemoved(path)
	return nil
}

func ensureObject(parent map[string]any, key string) (map[string]any, error) {
	if value, ok := parent[key]; ok {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", key)
		}
		return object, nil
	}

	object := map[string]any{}
	parent[key] = object
	return object, nil
}

func requireObject(parent map[string]any, key string) (map[string]any, error) {
	value, ok := parent[key]
	if !ok {
		return nil, fmt.Errorf("%s is missing", key)
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", key)
	}

	return object, nil
}

func getArray(parent map[string]any, key string) ([]any, error) {
	value, ok := parent[key]
	if !ok {
		return nil, nil
	}

	array, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array", key)
	}

	return array, nil
}

func removeArgusEntries(entries []any) ([]any, error) {
	cleaned := make([]any, 0, len(entries))
	for _, entryValue := range entries {
		entry, ok := entryValue.(map[string]any)
		if !ok {
			cleaned = append(cleaned, entryValue)
			continue
		}

		hooksValue, ok := entry["hooks"]
		if !ok {
			cleaned = append(cleaned, entryValue)
			continue
		}

		hooks, ok := hooksValue.([]any)
		if !ok {
			return nil, fmt.Errorf("hooks must be an array")
		}

		filteredHooks := make([]any, 0, len(hooks))
		for _, hookValue := range hooks {
			if isArgusHook(hookValue) {
				continue
			}
			filteredHooks = append(filteredHooks, hookValue)
		}

		if len(filteredHooks) == 0 {
			continue
		}

		entry["hooks"] = filteredHooks
		cleaned = append(cleaned, entry)
	}

	return cleaned, nil
}

func isArgusHook(value any) bool {
	hook, ok := value.(map[string]any)
	if !ok {
		return false
	}

	command, ok := hook["command"].(string)
	if !ok {
		return false
	}

	return strings.Contains(command, "argus tick") || strings.Contains(command, "argus trap")
}
