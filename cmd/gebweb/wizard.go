package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// newOptions captures the `gebweb new` wizard choices.
type newOptions struct {
	name   string
	typ    string // app | api
	db     string // sqlite | postgres | pgvector | mysql
	docker bool
	port   int
}

func runNew(args []string) int {
	if hasHelpFlag(args) {
		printNewHelp(os.Stdout)
		return 0
	}

	opts := newOptions{typ: "app", db: "sqlite", port: 8080}
	set := map[string]bool{}
	yes := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--type" && i+1 < len(args):
			i++
			opts.typ = args[i]
			set["type"] = true
		case a == "--db" && i+1 < len(args):
			i++
			opts.db = args[i]
			set["db"] = true
		case a == "--port" && i+1 < len(args):
			i++
			p, err := strconv.Atoi(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "gebweb new: invalid --port %q\n", args[i])
				return 2
			}
			opts.port = p
			set["port"] = true
		case a == "--docker":
			opts.docker = true
			set["docker"] = true
		case a == "--no-docker":
			opts.docker = false
			set["docker"] = true
		case a == "--yes" || a == "-y":
			yes = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "gebweb new: unknown flag %s\n", a)
			return 2
		default:
			if opts.name == "" {
				opts.name = a
				set["name"] = true
			}
		}
	}

	if err := fillOptions(&opts, set, isInteractive() && !yes); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb new: %v\n", err)
		return 2
	}
	if opts.name == "" {
		fmt.Fprintln(os.Stderr, "gebweb new: a project name is required")
		return 2
	}
	if opts.typ != "app" && opts.typ != "api" {
		fmt.Fprintf(os.Stderr, "gebweb new: unknown --type %q (want app or api)\n", opts.typ)
		return 2
	}
	if !validDBs[opts.db] {
		fmt.Fprintf(os.Stderr, "gebweb new: unknown --db %q (want sqlite, postgres, pgvector, or mysql)\n", opts.db)
		return 2
	}
	if _, err := os.Stat(opts.name); err == nil {
		fmt.Fprintf(os.Stderr, "gebweb new: %s already exists\n", opts.name)
		return 1
	}

	if err := scaffoldProject(opts); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb new: %v\n", err)
		return 1
	}

	fmt.Printf("scaffolded %s/ (%s, %s)\n", opts.name, opts.typ, opts.db)
	fmt.Printf("\n  cd %s\n  gebweb dev\n\n", opts.name)
	return 0
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// fillOptions prompts for any option not set by a flag when interactive; in
// non-interactive mode it leaves defaults in place (but a name is still
// required, enforced by the caller).
func fillOptions(opts *newOptions, set map[string]bool, interactive bool) error {
	if !interactive {
		return nil
	}
	r := bufio.NewReader(os.Stdin)
	if !set["name"] {
		opts.name = strings.TrimSpace(promptLine(r, "Project name", ""))
	}
	if !set["type"] {
		opts.typ = promptChoice(r, "Project type", []string{"app", "api"}, "app")
	}
	if !set["db"] {
		opts.db = promptChoice(r, "Database", []string{"sqlite", "postgres", "pgvector", "mysql"}, "sqlite")
	}
	if !set["docker"] {
		opts.docker = promptYesNo(r, "Generate Docker files?", false)
	}
	if !set["port"] {
		s := promptLine(r, "Port", strconv.Itoa(opts.port))
		if p, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			opts.port = p
		}
	}
	return nil
}

func promptLine(r *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s (%s): ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptChoice(r *bufio.Reader, label string, choices []string, def string) string {
	fmt.Printf("%s [%s] (%s): ", label, strings.Join(choices, "/"), def)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptYesNo(r *bufio.Reader, label string, def bool) bool {
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, hint)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return def
	}
	return line == "y" || line == "yes"
}

func scaffoldProject(opts newOptions) error {
	dirs := []string{filepath.Join(opts.name, "src")}
	if opts.typ == "app" {
		dirs = append(dirs, filepath.Join(opts.name, "templates"), filepath.Join(opts.name, "assets"))
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	files := map[string]string{
		"geblang.yaml":     manifestContent(opts),
		".env":             envContent(opts),
		".gitignore":       "/build/\n*.db\n.env\n",
		"README.md":        readmeContent(opts),
		"src/widget.gb":    modelContent(),
		"src/widget_repository.gb": repositoryContent(),
		"src/main.gb":      mainContent(opts),
		"src/main_test.gb": testContent(opts),
	}
	if opts.typ == "app" {
		files["src/home_controller.gb"] = appControllerContent()
		files["templates/page.html"] = templateContent(opts.name)
		files["assets/app.ts"] = "const ready = () => document.querySelector(\"h1\")?.classList.add(\"loaded\");\nready();\n"
		files["assets/app.css"] = "body { font-family: system-ui, sans-serif; margin: 2rem; }\nh1.loaded { color: teal; }\n"
	} else {
		files["src/widget_controller.gb"] = apiControllerContent()
	}

	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(opts.name, rel), []byte(content), 0o644); err != nil {
			return err
		}
	}

	if opts.docker {
		prev, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := os.Chdir(opts.name); err != nil {
			return err
		}
		defer os.Chdir(prev)
		if err := generateDocker(dockerOptions{db: opts.db, port: opts.port, binary: "build/app"}); err != nil {
			return err
		}
	}
	return nil
}

