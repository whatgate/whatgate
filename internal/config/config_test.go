package config

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes a temp JSON config file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// Config values fill in flags the operator did not set on the command line.
func TestApplyFileSetsUnsetFlags(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	foo := fs.String("foo", "default", "")
	n := fs.Int("n", 0, "")
	fs.Parse(nil)

	if err := ApplyFile(fs, writeConfig(t, `{"foo":"bar","n":5}`)); err != nil {
		t.Fatalf("ApplyFile: %v", err)
	}
	if *foo != "bar" || *n != 5 {
		t.Fatalf("foo=%q n=%d, want bar/5", *foo, *n)
	}
}

// A flag set explicitly on the command line wins over the config file.
func TestCommandLineOverridesConfig(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	foo := fs.String("foo", "default", "")
	fs.Parse([]string{"-foo", "cli"})

	if err := ApplyFile(fs, writeConfig(t, `{"foo":"cfg"}`)); err != nil {
		t.Fatalf("ApplyFile: %v", err)
	}
	if *foo != "cli" {
		t.Fatalf("foo=%q, want cli (command line wins)", *foo)
	}
}

// Bool and float flags round-trip from their native JSON types.
func TestApplyFileBoolAndFloat(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	b := fs.Bool("b", false, "")
	r := fs.Float64("r", 0, "")
	fs.Parse(nil)

	if err := ApplyFile(fs, writeConfig(t, `{"b":true,"r":0.3}`)); err != nil {
		t.Fatalf("ApplyFile: %v", err)
	}
	if !*b || *r != 0.3 {
		t.Fatalf("b=%v r=%v, want true/0.3", *b, *r)
	}
}

// An unknown key is a typo the operator wants to hear about, not silently ignore.
func TestApplyFileUnknownKeyErrors(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.String("foo", "", "")
	fs.Parse(nil)

	err := ApplyFile(fs, writeConfig(t, `{"nope":1}`))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("err = %v, want error naming unknown key nope", err)
	}
}

// A missing file is surfaced (the caller chose to require a config path).
func TestApplyFileMissingFileErrors(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.Parse(nil)
	if err := ApplyFile(fs, filepath.Join(t.TempDir(), "absent.json")); err == nil {
		t.Fatal("expected error for missing file")
	}
}
