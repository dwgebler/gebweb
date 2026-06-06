package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// swaggerUIVersion is the pinned swagger-ui-dist release. Keep in sync with
// gebweb/src/swaggerui.gb (the dev/CDN path uses the same version).
const swaggerUIVersion = "5.17.14"

// swaggerBundleDir is the bundle path the embedded assets are mapped to; it
// matches embeddedDir in gebweb/src/swaggerui.gb.
const swaggerBundleDir = "_swaggerui"

var swaggerUIFiles = []string{"swagger-ui.css", "swagger-ui-bundle.js"}

// vendorSwaggerUI ensures the pinned swagger-ui assets are cached under the user
// cache dir and returns that directory. Missing files are fetched via fetch
// (one online build populates the cache; later/offline builds reuse it).
func vendorSwaggerUI(fetch func(url, dest string) error) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cacheRoot, "gebweb", "swagger-ui", swaggerUIVersion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	for _, name := range swaggerUIFiles {
		dest := filepath.Join(dir, name)
		if fileExists(dest) {
			continue
		}
		url := fmt.Sprintf("https://cdn.jsdelivr.net/npm/swagger-ui-dist@%s/%s", swaggerUIVersion, name)
		if err := fetch(url, dest); err != nil {
			return "", fmt.Errorf("fetch swagger-ui %s: %w\n  (offline? the pinned swagger-ui is cached after one online build; or pass --no-swagger)", name, err)
		}
	}
	return dir, nil
}

// httpDownload fetches url to dest, writing through a temp file so a failed
// download never leaves a partial file in the cache.
func httpDownload(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dest)
}
