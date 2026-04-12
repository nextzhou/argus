package core

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOKEnvelope(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		data, err := OKEnvelope(nil)
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "ok", result["status"])
		assert.Len(t, result, 1) // only "status" field
	})

	t.Run("struct data - flat merge", func(t *testing.T) {
		type payload struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		}
		data, err := OKEnvelope(payload{Count: 42, Name: "test"})
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "ok", result["status"])
		assert.InDelta(t, 42, result["count"], 0)
		assert.Equal(t, "test", result["name"])
		// No "data" wrapper key
		assert.Nil(t, result["data"])
	})

	t.Run("map data - flat merge", func(t *testing.T) {
		data, err := OKEnvelope(map[string]any{"pipeline_status": "running", "progress": "1/5"})
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "ok", result["status"])
		assert.Equal(t, "running", result["pipeline_status"])
		assert.Equal(t, "1/5", result["progress"])
		assert.Nil(t, result["data"]) // No nested "data"
	})

	t.Run("produces valid JSON", func(t *testing.T) {
		type payload struct {
			Foo string `json:"foo"`
		}
		data, err := OKEnvelope(payload{Foo: "bar"})
		require.NoError(t, err)
		assert.True(t, json.Valid(data), "output must be valid JSON")
	})

	t.Run("no HTML escaping in angle brackets", func(t *testing.T) {
		type payload struct {
			Cmd string `json:"cmd"`
		}
		data, err := OKEnvelope(payload{Cmd: "argus workflow start <workflow-id>"})
		require.NoError(t, err)
		assert.Contains(t, string(data), "<workflow-id>")
		assert.NotContains(t, string(data), `\u003c`)
		assert.NotContains(t, string(data), `\u003e`)
	})

	t.Run("no HTML escaping in ampersand", func(t *testing.T) {
		type payload struct {
			Cmd string `json:"cmd"`
		}
		data, err := OKEnvelope(payload{Cmd: "make test && make lint"})
		require.NoError(t, err)
		assert.Contains(t, string(data), "&&")
		assert.NotContains(t, string(data), `\u0026`)
	})
}

func TestErrorEnvelope(t *testing.T) {
	t.Run("basic error message", func(t *testing.T) {
		data, err := ErrorEnvelope("not found")
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "error", result["status"])
		assert.Equal(t, "not found", result["message"])
	})

	t.Run("empty message", func(t *testing.T) {
		data, err := ErrorEnvelope("")
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "error", result["status"])
		assert.Empty(t, result["message"])
	})

	t.Run("valid JSON", func(t *testing.T) {
		data, err := ErrorEnvelope("some error")
		require.NoError(t, err)
		assert.True(t, json.Valid(data))
	})

	t.Run("no HTML escaping in angle brackets", func(t *testing.T) {
		data, err := ErrorEnvelope("use argus workflow start <workflow-id>")
		require.NoError(t, err)
		assert.Contains(t, string(data), "<workflow-id>")
		assert.NotContains(t, string(data), `\u003c`)
		assert.NotContains(t, string(data), `\u003e`)
	})

	t.Run("no HTML escaping in ampersand", func(t *testing.T) {
		data, err := ErrorEnvelope("run: make test && make lint")
		require.NoError(t, err)
		assert.Contains(t, string(data), "&&")
		assert.NotContains(t, string(data), `\u0026`)
	})

	t.Run("structured details are preserved", func(t *testing.T) {
		data, err := ErrorEnvelopeWithDetails("invalid invariant configuration", []map[string]any{
			{"file": "bad.yaml", "path": "order", "message": "order must be a positive integer"},
		})
		require.NoError(t, err)
		var result map[string]any
		require.NoError(t, json.Unmarshal(data, &result))
		assert.Equal(t, "error", result["status"])
		assert.Equal(t, "invalid invariant configuration", result["message"])
		details, ok := result["details"].([]any)
		require.True(t, ok)
		require.Len(t, details, 1)
	})
}

func TestWriteJSON(t *testing.T) {
	t.Run("writes valid JSON to buffer", func(t *testing.T) {
		type payload struct {
			Key string `json:"key"`
		}
		buf := new(bytes.Buffer)
		WriteJSON(buf, payload{Key: "value"})
		assert.True(t, json.Valid(bytes.TrimSpace(buf.Bytes())))
		var result map[string]any
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		assert.Equal(t, "value", result["key"])
	})

	t.Run("no HTML escaping in angle brackets", func(t *testing.T) {
		type payload struct {
			Msg string `json:"msg"`
		}
		buf := new(bytes.Buffer)
		WriteJSON(buf, payload{Msg: "<b>bold</b>"})
		assert.Contains(t, buf.String(), "<b>bold</b>")
		assert.NotContains(t, buf.String(), `\u003c`)
		assert.NotContains(t, buf.String(), `\u003e`)
	})

	t.Run("no HTML escaping in ampersand", func(t *testing.T) {
		type payload struct {
			Cmd string `json:"cmd"`
		}
		buf := new(bytes.Buffer)
		WriteJSON(buf, payload{Cmd: "make test && make lint"})
		assert.Contains(t, buf.String(), "&&")
		assert.NotContains(t, buf.String(), `\u0026`)
	})
}
