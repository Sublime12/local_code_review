package main

import "testing"

func TestParseFlagsDefaults(t *testing.T) {
	opts, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if opts.browser {
		t.Errorf("expected browser default to be false, got true")
	}
	if opts.port != 0 {
		t.Errorf("expected port default to be 0, got %d", opts.port)
	}
	if opts.ref != "" {
		t.Errorf("expected ref default to be empty, got %q", opts.ref)
	}
}

func TestParseFlagsBrowserOptIn(t *testing.T) {
	for _, arg := range []string{"-browser", "--browser"} {
		opts, err := parseFlags([]string{arg, "-port", "8080", "-ref", "main"})
		if err != nil {
			t.Fatalf("expected no error for %s, got %v", arg, err)
		}
		if !opts.browser {
			t.Errorf("expected browser to be true with %s, got false", arg)
		}
		if opts.port != 8080 {
			t.Errorf("expected port 8080, got %d", opts.port)
		}
		if opts.ref != "main" {
			t.Errorf("expected ref main, got %q", opts.ref)
		}
	}
}
