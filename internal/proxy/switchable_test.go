package proxy

import (
	"context"
	"net"
	"testing"
)

// labelDialer records that it was used and returns a closed pipe end.
type labelDialer struct {
	label string
	used  *string
}

func (d labelDialer) Dial(ctx context.Context, addr string) (net.Conn, error) {
	*d.used = d.label
	c, _ := net.Pipe()
	return c, nil
}

func TestSwitchableDialerDelegatesToCurrent(t *testing.T) {
	var used string
	sw := &SwitchableDialer{}

	sw.Set(labelDialer{label: "A", used: &used})
	if _, err := sw.Dial(context.Background(), "x:1"); err != nil {
		t.Fatalf("dial via A: %v", err)
	}
	if used != "A" {
		t.Fatalf("used = %q, want A", used)
	}

	sw.Set(labelDialer{label: "B", used: &used})
	if _, err := sw.Dial(context.Background(), "x:1"); err != nil {
		t.Fatalf("dial via B: %v", err)
	}
	if used != "B" {
		t.Fatalf("used = %q, want B (after switch)", used)
	}
}

func TestSwitchableDialerWithoutSetErrors(t *testing.T) {
	sw := &SwitchableDialer{}
	if _, err := sw.Dial(context.Background(), "x:1"); err == nil {
		t.Fatal("dial with no dialer set should error")
	}
}
