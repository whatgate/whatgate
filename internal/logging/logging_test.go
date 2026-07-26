package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The json format emits one machine-parseable JSON object per line with the
// message and structured attributes.
func TestJSONFormatEmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "json")
	log.Info("rate limited", "ip", "1.2.3.4", "endpoint", "join")

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("output is not JSON: %v (%q)", err, buf.String())
	}
	if rec["msg"] != "rate limited" || rec["ip"] != "1.2.3.4" || rec["endpoint"] != "join" {
		t.Fatalf("missing fields: %v", rec)
	}
}

// The text format is human-readable key=value, not JSON.
func TestTextFormatEmitsText(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "text")
	log.Info("hello", "k", "v")

	out := buf.String()
	if !strings.Contains(out, "hello") || !strings.Contains(out, "k=v") {
		t.Fatalf("text output missing expected content: %q", out)
	}
	if json.Valid(bytes.TrimSpace(buf.Bytes())) {
		t.Fatalf("text format should not be valid JSON: %q", out)
	}
}

// An unknown/empty format falls back to human-readable text.
func TestUnknownFormatDefaultsToText(t *testing.T) {
	var buf bytes.Buffer
	log := New(&buf, "")
	log.Info("hello", "k", "v")
	if !strings.Contains(buf.String(), "k=v") {
		t.Fatalf("empty format should default to text, got %q", buf.String())
	}
}
