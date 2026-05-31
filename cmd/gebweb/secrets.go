package main

// Secrets vault subcommands for `gebweb secrets`. Shares the wire
// format with the runtime EncryptedFileSecretsProvider in
// gebweb/src/secretstore.gb: AES-256-GCM over a JSON dict, with
// nonce||ciphertext concatenated, base64-encoded, chunked at 80
// columns, and wrapped between PEM-style BEGIN/END markers.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	secretsKeyEnv     = "GEBWEB_SECRETS_KEY"
	secretsDefaultKey = "config/secrets.key"
	secretsDefaultEnc = "config/secrets.enc"
	pemBegin          = "-----BEGIN GEBWEB SECRETS-----"
	pemEnd            = "-----END GEBWEB SECRETS-----"
	// Wire format uses 80-col chunks deliberately; encoding/pem hard-
	// codes 64 and can't be reused. Constant must stay in sync with
	// CHUNK_WIDTH in gebweb/src/secretstore.gb.
	pemChunkWidth = 80
	gcmNonceSize  = 12
	aesKeySize    = 32
)

// fail prints a uniformly-prefixed error to stderr and returns 1
// so handlers can `return fail(...)` in a single line.
func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "gebweb secrets: "+format+"\n", args...)
	return 1
}

// newGCM builds an AES-256-GCM AEAD from a 32-byte key.
func newGCM(key []byte) (cipher.AEAD, error) {
	if len(key) != aesKeySize {
		return nil, fmt.Errorf("AES-256 key must be %d bytes (got %d)", aesKeySize, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

type secretsOptions struct {
	keyPath string
	encPath string
	force   bool
}

func runSecrets(args []string) int {
	if hasHelpFlag(args) {
		printSecretsHelp(os.Stdout)
		return 0
	}
	if len(args) == 0 {
		printSecretsHelp(os.Stderr)
		return 2
	}
	sub, rest := args[0], args[1:]
	opts, rest, err := parseSecretsFlags(rest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb secrets: %v\n", err)
		return 2
	}
	switch sub {
	case "init":
		return runSecretsInit(opts)
	case "edit":
		return runSecretsEdit(opts)
	case "set":
		return runSecretsSet(opts, rest)
	case "get":
		return runSecretsGet(opts, rest)
	case "list":
		return runSecretsList(opts)
	default:
		fmt.Fprintf(os.Stderr, "gebweb secrets: unknown subcommand %q\n", sub)
		printSecretsHelp(os.Stderr)
		return 2
	}
}

func parseSecretsFlags(args []string) (secretsOptions, []string, error) {
	opts := secretsOptions{keyPath: secretsDefaultKey, encPath: secretsDefaultEnc}
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--key-file":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--key-file requires a path")
			}
			opts.keyPath = args[i+1]
			i++
		case "--file":
			if i+1 >= len(args) {
				return opts, nil, fmt.Errorf("--file requires a path")
			}
			opts.encPath = args[i+1]
			i++
		case "--force":
			opts.force = true
		default:
			rest = append(rest, args[i])
		}
	}
	return opts, rest, nil
}

func printSecretsHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb secrets <subcommand> [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "subcommands:")
	fmt.Fprintln(w, "  init                  generate a new AES-256 key and an empty vault")
	fmt.Fprintln(w, "  edit                  decrypt to a temp file, open $EDITOR, re-encrypt on save")
	fmt.Fprintln(w, "  set <name> <value>    non-interactively store one secret")
	fmt.Fprintln(w, "  get <name>            decrypt and print one secret to stdout")
	fmt.Fprintln(w, "  list                  print the names of stored secrets")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  --key-file <path>     override config/secrets.key (env GEBWEB_SECRETS_KEY still wins)")
	fmt.Fprintln(w, "  --file <path>         override config/secrets.enc")
	fmt.Fprintln(w, "  --force               overwrite existing files during init")
}

func runSecretsInit(opts secretsOptions) int {
	if !opts.force {
		if rc, refused := refuseIfExists(opts.keyPath); refused {
			return rc
		}
		if rc, refused := refuseIfExists(opts.encPath); refused {
			return rc
		}
	}
	key := make([]byte, aesKeySize)
	if _, err := rand.Read(key); err != nil {
		return fail("key generation failed: %v", err)
	}
	for _, dir := range uniqueDirs(opts.keyPath, opts.encPath) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fail("%v", err)
		}
	}
	if err := os.WriteFile(opts.keyPath, []byte(base64.StdEncoding.EncodeToString(key)+"\n"), 0o600); err != nil {
		return fail("writing key: %v", err)
	}
	body, err := encryptVault(map[string]string{}, key)
	if err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(opts.encPath, []byte(body), 0o600); err != nil {
		return fail("writing vault: %v", err)
	}
	fmt.Printf("Wrote %s (mode 0600) and %s\n", opts.keyPath, opts.encPath)
	fmt.Println("Add the key file to .gitignore and pass GEBWEB_SECRETS_KEY in production.")
	return 0
}

func refuseIfExists(path string) (int, bool) {
	if _, err := os.Stat(path); err == nil {
		return fail("%s already exists (use --force to overwrite)", path), true
	}
	return 0, false
}

func uniqueDirs(paths ...string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		d := filepath.Dir(p)
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	return out
}

