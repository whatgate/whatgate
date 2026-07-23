package protocol

import (
	"bytes"
	"testing"
)

func TestTargetRoundTrip(t *testing.T) {
	const addr = "example.com:443"

	var buf bytes.Buffer
	if err := WriteTarget(&buf, addr); err != nil {
		t.Fatalf("WriteTarget: %v", err)
	}

	got, err := ReadTarget(&buf)
	if err != nil {
		t.Fatalf("ReadTarget: %v", err)
	}
	if got != addr {
		t.Fatalf("round trip: got %q want %q", got, addr)
	}
}
