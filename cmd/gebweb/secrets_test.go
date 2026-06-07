package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeKeyFile(t *testing.T, path string, key []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir key dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

func randKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, aesKeySize)
	if _, err := rand.Read(k); err != nil {
		t.Fatalf("rand: %v", err)
	}
	return k
}

// setupVault produces a temp-dir secretsOptions with a fresh key and
// an empty (but valid) encrypted vault, ready for set/get/list tests.
func setupVault(t *testing.T) secretsOptions {
	t.Helper()
	dir := t.TempDir()
	opts := secretsOptions{
		keyPath: filepath.Join(dir, "secrets.key"),
		encPath: filepath.Join(dir, "secrets.enc"),
	}
	key := randKey(t)
	writeKeyFile(t, opts.keyPath, key)
	body, err := encryptVault(map[string]string{}, key)
	if err != nil {
		t.Fatalf("seed vault: %v", err)
	}
	if err := os.WriteFile(opts.encPath, []byte(body), 0o600); err != nil {
		t.Fatalf("write vault: %v", err)
	}
	return opts
}

func TestSecretsInitCreatesKeyAndVault(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secrets.key")
	encPath := filepath.Join(dir, "secrets.enc")
	if rc := runSecretsInit(secretsOptions{keyPath: keyPath, encPath: encPath}); rc != 0 {
		t.Fatalf("init returned %d", rc)
	}
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("key mode = %v, want 0600", info.Mode().Perm())
	}
	keyContent, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyContent)))
	if err != nil {
		t.Fatalf("key is not base64: %v", err)
	}
	if len(decoded) != aesKeySize {
		t.Errorf("key length = %d, want %d", len(decoded), aesKeySize)
	}
	body, err := os.ReadFile(encPath)
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	if !strings.Contains(string(body), pemBegin) || !strings.Contains(string(body), pemEnd) {
		t.Errorf("vault missing PEM markers:\n%s", body)
	}
}

func TestSecretsInitRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secrets.key")
	encPath := filepath.Join(dir, "secrets.enc")
	if err := os.WriteFile(keyPath, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if rc := runSecretsInit(secretsOptions{keyPath: keyPath, encPath: encPath}); rc == 0 {
		t.Errorf("init returned 0 with existing key file, want nonzero")
	}
	keyContent, _ := os.ReadFile(keyPath)
	if string(keyContent) != "pre-existing" {
		t.Errorf("key file was overwritten without --force: %q", keyContent)
	}
}

func TestSecretsInitForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "secrets.key")
	encPath := filepath.Join(dir, "secrets.enc")
	if err := os.WriteFile(keyPath, []byte("pre-existing"), 0o600); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if rc := runSecretsInit(secretsOptions{keyPath: keyPath, encPath: encPath, force: true}); rc != 0 {
		t.Fatalf("init --force returned %d", rc)
	}
	keyContent, _ := os.ReadFile(keyPath)
	if string(keyContent) == "pre-existing" {
		t.Errorf("key file was NOT overwritten with --force")
	}
}

func TestSecretsSetThenGetRoundTrip(t *testing.T) {
	opts := setupVault(t)
	if rc := runSecretsSet(opts, []string{"stripe.key", "sk_test_xyz"}); rc != 0 {
		t.Fatalf("set returned %d", rc)
	}
	value := captureStdout(t, func() int { return runSecretsGet(opts, []string{"stripe.key"}) })
	if strings.TrimSpace(value) != "sk_test_xyz" {
		t.Errorf("get returned %q, want sk_test_xyz", strings.TrimSpace(value))
	}
}

func TestSecretsSetUpdatesExistingName(t *testing.T) {
	opts := setupVault(t)
	runSecretsSet(opts, []string{"name", "first"})
	runSecretsSet(opts, []string{"name", "second"})
	value := captureStdout(t, func() int { return runSecretsGet(opts, []string{"name"}) })
	if strings.TrimSpace(value) != "second" {
		t.Errorf("get returned %q, want second", strings.TrimSpace(value))
	}
}

func TestSecretsListSortedNames(t *testing.T) {
	opts := setupVault(t)
	runSecretsSet(opts, []string{"zeta", "1"})
	runSecretsSet(opts, []string{"alpha", "2"})
	runSecretsSet(opts, []string{"mu", "3"})
	output := captureStdout(t, func() int { return runSecretsList(opts) })
	got := strings.Split(strings.TrimSpace(output), "\n")
	want := []string{"alpha", "mu", "zeta"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("list = %v, want %v", got, want)
	}
}

func TestSecretsGetUnknownNameReturnsError(t *testing.T) {
	opts := setupVault(t)
	if rc := runSecretsGet(opts, []string{"absent"}); rc == 0 {
		t.Errorf("get on absent name returned 0, want nonzero")
	}
}

