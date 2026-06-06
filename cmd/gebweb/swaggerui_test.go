package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestVendorSwaggerUI checks the cache contract: missing files are fetched once,
// and a second call is a cache hit with no further fetches.
func TestVendorSwaggerUI(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	fetched := map[string]int{}
	fetch := func(url, dest string) error {
		fetched[filepath.Base(dest)]++
		return os.WriteFile(dest, []byte("/* "+filepath.Base(dest)+" */"), 0o644)
	}

	dir, err := vendorSwaggerUI(fetch)
	if err != nil {
		t.Fatalf("vendorSwaggerUI: %v", err)
	}
	for _, name := range swaggerUIFiles {
		if !fileExists(filepath.Join(dir, name)) {
			t.Errorf("missing cached file %q", name)
		}
		if fetched[name] != 1 {
			t.Errorf("%q fetched %d times, want 1", name, fetched[name])
		}
	}

	// Second call: cache hit, no new fetches.
	if _, err := vendorSwaggerUI(fetch); err != nil {
		t.Fatalf("vendorSwaggerUI (cached): %v", err)
	}
	for _, name := range swaggerUIFiles {
		if fetched[name] != 1 {
			t.Errorf("%q re-fetched on cache hit (%d)", name, fetched[name])
		}
	}
}

// TestVendorSwaggerUIFetchError surfaces an actionable offline message.
func TestVendorSwaggerUIFetchError(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	fetch := func(url, dest string) error { return os.ErrDeadlineExceeded }
	if _, err := vendorSwaggerUI(fetch); err == nil {
		t.Fatal("expected error when fetch fails")
	} else if !strings.Contains(err.Error(), "--no-swagger") {
		t.Errorf("error should mention --no-swagger: %v", err)
	}
}
