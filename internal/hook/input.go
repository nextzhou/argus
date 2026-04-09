// Package hook provides hook command handlers for multi-agent integration.
package hook

import (
	"encoding/json"
	"fmt"
	"io"
)

// AgentInput represents parsed stdin JSON from any supported AI agent.
type AgentInput struct {
	SessionID     string // Session identifier (mapped from agent-specific field)
	CWD           string // Current working directory (may be empty for some agents)
	HookEventName string // Hook event name (Claude Code only)
	Prompt        string // User prompt text (Claude Code only)
	AgentID       string // Sub-agent ID for Claude Code (non-empty = sub-agent)
	ParentID      string // Parent session ID for OpenCode (non-empty = sub-agent)
}

// ParseInput parses stdin JSON from the specified agent and returns an AgentInput.
// The agent parameter determines which field names to expect in the JSON.
// Supported agents: claude-code, codex, opencode.
func ParseInput(r io.Reader, agent string) (*AgentInput, error) {
	// Read all bytes from reader
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	// Parse JSON into map for flexible field extraction
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s input: %w", agent, err)
	}

	input := &AgentInput{}

	// Agent-specific field mapping
	switch agent {
	case "claude-code":
		// Claude Code uses snake_case field names
		if sessionID, ok := raw["session_id"].(string); ok {
			input.SessionID = sessionID
		}
		if cwd, ok := raw["cwd"].(string); ok {
			input.CWD = cwd
		}
		if hookEventName, ok := raw["hook_event_name"].(string); ok {
			input.HookEventName = hookEventName
		}
		if prompt, ok := raw["prompt"].(string); ok {
			input.Prompt = prompt
		}
		if agentID, ok := raw["agent_id"].(string); ok {
			input.AgentID = agentID
		}

		// Validate required field
		if input.SessionID == "" {
			return nil, fmt.Errorf("claude-code input: missing session_id")
		}

	case "codex":
		// Codex uses snake_case field names
		if sessionID, ok := raw["session_id"].(string); ok {
			input.SessionID = sessionID
		}
		if cwd, ok := raw["cwd"].(string); ok {
			input.CWD = cwd
		}

		// Validate required field
		if input.SessionID == "" {
			return nil, fmt.Errorf("codex input: missing session_id")
		}

	case "opencode":
		// OpenCode uses camelCase field names
		if sessionID, ok := raw["sessionID"].(string); ok {
			input.SessionID = sessionID
		}
		if cwd, ok := raw["cwd"].(string); ok {
			input.CWD = cwd
		}
		if parentID, ok := raw["parentID"].(string); ok {
			input.ParentID = parentID
		}

		// Validate required field
		if input.SessionID == "" {
			return nil, fmt.Errorf("opencode input: missing sessionID")
		}

	default:
		// Graceful fallback for unknown agent types: attempt snake_case field mapping.
		// This avoids hard-failing on future agents that follow similar JSON conventions,
		// while still enforcing session_id as a required field.
		if sessionID, ok := raw["session_id"].(string); ok {
			input.SessionID = sessionID
		}
		if cwd, ok := raw["cwd"].(string); ok {
			input.CWD = cwd
		}

		// Validate required field
		if input.SessionID == "" {
			return nil, fmt.Errorf("%s input: missing session_id", agent)
		}
	}

	return input, nil
}

// IsSubAgent returns true if the input represents a sub-agent invocation.
// For Claude Code, a sub-agent is detected by non-empty AgentID.
// For OpenCode, a sub-agent is detected by non-empty ParentID.
func IsSubAgent(input *AgentInput) bool {
	return input.AgentID != "" || input.ParentID != ""
}