func runSecretsSet(opts secretsOptions, rest []string) int {
	if len(rest) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gebweb secrets set <name> <value>")
		return 2
	}
	name, value := rest[0], rest[1]
	key, err := resolveSecretsKey(opts.keyPath)
	if err != nil {
		return fail("%v", err)
	}
	vault, err := loadVault(opts.encPath, key)
	if err != nil {
		return fail("%v", err)
	}
	vault[name] = value
	if err := saveVault(opts.encPath, vault, key); err != nil {
		return fail("%v", err)
	}
	return 0
}

func runSecretsGet(opts secretsOptions, rest []string) int {
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gebweb secrets get <name>")
		return 2
	}
	name := rest[0]
	key, err := resolveSecretsKey(opts.keyPath)
	if err != nil {
		return fail("%v", err)
	}
	vault, err := loadVault(opts.encPath, key)
	if err != nil {
		return fail("%v", err)
	}
	value, ok := vault[name]
	if !ok {
		return fail("unknown secret %q", name)
	}
	fmt.Println(value)
	return 0
}

func runSecretsList(opts secretsOptions) int {
	key, err := resolveSecretsKey(opts.keyPath)
	if err != nil {
		return fail("%v", err)
	}
	vault, err := loadVault(opts.encPath, key)
	if err != nil {
		return fail("%v", err)
	}
	names := make([]string, 0, len(vault))
	for k := range vault {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

func runSecretsEdit(opts secretsOptions) int {
	key, err := resolveSecretsKey(opts.keyPath)
	if err != nil {
		return fail("%v", err)
	}
	vault, err := loadVault(opts.encPath, key)
	if err != nil {
		return fail("%v", err)
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		return fail("$EDITOR is not set")
	}
	tmp, err := os.CreateTemp("", "gebweb-secrets-*.json")
	if err != nil {
		return fail("%v", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()
	pretty, err := json.MarshalIndent(vault, "", "  ")
	if err != nil {
		return fail("%v", err)
	}
	if _, err := tmp.Write(append(pretty, '\n')); err != nil {
		return fail("%v", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail("%v", err)
	}
	tmp.Close()

	cmd := exec.Command("sh", "-c", editor+" "+shellQuote(tmpPath))
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fail("editor exited with error: %v", err)
	}
	updated, err := os.ReadFile(tmpPath)
	if err != nil {
		return fail("%v", err)
	}
	var newVault map[string]string
	if err := json.Unmarshal(updated, &newVault); err != nil {
		return fail("edited file is not valid JSON: %v", err)
	}
	if err := saveVault(opts.encPath, newVault, key); err != nil {
		return fail("%v", err)
	}
	return 0
}

func resolveSecretsKey(keyPath string) ([]byte, error) {
	if env := os.Getenv(secretsKeyEnv); env != "" {
		return base64.StdEncoding.DecodeString(strings.TrimSpace(env))
	}
	data, err := os.ReadFile(keyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no key available (set %s or create %s)", secretsKeyEnv, keyPath)
		}
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
}

func loadVault(encPath string, key []byte) (map[string]string, error) {
	data, err := os.ReadFile(encPath)
	if err != nil {
		return nil, fmt.Errorf("reading vault: %w", err)
	}
	plain, err := decryptVault(string(data), key)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("vault is not a JSON object of strings: %w", err)
	}
	return out, nil
}

func saveVault(encPath string, vault map[string]string, key []byte) error {
	body, err := encryptVault(vault, key)
	if err != nil {
		return err
	}
	return os.WriteFile(encPath, []byte(body), 0o600)
}

func encryptVault(vault map[string]string, key []byte) (string, error) {
	plaintext, err := json.Marshal(vault)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	// Seal appends ciphertext to its first arg; pre-seed with the
	// nonce so the result is nonce||ct in one allocation.
	payload := make([]byte, gcm.NonceSize(), gcm.NonceSize()+len(plaintext)+gcm.Overhead())
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	payload = gcm.Seal(payload, payload[:gcm.NonceSize()], plaintext, nil)
	encoded := base64.StdEncoding.EncodeToString(payload)
	return pemBegin + "\n" + chunkBase64(encoded, pemChunkWidth) + "\n" + pemEnd + "\n", nil
}

func decryptVault(body string, key []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	beginIdx := strings.Index(body, pemBegin)
	endIdx := strings.Index(body, pemEnd)
	if beginIdx < 0 || endIdx < 0 || endIdx <= beginIdx {
		return nil, fmt.Errorf("file is missing BEGIN/END markers")
	}
	inner := body[beginIdx+len(pemBegin) : endIdx]
	stripped := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, inner)
	payload, err := base64.StdEncoding.DecodeString(stripped)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(payload) < gcmNonceSize {
		return nil, fmt.Errorf("ciphertext shorter than nonce")
	}
	plain, err := gcm.Open(nil, payload[:gcmNonceSize], payload[gcmNonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("authentication failed")
	}
	return plain, nil
}

func chunkBase64(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/width)
	for i := 0; i < len(s); i += width {
		if i > 0 {
			b.WriteByte('\n')
		}
		end := i + width
		if end > len(s) {
			end = len(s)
		}
		b.WriteString(s[i:end])
	}
	return b.String()
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
