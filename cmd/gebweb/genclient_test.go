package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// petstoreFixturePath returns the on-disk path to the YAML spec we
// generate against in these tests; running with go test always
// resolves the testdata/ dir relative to the package.
func petstoreFixturePath(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs("testdata/petstore.yaml")
	if err != nil {
		t.Fatalf("resolve fixture: %v", err)
	}
	return abs
}

func generateFromFixture(t *testing.T, name string) string {
	t.Helper()
	spec, err := loadSpec(petstoreFixturePath(t))
	if err != nil {
		t.Fatalf("loadSpec: %v", err)
	}
	out, err := emitClient(spec, name)
	if err != nil {
		t.Fatalf("emitClient: %v", err)
	}
	return out
}

func TestGenerateClientEmitsDTOsForComponentSchemas(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	for _, want := range []string{
		"export class Pet {",
		"export class NewPet {",
		"export class Error {",
		"int id;",
		"string name;",
		"?string tag;",
		"?decimal price;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGenerateClientEmitsClientClassWithAuth(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	for _, want := range []string{
		"export class PetstoreClient {",
		"func PetstoreClient(string baseUrl, dict<string, any> auth = {})",
		"?string bearerToken;",
		"?string apiKey;",
		`h["Authorization"] = "Bearer "`,
		`h["X-API-Key"] = this.apiKey as string;`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestGenerateClientOneMethodPerOperation(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	for _, want := range []string{
		"func listPets(",
		"func createPet(NewPet body): Pet {",
		"func getPet(string petId): Pet {",
		"func deletePet(string petId)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing method %q in:\n%s", want, out)
		}
	}
}

func TestGenerateClientPathSubstitution(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	if !strings.Contains(out, `string path = "/pets/" + (petId as string)`) {
		t.Errorf("expected path interpolation for {petId} in:\n%s", out)
	}
}

func TestGenerateClientQueryParamsRenderConditionally(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	if !strings.Contains(out, "if (limit != null) {") {
		t.Errorf("expected null-guarded query param emission in:\n%s", out)
	}
	if !strings.Contains(out, "qs.length() > 0") {
		t.Errorf("expected query string is built from list in:\n%s", out)
	}
}

func TestGenerateClientResponseDispatch(t *testing.T) {
	out := generateFromFixture(t, "Petstore")
	// List of Pet response: list<Pet> via json.parse
	if !strings.Contains(out, "return json.parse(r[\"body\"] as string) as list<Pet>;") {
		t.Errorf("expected list<Pet> response decoding in:\n%s", out)
	}
	// Single Pet response: parseAs
	if !strings.Contains(out, "return json.parseAs(r[\"body\"] as string, Pet);") {
		t.Errorf("expected json.parseAs for typed response in:\n%s", out)
	}
}

func TestGenerateClientAcceptsJSON(t *testing.T) {
	jsonSpec := `{"openapi": "3.0.0", "info": {"title": "Tiny", "version": "1.0"},
		"paths": {"/ping": {"get": {"operationId": "ping",
		"responses": {"200": {"description": "ok"}}}}}}`
	tmp, err := os.CreateTemp("", "spec-*.json")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer os.Remove(tmp.Name())
	tmp.WriteString(jsonSpec)
	tmp.Close()
	spec, err := loadSpec(tmp.Name())
	if err != nil {
		t.Fatalf("loadSpec(json): %v", err)
	}
	out, err := emitClient(spec, "Tiny")
	if err != nil {
		t.Fatalf("emitClient: %v", err)
	}
	if !strings.Contains(out, "func ping(): string {") {
		t.Errorf("expected ping method emitted from JSON spec; got:\n%s", out)
	}
}

func TestRunGenerateClientWritesFile(t *testing.T) {
	// Resolve the fixture before chdir; otherwise filepath.Abs picks up
	// the new working directory and points at a path that doesn't exist.
	spec := petstoreFixturePath(t)
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runGenerateClient([]string{spec, "Petstore"}); code != 0 {
		t.Fatalf("runGenerateClient exit code: %d", code)
	}
	if _, err := os.Stat(filepath.Join("src", "petstore_client.gb")); err != nil {
		t.Errorf("expected src/petstore_client.gb to be written: %v", err)
	}
}

func TestRunGenerateClientRejectsBadName(t *testing.T) {
	spec := petstoreFixturePath(t)
	dir := t.TempDir()
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if code := runGenerateClient([]string{spec, "lowerName"}); code != 2 {
		t.Errorf("expected exit 2 for bad name, got %d", code)
	}
}

// TestGeneratedClientParsesViaGeblang verifies the emitter output
// survives the Geblang parser + semantic analyser end-to-end. Runs
// `geblang check` on the generated file. Skipped when the geblang
// binary isn't on PATH (i.e. when running these tests in isolation
// from the main `make build` workflow).
func TestGeneratedClientParsesViaGeblang(t *testing.T) {
	if _, err := exec.LookPath("geblang"); err != nil {
		// Fall back to ../../geblang built by `make build`.
		fallback, _ := filepath.Abs("../../geblang")
		if _, err := os.Stat(fallback); err != nil {
			t.Skip("geblang binary not available")
		}
		t.Setenv("PATH", filepath.Dir(fallback)+":"+os.Getenv("PATH"))
	}
	dir := t.TempDir()
	out := generateFromFixture(t, "Petstore")
	path := filepath.Join(dir, "petstore_client.gb")
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		t.Fatalf("write generated: %v", err)
	}
	cmd := exec.Command("geblang", "check", path)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("geblang check failed:\n%s", string(combined))
	}
}

func TestPascalAndCamelCaseHandleEdgeCases(t *testing.T) {
	cases := []struct {
		in, pascal, camel string
	}{
		{"user_profile", "UserProfile", "userProfile"},
		{"x-api-key", "XApiKey", "xApiKey"},
		{"42abc", "X42abc", "x42abc"},
		{"", "X", "x"},
	}
	for _, c := range cases {
		if got := pascal(c.in); got != c.pascal {
			t.Errorf("pascal(%q) = %q, want %q", c.in, got, c.pascal)
		}
		if got := camelCase(c.in); got != c.camel {
			t.Errorf("camelCase(%q) = %q, want %q", c.in, got, c.camel)
		}
	}
}
