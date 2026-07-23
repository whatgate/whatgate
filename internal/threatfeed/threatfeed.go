// Package threatfeed fetches lists of known-malicious domains and parses them
// into a denylist an exit merges into its ExitGuard. It accepts both plain
// domain-per-line lists and hosts-file format ("0.0.0.0 baddomain.com"), the
// two common shapes public blocklists ship in.
package threatfeed

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// ParseList reads a domain list, tolerating blank lines, '#' comments, plain
// "domain.com" lines, and hosts-file lines ("0.0.0.0 domain.com").
func ParseList(r io.Reader) (map[string]bool, error) {
	out := make(map[string]bool)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Plain "domain" or hosts-file "IP domain" (take the last field).
		domain := strings.ToLower(fields[len(fields)-1])
		if domain == "" || domain == "localhost" {
			continue
		}
		out[domain] = true
	}
	return out, sc.Err()
}

// Fetch retrieves and parses a domain list from source, which may be an
// http(s):// URL or a local file path.
func Fetch(source string) (map[string]bool, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("threatfeed: GET %s: %s", source, resp.Status)
		}
		return ParseList(resp.Body)
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseList(f)
}
