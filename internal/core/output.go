package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// marshalNoHTMLEscape marshals v to JSON without escaping <, >, and & characters.
// This is necessary because Argus outputs JSON to terminal/Agent consumers, not HTML.
// The default json.Marshal escapes these characters as \u003c, \u003e, \u0026, which
// produces unreadable error messages.
func marshalNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	// Encode appends a trailing newline; trim it to match json.Marshal behavior
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// errorEnvelopeJSON is the internal struct for error envelope serialization.
// Using a struct ensures consistent field ordering in JSON output.
type errorEnvelopeJSON struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
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
		return marshalNoHTMLEscape(map[string]any{"status": "ok"})
	}

	// Marshal data to get its fields as a map
	dataBytes, err := marshalNoHTMLEscape(data)
	if err != nil {
		return nil, fmt.Errorf("marshaling envelope data: %w", err)
	}

	// OKEnvelope only accepts JSON objects (structs/maps); slices and primitives cannot be
	// merged into a flat top-level envelope and will return an error.
	var dataMap map[string]any
	if err := json.Unmarshal(dataBytes, &dataMap); err != nil {
		return nil, fmt.Errorf("envelope data must be a JSON object (struct or map), got non-object: %w", err)
	}

	// Inject "status":"ok" into the map
	dataMap["status"] = "ok"

	return marshalNoHTMLEscape(dataMap)
}

// ErrorEnvelope marshals an error message into the standard error envelope.
// Always produces: {"status":"error","message":"..."}
func ErrorEnvelope(msg string) ([]byte, error) {
	return ErrorEnvelopeWithDetails(msg, nil)
}

// ErrorEnvelopeWithDetails marshals an error message and optional structured details
// into the standard error envelope.
func ErrorEnvelopeWithDetails(msg string, details any) ([]byte, error) {
	return marshalNoHTMLEscape(errorEnvelopeJSON{
		Status:  "error",
		Message: msg,
		Details: details,
	})
}

// WriteJSON writes data as JSON to w.
// This is a best-effort operation: if writing fails, the error is logged via slog
// but not returned. Callers at the CLI boundary cannot meaningfully handle stdout failures.
func WriteJSON(w io.Writer, data any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(data); err != nil {
		slog.Error("failed to write JSON output", "error", err)
	}
}
