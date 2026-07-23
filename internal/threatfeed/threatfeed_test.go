package threatfeed

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseListHandlesFormats(t *testing.T) {
	input := `# comment line
evil-one.example

0.0.0.0 evil-two.example
127.0.0.1 evil-three.example
Evil-Four.Example   # inline comment
localhost
`
	got, err := ParseList(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseList: %v", err)
	}

	for _, want := range []string{"evil-one.example", "evil-two.example", "evil-three.example", "evil-four.example"} {
		if !got[want] {
			t.Errorf("expected %q to be blocked; set = %v", want, got)
		}
	}
	if got["localhost"] {
		t.Error("localhost should be skipped")
	}
	if len(got) != 4 {
		t.Fatalf("got %d domains, want 4: %v", len(got), got)
	}
}

func TestFetchFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feed.txt")
	if err := os.WriteFile(path, []byte("bad.example\n0.0.0.0 worse.example\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := Fetch(path)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !got["bad.example"] || !got["worse.example"] {
		t.Fatalf("fetched set missing domains: %v", got)
	}
}
