// Package config lets a command populate its flags from a JSON file without any
// per-flag wiring. Precedence is command line > config file > default: values in
// the file fill in only the flags the operator did not set explicitly on the
// command line. JSON (not YAML) keeps the dependency surface at zero — matching
// the project's distribution-simplicity goal and internal/persist's convention.
package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
)

// ApplyFile reads path as a JSON object mapping flag names to values and applies
// each to the matching flag in fs, unless that flag was already set on the
// command line (fs.Parse must have run first). An unknown key is an error so
// typos surface instead of being silently ignored.
func ApplyFile(fs *flag.FlagSet, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Flags explicitly set on the command line take precedence over the file.
	setOnCLI := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) { setOnCLI[f.Name] = true })

	for key, val := range raw {
		if fs.Lookup(key) == nil {
			return fmt.Errorf("config: unknown key %q in %s", key, path)
		}
		if setOnCLI[key] {
			continue // command line wins
		}
		if err := fs.Set(key, formatValue(val)); err != nil {
			return fmt.Errorf("config: key %q: %w", key, err)
		}
	}
	return nil
}

// formatValue renders a JSON-decoded value as the string flag.Set expects.
// JSON numbers decode to float64, so integral values must not gain a ".0"
// (e.g. an int flag would reject "5.0"); strconv with -1 precision handles this.
func formatValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", t)
	}
}
