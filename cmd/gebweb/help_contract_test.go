package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDevHelpDescribesSourceWatchScope(t *testing.T) {
	var output bytes.Buffer

	code := runHelp(&output, []string{"dev"})

	if code != 0 {
		t.Fatalf("help exit code: %d", code)
	}
	help := output.String()
	if !strings.Contains(help, ".gb or .yaml") || !strings.Contains(help, "src/") {
		t.Errorf("help must describe watched source files:\n%s", help)
	}
	if strings.Contains(help, ".html") || strings.Contains(help, "template-file change") {
		t.Errorf("help advertises unsupported template watching:\n%s", help)
	}
}

func TestGeneratedClientHelpMatchesGeneratedAPI(t *testing.T) {
	spec, err := loadSpec(petstoreFixturePath(t))
	if err != nil {
		t.Fatal(err)
	}
	spec.Components.SecuritySchemes["basicAuth"] = &apiSecurityScheme{Type: "http", Scheme: "basic"}
	generated, err := emitClient(spec, "Petstore")
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	code := runHelp(&output, []string{"generate-client"})

	if code != 0 {
		t.Fatalf("help exit code: %d", code)
	}
	help := output.String()
	for _, contract := range []string{
		"PetstoreClient(string baseUrl, dict<string, any> auth = {})",
		"bearerToken", "basicUser", "basicPassword", "apiKey", "RuntimeError",
	} {
		if !strings.Contains(help, contract) || !strings.Contains(generated, contract) {
			t.Errorf("help and generated API must both describe %q", contract)
		}
	}
	for _, obsolete := range []string{"errors.HttpException", "config dict's \"baseUrl\"", "security` clause"} {
		if strings.Contains(help, obsolete) {
			t.Errorf("help retains unsupported contract %q", obsolete)
		}
	}
}
