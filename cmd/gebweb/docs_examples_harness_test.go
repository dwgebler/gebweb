package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureToolchain(t *testing.T) docToolchain {
	t.Helper()
	bin, framework := scaffoldToolchain(t)
	fixtures, err := filepath.Abs("testdata/doctest")
	if err != nil {
		t.Fatal(err)
	}
	return docToolchain{
		bin:       bin,
		framework: framework,
		contexts:  filepath.Join(fixtures, "contexts"),
		excerpts:  filepath.Join(fixtures, "excerpts"),
	}
}

func onlyExample(t *testing.T, chapter string) docExample {
	t.Helper()
	examples, problems := parseDocExamples(chapter)
	if len(problems) > 0 || len(examples) != 1 {
		t.Fatalf("want one well-formed example, got %d examples and problems %v", len(examples), problems)
	}
	return examples[0]
}

func TestManualExampleHarnessAcceptsEveryMarkerKind(t *testing.T) {
	tc := fixtureToolchain(t)
	raw, err := os.ReadFile(filepath.Join("testdata", "doctest", "chapter.md"))
	if err != nil {
		t.Fatal(err)
	}

	examples, problems := parseDocExamples(string(raw))
	var kinds []string
	var failures []string
	for _, ex := range examples {
		kinds = append(kinds, ex.kind)
		if err := tc.verify(t, ex); err != nil {
			failures = append(failures, err.Error())
		}
	}

	if len(problems) != 0 {
		t.Fatalf("unexpected marker problems: %v", problems)
	}
	if got := strings.Join(kinds, ","); got != "run,run,fragment,excerpt,skip,file,run,run,file,run" {
		t.Fatalf("kinds = %s", got)
	}
	if len(failures) != 0 {
		t.Fatalf("fixture examples failed:\n%s", strings.Join(failures, "\n"))
	}
}

func TestManualExampleHarnessReportsMarkerProblems(t *testing.T) {
	chapter := strings.Join([]string{
		"<!-- doctest:strict -->",
		"```gb", "let a = 1;", "```",
		"<!-- doctest:run -->", "```gb", "let b = 2;", "```", "no output block follows",
		"<!-- doctest:fragment -->", "```gb", "let c = 3;", "```",
		"<!-- doctest:excerpt src=file.gb -->", "```gb", "let d = 4;", "```",
		"<!-- doctest:skip -->", "```gb", "let e = 5;", "```",
		"<!-- doctest:maybe -->", "```gb", "let f = 6;", "```",
		"<!-- doctest:file nested/path.gb -->", "```gb", "let g = 7;", "```",
		"<!-- doctest:file ../escape.html -->", "```html", "<p></p>", "```",
	}, "\n")

	_, problems := parseDocExamples(chapter)

	var got []string
	for _, p := range problems {
		got = append(got, p.msg)
	}
	want := []string{
		"gb block has no doctest marker (run, file, fragment, excerpt or skip)",
		"doctest:run block is not followed by a ```text output block",
		"doctest:fragment needs context=<name>",
		"doctest:excerpt needs src=<path>#<region>",
		"doctest:skip needs a reason",
		`unknown doctest marker "maybe"`,
		"doctest:file needs a module file name such as notes.gb",
		"doctest:file needs a project-relative path such as templates/home.html",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("problems:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestManualExampleHarnessIgnoresUnmarkedBlocksOutsideStrictChapters(t *testing.T) {
	_, problems := parseDocExamples("```gb\nlet a = 1;\n```\n")

	if len(problems) != 0 {
		t.Fatalf("unexpected problems: %v", problems)
	}
}

func TestManualExampleHarnessRejectsWrongOutput(t *testing.T) {
	tc := fixtureToolchain(t)
	ex := onlyExample(t, "<!-- doctest:run -->\n```gb\nimport io;\n\nio.println(\"actual\");\n```\n\n```text\nexpected\n```\n")

	err := tc.verify(t, ex)

	if err == nil || !strings.Contains(err.Error(), "VM output differs") {
		t.Fatalf("want an output mismatch, got %v", err)
	}
}

func TestManualExampleHarnessRejectsCodeThatFailsCheck(t *testing.T) {
	tc := fixtureToolchain(t)
	ex := onlyExample(t, "<!-- doctest:run -->\n```gb\nimport io;\n\nio.println(undefinedName);\n```\n\n```text\n\n```\n")

	err := tc.verify(t, ex)

	if err == nil || !strings.Contains(err.Error(), "geblang check --strict failed") {
		t.Fatalf("want a check failure, got %v", err)
	}
}

func TestManualExampleHarnessRejectsBrokenFragment(t *testing.T) {
	tc := fixtureToolchain(t)
	ex := onlyExample(t, "<!-- doctest:fragment context=controller -->\n```gb\n@Get(\"/x\")\nfunc x(): MissingType {\n    return MissingType();\n}\n```\n")

	err := tc.verify(t, ex)

	if err == nil || !strings.Contains(err.Error(), "geblang check --strict failed") {
		t.Fatalf("want a check failure, got %v", err)
	}
}

func TestManualExampleHarnessRejectsDriftedExcerpt(t *testing.T) {
	tc := fixtureToolchain(t)
	ex := onlyExample(t, "<!-- doctest:excerpt src=source.gb#build -->\n```gb\nexport func build(): gebweb.GebwebApp {\n    return gebweb.app([]);\n}\n```\n")

	err := tc.verify(t, ex)

	if err == nil || !strings.Contains(err.Error(), "excerpt differs from source.gb#build") {
		t.Fatalf("want an excerpt drift, got %v", err)
	}
}

func TestSourceRegionDedentsNestedRegions(t *testing.T) {
	src := "class A {\n    /* doctest:region m */\n    func m(): int {\n        return 1;\n    }\n    /* doctest:endregion */\n}\n"

	got, ok := sourceRegion(src, "m")

	if !ok || got != "func m(): int {\n    return 1;\n}" {
		t.Fatalf("region = %q, %v", got, ok)
	}
}

func TestOutputBelowAllowsOneLeadInParagraph(t *testing.T) {
	wrapped := []string{"", "A lead-in that", "wraps onto two lines:", "", "```text", "out", "```"}
	twoParagraphs := []string{"", "First paragraph.", "", "Second paragraph.", "", "```text", "out", "```"}

	got, _, okWrapped := outputBelow(wrapped, 0)
	_, _, okTwo := outputBelow(twoParagraphs, 0)

	if !okWrapped || got != "out" {
		t.Fatalf("wrapped lead-in: %q, %v", got, okWrapped)
	}
	if okTwo {
		t.Fatal("two lead-in paragraphs were accepted")
	}
}