func dbDriver(db string) string {
	if db == "pgvector" {
		return "postgres"
	}
	return db
}

func dbDSN(db string) string {
	switch db {
	case "postgres", "pgvector":
		return "postgres://app:app@localhost:5432/app?sslmode=disable"
	case "mysql":
		return "app:app@tcp(localhost:3306)/app"
	default:
		return "./app.db"
	}
}

func manifestContent(opts newOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\nversion: 0.1.0\nsource: src\n", opts.name)
	if opts.typ == "app" {
		b.WriteString("assets:\n")
		b.WriteString("  sourceDir: assets\n")
		b.WriteString("  outDir: build/assets\n")
		b.WriteString("  entryPoints:\n")
		b.WriteString("    - app.ts\n")
		b.WriteString("    - app.css\n")
	}
	return b.String()
}

func envContent(opts newOptions) string {
	return fmt.Sprintf("GEBWEB_PORT=%d\nDB_DRIVER=%s\nDATABASE_URL=%s\n",
		opts.port, dbDriver(opts.db), dbDSN(opts.db))
}

func modelContent() string {
	return `module widget;

/* Sample model. Replace its fields and add validation decorators as needed. */
export class Widget {
    string id;
    string name;

    func Widget(string id, string name) {
        this.id = id;
        this.name = name;
    }
}
`
}

func repositoryContent() string {
	return `module widget_repository;

import db;

/* In-memory sample repository. The database connection is injected and
 * stored, ready for db.query / db.exec when you move to real persistence. */
export class WidgetRepository {
    db.Connection conn;
    dict<string, dict<string, any>> store;

    func WidgetRepository(db.Connection conn) {
        this.conn = conn;
        this.store = {"1": {"id": "1", "name": "Sample widget"}};
    }

    func findAll(): list<any> {
        list<any> out = [];
        for (key in this.store.keys()) {
            out = out.push(this.store[key]);
        }
        return out;
    }

    func findById(string id): ?dict<string, any> {
        return this.store.contains(id) ? this.store[id] : null;
    }

    func save(dict<string, any> widget): void {
        this.store[widget["id"] as string] = widget;
    }
}
`
}

func appControllerContent() string {
	return `module home_controller;

import gebweb;
import widget_repository as repo;

/* Server-rendered controller. Renders templates/page.html via the base
 * Controller's this.view, which threads the framework view context. */
export class HomeController extends gebweb.Controller {
    repo.WidgetRepository repository;

    func HomeController(repo.WidgetRepository repository) {
        parent();
        this.repository = repository;
    }

    @Get("/")
    @Summary("Home page")
    func index(gebweb.Request request): gebweb.Response {
        return this.view(request, "page.html", {
            "title": "Welcome",
            "widgets": this.repository.findAll(),
        });
    }
}
`
}

func apiControllerContent() string {
	return `module widget_controller;

import gebweb;
import widget as model;
import widget_repository as repo;

/* JSON controller. Handlers return gebweb.Response via the base Controller's
 * response builders (this.json / this.notFound / this.created). */
export class WidgetController extends gebweb.Controller {
    repo.WidgetRepository repository;

    func WidgetController(repo.WidgetRepository repository) {
        parent();
        this.repository = repository;
    }

    @Get("/widgets")
    @Summary("List widgets")
    func list(): gebweb.Response {
        return this.json(this.repository.findAll());
    }

    @Get("/widgets/{id}")
    @Summary("Get one widget")
    func get(string id): gebweb.Response {
        let found = this.repository.findById(id);
        if (found == null) {
            this.notFound("no widget " + id);
        }
        return this.json(found);
    }

    @Post("/widgets")
    @Summary("Create a widget")
    func create(model.Widget body): gebweb.Response {
        let row = {"id": body.id, "name": body.name};
        this.repository.save(row);
        return this.created(row, "/widgets/" + body.id);
    }
}
`
}

