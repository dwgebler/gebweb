package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
	"github.com/tdewolff/minify/v2"
	mhtml "github.com/tdewolff/minify/v2/html"
	"gopkg.in/yaml.v3"
)

// assetsConfig is the gebweb-specific `assets:` block read from geblang.yaml.
// The engine ignores it; gebweb processes sources here and embeds the output
// via `geblang build --resource`.
type assetsConfig struct {
	SourceDir    string   `yaml:"sourceDir"`
	OutDir       string   `yaml:"outDir"`
	EntryPoints  []string `yaml:"entryPoints"`
	TemplatesDir string   `yaml:"templatesDir"`
	PublicDir    string   `yaml:"publicDir"`
}

type manifestAssets struct {
	Assets *assetsConfig `yaml:"assets"`
}

// assetBuildOptions controls one pipeline run.
type assetBuildOptions struct {
	minify bool
	noSass bool
}

// stageDir holds processed template copies so the source tree is never touched.
const stageDir = "build/.gebweb"

// readAssetsConfig parses the `assets:` block from geblang.yaml. It returns nil
// when the file has no assets block.
func readAssetsConfig(manifestPath string) (*assetsConfig, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var m manifestAssets
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	if m.Assets == nil {
		return nil, nil
	}
	cfg := m.Assets
	if cfg.TemplatesDir == "" {
		cfg.TemplatesDir = "templates"
	}
	if cfg.PublicDir == "" {
		cfg.PublicDir = "public"
	}
	return cfg, nil
}

// buildAssets runs the asset pipeline for cfg and returns the --resource specs
// that `geblang build` should embed (compiled assets, minified templates, and
// static public files). With cfg nil it still embeds templates/public when they
// exist, so a template-only project bundles correctly.
func buildAssets(cfg *assetsConfig, opts assetBuildOptions) ([]string, error) {
	templatesDir := "templates"
	publicDir := "public"
	if cfg != nil {
		templatesDir = cfg.TemplatesDir
		publicDir = cfg.PublicDir
		if err := compileEntryPoints(cfg, opts); err != nil {
			return nil, err
		}
	}

	var resources []string
	if cfg != nil && cfg.OutDir != "" && dirExists(cfg.OutDir) {
		resources = append(resources, cfg.OutDir)
	}

	if dirExists(templatesDir) {
		staged := filepath.Join(stageDir, "templates")
		if err := stageTemplates(templatesDir, staged, opts.minify); err != nil {
			return nil, err
		}
		resources = append(resources, staged+"="+filepath.ToSlash(templatesDir))
	}

	if dirExists(publicDir) {
		resources = append(resources, publicDir)
	}

	return resources, nil
}

// compileEntryPoints runs esbuild (and dart-sass for SASS) over each entry,
// writing compiled output into cfg.OutDir.
func compileEntryPoints(cfg *assetsConfig, opts assetBuildOptions) error {
	if len(cfg.EntryPoints) == 0 {
		return nil
	}
	if cfg.OutDir == "" {
		return fmt.Errorf("assets: outDir is required when entryPoints are set")
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return err
	}

	for _, entry := range cfg.EntryPoints {
		src := filepath.Join(cfg.SourceDir, entry)
		if !fileExists(src) {
			return fmt.Errorf("assets: entry point %q not found", src)
		}
		ext := strings.ToLower(filepath.Ext(entry))
		base := strings.TrimSuffix(filepath.Base(entry), filepath.Ext(entry))
		switch ext {
		case ".scss", ".sass":
			out := filepath.Join(cfg.OutDir, base+".css")
			if err := compileSass(src, out, opts); err != nil {
				return err
			}
		case ".css":
			out := filepath.Join(cfg.OutDir, base+".css")
			if err := bundleWithEsbuild(src, out, opts.minify); err != nil {
				return err
			}
		case ".js", ".jsx", ".ts", ".tsx":
			out := filepath.Join(cfg.OutDir, base+".js")
			if err := bundleWithEsbuild(src, out, opts.minify); err != nil {
				return err
			}
		default:
			return fmt.Errorf("assets: unsupported entry point %q (want .js/.ts/.jsx/.tsx/.css/.scss/.sass)", entry)
		}
	}
	return nil
}

// bundleWithEsbuild bundles+minifies a JS/TS/CSS entry to outFile.
func bundleWithEsbuild(src, outFile string, doMinify bool) error {
	result := esbuild.Build(esbuild.BuildOptions{
		EntryPoints:       []string{src},
		Outfile:           outFile,
		Bundle:            true,
		Write:             true,
		MinifyWhitespace:  doMinify,
		MinifyIdentifiers: doMinify,
		MinifySyntax:      doMinify,
		LogLevel:          esbuild.LogLevelSilent,
	})
	if len(result.Errors) > 0 {
		msgs := esbuild.FormatMessages(result.Errors, esbuild.FormatMessagesOptions{})
		return fmt.Errorf("esbuild %s:\n%s", src, strings.Join(msgs, ""))
	}
	return nil
}

// compileSass shells out to dart-sass, then minifies the result with esbuild.
func compileSass(src, outFile string, opts assetBuildOptions) error {
	sassBin := findSass()
	if sassBin == "" {
		if opts.noSass {
			return nil
		}
		return fmt.Errorf("%s needs dart-sass, not found on PATH.\n  install: https://sass-lang.com/install\n  or pass --no-sass to skip SASS this build", src)
	}
	cssOut, err := exec.Command(sassBin, "--no-source-map", src).Output()
	if err != nil {
		return fmt.Errorf("dart-sass %s: %w", src, err)
	}
	transform := esbuild.Transform(string(cssOut), esbuild.TransformOptions{
		Loader:           esbuild.LoaderCSS,
		MinifyWhitespace: opts.minify,
		MinifySyntax:     opts.minify,
		LogLevel:         esbuild.LogLevelSilent,
	})
	if len(transform.Errors) > 0 {
		msgs := esbuild.FormatMessages(transform.Errors, esbuild.FormatMessagesOptions{})
		return fmt.Errorf("minify %s:\n%s", src, strings.Join(msgs, ""))
	}
	if err := os.MkdirAll(filepath.Dir(outFile), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outFile, transform.Code, 0o644)
}

// stageTemplates copies templates from srcDir to stagedDir, minifying .html
// when doMinify is set. tdewolff's HTML minifier preserves template tags
// (`{{ }}`, `{% %}`) as text.
func stageTemplates(srcDir, stagedDir string, doMinify bool) error {
	if err := os.RemoveAll(stagedDir); err != nil {
		return err
	}
	var m *minify.M
	if doMinify {
		m = minify.New()
		m.AddFunc("text/html", mhtml.Minify)
	}
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(stagedDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if m != nil && strings.EqualFold(filepath.Ext(path), ".html") {
			out, mErr := m.Bytes("text/html", data)
			if mErr != nil {
				return fmt.Errorf("minify template %s: %w", path, mErr)
			}
			data = out
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

func findSass() string {
	for _, name := range []string{"sass", "dart-sass"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
