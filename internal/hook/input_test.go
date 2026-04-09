package hook

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInput_ClaudeCode_AllFields(t *testing.T) {
	input := `{
		"session_id": "abc-123",
		"cwd": "/project",
		"hook_event_name": "UserPromptSubmit",
		"prompt": "Run the test suite"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "claude-code")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", result.SessionID)
	assert.Equal(t, "/project", result.CWD)
	assert.Equal(t, "UserPromptSubmit", result.HookEventName)
	assert.Equal(t, "Run the test suite", result.Prompt)
	assert.Empty(t, result.AgentID)
	assert.Empty(t, result.ParentID)
}

func TestParseInput_ClaudeCode_WithAgentID(t *testing.T) {
	input := `{
		"session_id": "abc-123",
		"cwd": "/project",
		"hook_event_name": "UserPromptSubmit",
		"prompt": "Run the test suite",
		"agent_id": "sub-agent-456"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "claude-code")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", result.SessionID)
	assert.Equal(t, "sub-agent-456", result.AgentID)
	assert.True(t, IsSubAgent(result))
}

func TestParseInput_Codex_BasicFields(t *testing.T) {
	input := `{
		"session_id": "codex-789",
		"cwd": "/workspace"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "codex")
	require.NoError(t, err)
	assert.Equal(t, "codex-789", result.SessionID)
	assert.Equal(t, "/workspace", result.CWD)
	assert.Empty(t, result.HookEventName)
	assert.Empty(t, result.Prompt)
	assert.Empty(t, result.AgentID)
	assert.Empty(t, result.ParentID)
}

func TestParseInput_OpenCode_NoParentID(t *testing.T) {
	input := `{
		"sessionID": "opencode-111",
		"cwd": "/project"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "opencode")
	require.NoError(t, err)
	assert.Equal(t, "opencode-111", result.SessionID)
	assert.Equal(t, "/project", result.CWD)
	assert.Empty(t, result.ParentID)
	assert.False(t, IsSubAgent(result))
}

func TestParseInput_OpenCode_WithParentID(t *testing.T) {
	input := `{
		"sessionID": "opencode-222",
		"cwd": "/workspace",
		"parentID": "parent-session-333"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "opencode")
	require.NoError(t, err)
	assert.Equal(t, "opencode-222", result.SessionID)
	assert.Equal(t, "/workspace", result.CWD)
	assert.Equal(t, "parent-session-333", result.ParentID)
	assert.True(t, IsSubAgent(result))
}

func TestParseInput_MalformedJSON(t *testing.T) {
	input := `{invalid json}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing claude-code input")
}

func TestParseInput_ClaudeCode_MissingSessionID(t *testing.T) {
	input := `{
		"cwd": "/project",
		"hook_event_name": "UserPromptSubmit"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "claude-code")
	require.Error(t, err)
	assert.Equal(t, "claude-code input: missing session_id", err.Error())
}

func TestParseInput_Codex_MissingSessionID(t *testing.T) {
	input := `{
		"cwd": "/workspace"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "codex")
	require.Error(t, err)
	assert.Equal(t, "codex input: missing session_id", err.Error())
}

func TestParseInput_OpenCode_MissingSessionID(t *testing.T) {
	input := `{
		"parentID": "parent-123"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "opencode")
	require.Error(t, err)
	assert.Equal(t, "opencode input: missing sessionID", err.Error())
}

func TestParseInput_EmptyReader(t *testing.T) {
	r := bytes.NewReader([]byte(""))

	_, err := ParseInput(r, "claude-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing claude-code input")
}

func TestParseInput_UnknownAgent_Fallback(t *testing.T) {
	input := `{
		"session_id": "unknown-agent-123",
		"cwd": "/some/path"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "unknown-agent")
	require.NoError(t, err)
	assert.Equal(t, "unknown-agent-123", result.SessionID)
	assert.Equal(t, "/some/path", result.CWD)
}

func TestParseInput_UnknownAgent_MissingSessionID(t *testing.T) {
	input := `{
		"cwd": "/some/path"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "unknown-agent")
	require.Error(t, err)
	assert.Equal(t, "unknown-agent input: missing session_id", err.Error())
}

func TestIsSubAgent_ClaudeCodeSubAgent(t *testing.T) {
	input := &AgentInput{
		SessionID: "abc-123",
		AgentID:   "sub-agent-456",
	}
	assert.True(t, IsSubAgent(input))
}

func TestIsSubAgent_OpenCodeSubAgent(t *testing.T) {
	input := &AgentInput{
		SessionID: "opencode-111",
		ParentID:  "parent-222",
	}
	assert.True(t, IsSubAgent(input))
}

func TestIsSubAgent_NotSubAgent(t *testing.T) {
	input := &AgentInput{
		SessionID: "abc-123",
	}
	assert.False(t, IsSubAgent(input))
}

func TestIsSubAgent_BothAgentIDAndParentID(t *testing.T) {
	input := &AgentInput{
		SessionID: "abc-123",
		AgentID:   "sub-agent-456",
		ParentID:  "parent-789",
	}
	assert.True(t, IsSubAgent(input))
}

func TestParseInput_ClaudeCode_PartialFields(t *testing.T) {
	input := `{
		"session_id": "abc-123"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "claude-code")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", result.SessionID)
	assert.Empty(t, result.CWD)
	assert.Empty(t, result.HookEventName)
	assert.Empty(t, result.Prompt)
}

func TestParseInput_ClaudeCode_ExtraFields(t *testing.T) {
	input := `{
		"session_id": "abc-123",
		"cwd": "/project",
		"extra_field": "should be ignored",
		"another_extra": 123
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "claude-code")
	require.NoError(t, err)
	assert.Equal(t, "abc-123", result.SessionID)
	assert.Equal(t, "/project", result.CWD)
}

func TestParseInput_OpenCode_ExtraFields(t *testing.T) {
	input := `{
		"sessionID": "opencode-111",
		"extra_field": "ignored",
		"nested": {"key": "value"}
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "opencode")
	require.NoError(t, err)
	assert.Equal(t, "opencode-111", result.SessionID)
}

func TestParseInput_Codex_EmptySessionID(t *testing.T) {
	input := `{
		"session_id": "",
		"cwd": "/workspace"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "codex")
	require.Error(t, err)
	assert.Equal(t, "codex input: missing session_id", err.Error())
}

func TestParseInput_ClaudeCode_NullSessionID(t *testing.T) {
	input := `{
		"session_id": null,
		"cwd": "/project"
	}`
	r := bytes.NewReader([]byte(input))

	_, err := ParseInput(r, "claude-code")
	require.Error(t, err)
	assert.Equal(t, "claude-code input: missing session_id", err.Error())
}

func TestParseInput_OpenCode_CaseSensitive(t *testing.T) {
	input := `{
		"session_id": "wrong-case",
		"sessionID": "correct-case"
	}`
	r := bytes.NewReader([]byte(input))

	result, err := ParseInput(r, "opencode")
	require.NoError(t, err)
	assert.Equal(t, "correct-case", result.SessionID)
}