func TestSecretsEnvKeyOverridesFile(t *testing.T) {
	dir := t.TempDir()
	fileKey := randKey(t)
	envKey := randKey(t)
	keyPath := filepath.Join(dir, "secrets.key")
	writeKeyFile(t, keyPath, fileKey)
	t.Setenv(secretsKeyEnv, base64.StdEncoding.EncodeToString(envKey))
	resolved, err := resolveSecretsKey(keyPath)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if base64.StdEncoding.EncodeToString(resolved) != base64.StdEncoding.EncodeToString(envKey) {
		t.Errorf("env key did not win over file key")
	}
}

func TestSecretsMissingKeyReturnsClearError(t *testing.T) {
	t.Setenv(secretsKeyEnv, "")
	_, err := resolveSecretsKey(filepath.Join(t.TempDir(), "absent.key"))
	if err == nil {
		t.Fatalf("resolve returned no error for missing key")
	}
	if !strings.Contains(err.Error(), secretsKeyEnv) {
		t.Errorf("error %q does not mention %s", err, secretsKeyEnv)
	}
}

func TestSecretsWireFormatChunkWidth(t *testing.T) {
	key := randKey(t)
	vault := map[string]string{"a": strings.Repeat("x", 256), "b": "y"}
	body, err := encryptVault(vault, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if len(line) > pemChunkWidth {
			t.Errorf("line %d is %d chars (> %d): %q", i, len(line), pemChunkWidth, line)
		}
	}
}

func TestSecretsWireFormatRoundTrip(t *testing.T) {
	key := randKey(t)
	vault := map[string]string{
		"stripe.key":   "sk_test_abc",
		"github.token": "ghp_xyz",
		"empty":        "",
	}
	body, err := encryptVault(vault, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	plain, err := decryptVault(body, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(plain, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k, v := range vault {
		if got[k] != v {
			t.Errorf("round-trip %s: got %q want %q", k, got[k], v)
		}
	}
}

func TestSecretsWrongKeyFailsAuthentication(t *testing.T) {
	right := randKey(t)
	wrong := randKey(t)
	body, err := encryptVault(map[string]string{"x": "y"}, right)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := decryptVault(body, wrong); err == nil {
		t.Errorf("decrypt with wrong key returned no error")
	}
}

func TestSecretsTamperedCiphertextRejected(t *testing.T) {
	key := randKey(t)
	body, err := encryptVault(map[string]string{"x": "y"}, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	beforeIdx := strings.Index(body, pemBegin) + len(pemBegin) + 1
	// Replace the first ciphertext byte with a different base64 char; "A" alone
	// is a no-op when the byte is already 'A' (the source of past flakiness).
	repl := byte('A')
	if body[beforeIdx] == repl {
		repl = 'B'
	}
	tampered := body[:beforeIdx] + string(repl) + body[beforeIdx+1:]
	if _, err := decryptVault(tampered, key); err == nil {
		t.Errorf("decrypt of tampered body returned no error")
	}
}

func TestSecretsParseFlags(t *testing.T) {
	opts, rest, err := parseSecretsFlags([]string{"name", "value", "--key-file", "/a", "--file", "/b", "--force"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if opts.keyPath != "/a" || opts.encPath != "/b" || !opts.force {
		t.Errorf("opts not parsed: %+v", opts)
	}
	if len(rest) != 2 || rest[0] != "name" || rest[1] != "value" {
		t.Errorf("rest = %v, want [name value]", rest)
	}
}

func TestSecretsParseFlagsMissingValueErrors(t *testing.T) {
	if _, _, err := parseSecretsFlags([]string{"--key-file"}); err == nil {
		t.Errorf("missing --key-file value returned no error")
	}
}

func TestChunkBase64Boundaries(t *testing.T) {
	if got := chunkBase64("abc", 5); got != "abc" {
		t.Errorf("short string = %q, want abc", got)
	}
	if got := chunkBase64("abcde", 5); got != "abcde" {
		t.Errorf("exact length = %q, want abcde", got)
	}
	if got := chunkBase64("abcdef", 5); got != "abcde\nf" {
		t.Errorf("overflow = %q, want abcde\\nf", got)
	}
}

func captureStdout(t *testing.T, fn func() int) string {
	t.Helper()
	r, w, _ := os.Pipe()
	orig := os.Stdout
	os.Stdout = w
	rc := fn()
	w.Close()
	os.Stdout = orig
	if rc != 0 {
		t.Errorf("returned %d", rc)
	}
	data, _ := io.ReadAll(r)
	return string(data)
}
