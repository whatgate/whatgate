package dnsx

import "testing"

func TestNormalizeServerAddsDefaultPort(t *testing.T) {
	cases := map[string]string{
		"8.8.8.8":                   "8.8.8.8:53",                // bare IPv4 → :53
		"1.1.1.1:5353":              "1.1.1.1:5353",              // explicit port kept
		"2001:4860:4860::8888":      "[2001:4860:4860::8888]:53", // bare IPv6 bracketed + :53
		"[2001:4860:4860::8888]:53": "[2001:4860:4860::8888]:53", // already bracketed+port kept
		"dns.example.com":           "dns.example.com:53",        // hostname → :53
	}
	for in, want := range cases {
		got, err := normalizeServer(in)
		if err != nil {
			t.Errorf("normalizeServer(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeServer(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeServerRejectsEmpty(t *testing.T) {
	if _, err := normalizeServer(""); err == nil {
		t.Fatal("empty server should error")
	}
}

// An empty server means "use the system resolver": Resolver returns nil so the
// dialer's default resolution applies.
func TestResolverEmptyReturnsNil(t *testing.T) {
	r, err := Resolver("")
	if err != nil {
		t.Fatalf("Resolver(\"\"): %v", err)
	}
	if r != nil {
		t.Fatal("empty server should return a nil resolver (system default)")
	}
}

// A configured server yields a custom Go resolver.
func TestResolverConfigured(t *testing.T) {
	r, err := Resolver("8.8.8.8")
	if err != nil {
		t.Fatalf("Resolver: %v", err)
	}
	if r == nil {
		t.Fatal("configured server should return a non-nil resolver")
	}
	if !r.PreferGo || r.Dial == nil {
		t.Fatalf("resolver should use the Go resolver with a custom Dial: %+v", r)
	}
}
