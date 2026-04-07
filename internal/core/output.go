package core

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// MarkdownRenderer is implemented by types that can render themselves as Markdown.
// Reserved for future --markdown flag support. No implementation in M1.
type MarkdownRenderer interface {
	RenderMarkdown(w io.Writer)
}

// errorEnvelopeJSON is the internal struct for error envelope serialization.
// Using a struct ensures consistent field ordering in JSON output.
type errorEnvelopeJSON struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// OKEnvelope marshals data into a flat JSON envelope with "status":"ok".
//
// The data's fields are merged at the top level alongside "status":
//
//	OKEnvelope(struct{Foo string `json:"foo"`}{Foo:"bar"})
//	// produces: {"status":"ok","foo":"bar"}
//
// If data is nil, returns {"status":"ok"}.
// Returns an error if marshaling fails.
func OKEnvelope(data any) ([]byte, error) {
	if data == nil {
		return json.Marshal(map[string]any{"status": "ok"})
	}

	// Marshal data to get its fields as a map
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling envelope data: %w", err)
	}

	// Decode data into a generic map
	var dataMap map[string]any
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		// Data might not be an object (e.g., slice or primitive) — fall back to wrapping
		return json.Marshal(map[string]any{"status": "ok", "data": data})
	}

	// Inject "status":"ok" into the map
	dataMap["status"] = "ok"

	return json.Marshal(dataMap)
}

// ErrorEnvelope marshals an error message into the standard error envelope.
// Always produces: {"status":"error","message":"..."}
func ErrorEnvelope(msg string) ([]byte, error) {
	return json.Marshal(errorEnvelopeJSON{
		Status:  "error",
		Message: msg,
	})
}

// WriteJSON writes data as JSON to w.
// This is a best-effort operation: if writing fails, the error is logged via slog
// but not returned. Callers at the CLI boundary cannot meaningfully handle stdout failures.
func WriteJSON(w io.Writer, data any) {
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("failed to write JSON output", "error", err)
	}
}