func templateContent(name string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>{{ title }} | ` + name + `</title>
  <link rel="stylesheet" href="{{ 'app.css' | asset }}">
</head>
<body>
  <h1>{{ title }}</h1>
  <ul>
  {% for w in widgets %}
    <li>{{ w.name }}</li>
  {% endfor %}
  </ul>
  <script src="{{ 'app.js' | asset }}"></script>
</body>
</html>
`
}

func mainContent(opts newOptions) string {
	controllerCtor := "ctrl.WidgetController(repository)"
	ctrlImport := "import widget_controller as ctrl;"
	if opts.typ == "app" {
		controllerCtor = "ctrl.HomeController(repository)"
		ctrlImport = "import home_controller as ctrl;"
	}

	var b strings.Builder
	b.WriteString("module main;\n\n")
	b.WriteString("import gebweb;\n")
	b.WriteString("import io;\n")
	b.WriteString("import sys;\n")
	b.WriteString("import dotenv;\n")
	b.WriteString("import db;\n")
	b.WriteString("import widget_repository as repo;\n")
	b.WriteString(ctrlImport + "\n\n")
	b.WriteString("func envOr(string key, string fallback): string {\n")
	b.WriteString("    let v = sys.getenv(key);\n")
	b.WriteString("    return v == null ? fallback : v as string;\n")
	b.WriteString("}\n\n")
	b.WriteString("/* Entry point. Run directly with `gebweb dev` or build with\n")
	b.WriteString(" * `gebweb build`; both invoke main. */\n")
	b.WriteString("export func main(list<string> args): int {\n")
	b.WriteString("    if (io.exists(\".env\")) { dotenv.loadAndApply(\".env\"); }\n")
	fmt.Fprintf(&b, "    let conn = db.connect(envOr(\"DB_DRIVER\", \"%s\"), envOr(\"DATABASE_URL\", \"%s\"));\n",
		dbDriver(opts.db), dbDSN(opts.db))
	b.WriteString("    let repository = repo.WidgetRepository(conn);\n")
	fmt.Fprintf(&b, "    let app = gebweb.setInfo(gebweb.app([%s]), {\n", controllerCtor)
	fmt.Fprintf(&b, "        \"title\": \"%s\",\n", opts.name)
	b.WriteString("        \"version\": \"0.1.0\",\n")
	b.WriteString("    });\n")
	if opts.typ == "app" {
		b.WriteString("    gebweb.useViews(app, \"templates\");\n")
		b.WriteString("    gebweb.useStaticAssets(app, \"build/assets\");\n")
	}
	fmt.Fprintf(&b, "    let addr = \"0.0.0.0:\" + envOr(\"GEBWEB_PORT\", \"%d\");\n", opts.port)
	fmt.Fprintf(&b, "    io.println(\"%s listening on http://\" + addr);\n", opts.name)
	b.WriteString("    gebweb.serve(app, addr);\n")
	b.WriteString("    return 0;\n")
	b.WriteString("}\n")
	return b.String()
}

func testContent(opts newOptions) string {
	controllerCtor := "ctrl.WidgetController(repository)"
	ctrlImport := "import widget_controller as ctrl;"
	path := "/widgets"
	if opts.typ == "app" {
		controllerCtor = "ctrl.HomeController(repository)"
		ctrlImport = "import home_controller as ctrl;"
		path = "/"
	}

	var b strings.Builder
	b.WriteString("import test;\n")
	b.WriteString("import gebweb;\n")
	b.WriteString("import db;\n")
	b.WriteString("import widget_repository as repo;\n")
	b.WriteString(ctrlImport + "\n\n")
	b.WriteString("class MainTest extends test.Test {\n")
	b.WriteString("    func buildApp(): gebweb.GebwebApp {\n")
	b.WriteString("        let conn = db.connect(\"sqlite\", \":memory:\");\n")
	b.WriteString("        let repository = repo.WidgetRepository(conn);\n")
	fmt.Fprintf(&b, "        let app = gebweb.app([%s]);\n", controllerCtor)
	if opts.typ == "app" {
		b.WriteString("        gebweb.useViews(app, \"templates\");\n")
		b.WriteString("        gebweb.useStaticAssets(app, \"build/assets\");\n")
	}
	b.WriteString("        return app;\n")
	b.WriteString("    }\n\n")
	b.WriteString("    @test\n")
	b.WriteString("    func indexReturns200(): void {\n")
	b.WriteString("        let client = gebweb.TestClient(this.buildApp());\n")
	fmt.Fprintf(&b, "        let r = client.get(\"%s\");\n", path)
	b.WriteString("        this.assertEquals(200, r.status);\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	return b.String()
}

func readmeContent(opts newOptions) string {
	var b strings.Builder
	kind := "server-rendered app"
	if opts.typ == "api" {
		kind = "JSON API"
	}
	fmt.Fprintf(&b, "# %s\n\nA Gebweb %s (database: %s).\n\n", opts.name, kind, opts.db)
	b.WriteString("## Develop\n\n    gebweb dev\n\n")
	b.WriteString("## Test\n\n    geblang test src/\n\n")
	b.WriteString("## Build\n\n    gebweb build            # single binary at build/app\n")
	if opts.docker {
		b.WriteString("    gebweb build --docker   # also (re)generate Docker files\n\n")
		b.WriteString("## Run with Docker\n\n    gebweb build --docker\n    docker compose up --build\n")
	} else {
		b.WriteString("\nGenerate Docker files later with `gebweb docker`.\n")
	}
	return b.String()
}
