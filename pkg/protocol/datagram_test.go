package protocol

import (
	"bytes"
	"testing"
)

func TestDatagramRoundTrip(t *testing.T) {
	const target = "1.2.3.4:53"
	payload := []byte("dns query bytes")

	var buf bytes.Buffer
	if err := WriteDatagram(&buf, target, payload); err != nil {
		t.Fatalf("WriteDatagram: %v", err)
	}

	gotTarget, gotPayload, err := ReadDatagram(&buf)
	if err != nil {
		t.Fatalf("ReadDatagram: %v", err)
	}
	if gotTarget != target {
		t.Fatalf("target = %q, want %q", gotTarget, target)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("payload = %q, want %q", gotPayload, payload)
	}
}

func TestDatagramTwoInARow(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteDatagram(&buf, "a:1", []byte("first"))
	_ = WriteDatagram(&buf, "b:2", []byte("second"))

	tg1, p1, err := ReadDatagram(&buf)
	if err != nil || tg1 != "a:1" || string(p1) != "first" {
		t.Fatalf("first = (%q,%q,%v)", tg1, p1, err)
	}
	tg2, p2, err := ReadDatagram(&buf)
	if err != nil || tg2 != "b:2" || string(p2) != "second" {
		t.Fatalf("second = (%q,%q,%v)", tg2, p2, err)
	}
}

func TestDatagramEmptyPayloadOK(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteDatagram(&buf, "a:1", nil); err != nil {
		t.Fatalf("WriteDatagram empty: %v", err)
	}
	tg, p, err := ReadDatagram(&buf)
	if err != nil || tg != "a:1" || len(p) != 0 {
		t.Fatalf("empty payload = (%q,%q,%v)", tg, p, err)
	}
}
