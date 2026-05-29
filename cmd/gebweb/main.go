// gebweb is the developer CLI for the Gebweb web framework. It
// scaffolds new projects (`gebweb new`), runs them with hot-reload
// (`gebweb dev`), builds release binaries (`gebweb build`), prints
// the routes table (`gebweb routes`), and emits boilerplate
// (`gebweb generate`).
//
// The CLI shells out to the host `geblang` binary for execution.
// File watching uses fsnotify directly so the dev loop doesn't pay
// a Geblang startup cost per change.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stdout)
		os.Exit(0)
	}
	cmd, args := os.Args[1], os.Args[2:]
	switch cmd {
	case "new":
		os.Exit(runNew(args))
	case "dev":
		os.Exit(runDev(args))
	case "build":
		os.Exit(runBuild(args))
	case "routes":
		os.Exit(runRoutes(args))
	case "generate", "gen":
		os.Exit(runGenerate(args))
	case "migrate":
		os.Exit(runMigrate(args))
	case "worker":
		os.Exit(runWorker(args))
	case "version", "--version":
		fmt.Printf("gebweb %s\n", version)
		os.Exit(0)
	case "help", "--help", "-h":
		os.Exit(runHelp(os.Stdout, args))
	default:
		fmt.Fprintf(os.Stderr, "gebweb: unknown command %q\n", cmd)
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

// hasHelpFlag reports whether the argument list contains --help or
// -h anywhere. Used by every subcommand to short-circuit before any
// other parsing or file probing happens.
func hasHelpFlag(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// runHelp dispatches `gebweb help <command>` to the matching
// subcommand's help printer. Bare `gebweb help` prints the top-
// level usage.
func runHelp(w io.Writer, args []string) int {
	if len(args) == 0 {
		printUsage(w)
		return 0
	}
	switch args[0] {
	case "new":
		printNewHelp(w)
	case "dev":
		printDevHelp(w)
	case "build":
		printBuildHelp(w)
	case "routes":
		printRoutesHelp(w)
	case "generate", "gen":
		printGenerateHelp(w)
	case "migrate":
		printMigrateHelp(w)
	case "worker":
		printWorkerHelp(w)
	case "version":
		fmt.Fprintln(w, "usage: gebweb version")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Print the gebweb CLI version and exit.")
	default:
		fmt.Fprintf(os.Stderr, "gebweb help: unknown command %q\n", args[0])
		printUsage(os.Stderr)
		return 2
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb <command> [args...]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "commands:")
	fmt.Fprintln(w, "  new <name>                scaffold a new Gebweb project in ./<name>/")
	fmt.Fprintln(w, "  dev                       run the project with hot-reload")
	fmt.Fprintln(w, "  build                     produce a release binary via geblang build")
	fmt.Fprintln(w, "  routes                    list the project's routes")
	fmt.Fprintln(w, "  generate <kind> <name>    scaffold a controller / dto / repository / resource")
	fmt.Fprintln(w, "  migrate <create|up|down|status>")
	fmt.Fprintln(w, "                            run schema migrations against $DATABASE_URL")
	fmt.Fprintln(w, "  worker                    run the background-job + messaging worker")
	fmt.Fprintln(w, "  version                   print the gebweb CLI version")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Pass --help (or -h) to any subcommand for its options.")
}

func printNewHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb new <name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scaffold a new Gebweb project under ./<name>/ with geblang.yaml,")
	fmt.Fprintln(w, "src/main.gb, src/main_test.gb, and a starter README.")
}

func printDevHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb dev [--entry <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run the project with hot-reload. Watches src/ recursively and")
	fmt.Fprintln(w, "restarts on any .gb or .yaml change.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  --entry <path>    entry file (default: src/main.gb)")
}

func printBuildHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb build [--entry <path>] [--out <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Produce a release binary via `geblang build`.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  --entry <path>    entry file (default: src/main.gb)")
	fmt.Fprintln(w, "  --out <path>      output binary path (default: build/app)")
}

func printRoutesHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb routes [--entry <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Print the project's route table. Runs the entry script with")
	fmt.Fprintln(w, "GEBWEB_PRINT_ROUTES=1; the scaffolded main.gb branches on the")
	fmt.Fprintln(w, "env var and calls `gebweb.printRoutesAndExit(app)`. Hand-rolled")
	fmt.Fprintln(w, "apps can adopt the same convention.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  --entry <path>    entry file (default: src/main.gb)")
}

func printGenerateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb generate <kind> <Name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scaffold a boilerplate file in src/.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "kinds:")
	fmt.Fprintln(w, "  controller    a class with @Get / @Get(\"/{id}\") handler stubs")
	fmt.Fprintln(w, "  dto           a data class with two example fields")
	fmt.Fprintln(w, "  repository    a class with findAll / findById / save")
	fmt.Fprintln(w, "  resource      controller + DTO + repository + test (full bundle)")
}

func printMigrateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb migrate <create|up|down|status> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run schema migrations against $DATABASE_URL. Migration files")
	fmt.Fprintln(w, "live in ./migrations/ and use the `-- +gebweb up` /")
	fmt.Fprintln(w, "`-- +gebweb down` delimiter convention.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "subcommands:")
	fmt.Fprintln(w, "  create <name>            scaffold a new timestamped migration file")
	fmt.Fprintln(w, "  up [--target <version>]  apply pending migrations (optionally up to <version>)")
	fmt.Fprintln(w, "  down [--target <version>] roll back migrations (optionally down to <version>)")
	fmt.Fprintln(w, "  status                   list applied + pending migrations")
}

func printWorkerHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb worker [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run the project's background-job + messaging worker. Sets")
	fmt.Fprintln(w, "GEBWEB_RUN=worker on the child process and shells out to")
	fmt.Fprintln(w, "`geblang <entry>`; the user's main.gb is expected to branch on")
	fmt.Fprintln(w, "the env var and call gebweb.runWorker(app) /")
	fmt.Fprintln(w, "gebweb.runMessageWorker(app).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Filtering flags let one server drain a subset of work so different")
	fmt.Fprintln(w, "machines can specialise (e.g. one server processes email-send jobs,")
	fmt.Fprintln(w, "another processes image-resize jobs). Flags translate into")
	fmt.Fprintln(w, "GEBWEB_WORKER_KIND / GEBWEB_WORKER_JOBS / GEBWEB_WORKER_HANDLES env")
	fmt.Fprintln(w, "vars that gebweb.runWorker / gebweb.runMessageWorker honour.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "options:")
	fmt.Fprintln(w, "  --entry <path>      entry file (default: src/main.gb)")
	fmt.Fprintln(w, "  --job <name>        process only jobs with this name (repeatable)")
	fmt.Fprintln(w, "  --handle <name>     process only this messaging handle (repeatable)")
	fmt.Fprintln(w, "  --jobs-only         skip the messaging loop")
	fmt.Fprintln(w, "  --messaging-only    skip the background-job loop")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "examples:")
	fmt.Fprintln(w, "  gebweb worker                                # drain everything")
	fmt.Fprintln(w, "  gebweb worker --job email --job sms          # only the email + sms jobs")
	fmt.Fprintln(w, "  gebweb worker --handle orders --jobs-only    # SKIPPED: --jobs-only wins; --handle ignored")
	fmt.Fprintln(w, "  gebweb worker --messaging-only --handle orders")
	fmt.Fprintln(w, "                                                # only consume the orders messaging handle")
}

// runNew scaffolds a new project. Layout:
//
//	<name>/
//	  geblang.yaml
//	  src/main.gb
//	  src/main_test.gb
//	  README.md
func runNew(args []string) int {
	if hasHelpFlag(args) {
		printNewHelp(os.Stdout)
		return 0
	}
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: gebweb new <name>")
		return 2
	}
	name := args[0]
	if err := os.MkdirAll(filepath.Join(name, "src"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb new: %v\n", err)
		return 1
	}
	files := map[string]string{
		"geblang.yaml":     fmt.Sprintf("name: %s\nversion: 0.1.0\nsource: src\n", name),
		"src/main.gb":      scaffoldMain(name),
		"src/main_test.gb": scaffoldTest(),
		"README.md":        fmt.Sprintf("# %s\n\nA Gebweb application.\n\nRun the dev server:\n\n    gebweb dev\n", name),
	}
	for rel, content := range files {
		path := filepath.Join(name, rel)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb new: write %s: %v\n", path, err)
			return 1
		}
	}
	fmt.Printf("scaffolded %s/\n", name)
	fmt.Printf("\n  cd %s && gebweb dev\n\n", name)
	return 0
}

func scaffoldMain(name string) string {
	return `import gebweb;
import io;

class HelloController {
    @Get("/")
    @Summary("Health check")
    func health(): dict<string, any> {
        return {"status": "ok", "service": "` + name + `"};
    }

    @Get("/hello/{who}")
    @Summary("Greet someone")
    func hello(string who): dict<string, any> {
        return {"message": "hello, " + who + "!"};
    }
}

let app = gebweb.setInfo(gebweb.app([HelloController()]), {
    "title": "` + name + `",
    "version": "0.1.0",
});

io.println("` + name + ` listening on http://127.0.0.1:8080");
gebweb.serve(app, "127.0.0.1:8080");
`
}

func scaffoldTest() string {
	return `import test;
import gebweb;
import gebweb.testclient as tc;

class HelloControllerTest extends test.Test {
    @test
    func healthReturnsOk(): void {
        let client = tc.newClient();
        let r = client.get("/");
        this.assertEquals(200, r["status"]);
    }
}
`
}

// runDev watches src/ (recursively) and restarts the app on
// every change. Coalesces rapid bursts (e.g. editor saves) via a
// 200 ms debounce.
func runDev(args []string) int {
	if hasHelpFlag(args) {
		printDevHelp(os.Stdout)
		return 0
	}
	if !fileExists("geblang.yaml") {
		fmt.Fprintln(os.Stderr, "gebweb dev: no geblang.yaml in current directory")
		return 1
	}
	entry := "src/main.gb"
	for i, a := range args {
		if a == "--entry" && i+1 < len(args) {
			entry = args[i+1]
		}
	}
	if !fileExists(entry) {
		fmt.Fprintf(os.Stderr, "gebweb dev: entry file %q not found\n", entry)
		return 1
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb dev: %v\n", err)
		return 1
	}
	defer watcher.Close()
	if err := filepath.Walk("src", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return watcher.Add(path)
		}
		return nil
	}); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb dev: %v\n", err)
		return 1
	}
	fmt.Printf("gebweb dev: starting %s (watching src/)\n", entry)
	restart := make(chan struct{}, 1)
	go func() {
		var debounce *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if !strings.HasSuffix(event.Name, ".gb") && !strings.HasSuffix(event.Name, ".yaml") {
					continue
				}
				if debounce != nil {
					debounce.Stop()
				}
				debounce = time.AfterFunc(200*time.Millisecond, func() {
					select {
					case restart <- struct{}{}:
					default:
					}
				})
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "gebweb dev: watcher error: %v\n", err)
			}
		}
	}()
	for {
		cmd := exec.Command("geblang", entry)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb dev: start: %v\n", err)
			return 1
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-restart:
			fmt.Fprintln(os.Stderr, "\ngebweb dev: change detected, restarting")
			if cmd.Process != nil {
				_ = cmd.Process.Signal(syscall.SIGTERM)
				select {
				case <-done:
				case <-time.After(2 * time.Second):
					_ = cmd.Process.Kill()
					<-done
				}
			}
		case err := <-done:
			if err != nil && !errors.Is(err, &exec.ExitError{}) {
				if _, ok := err.(*exec.ExitError); !ok {
					fmt.Fprintf(os.Stderr, "gebweb dev: child error: %v\n", err)
				}
			}
			fmt.Fprintln(os.Stderr, "gebweb dev: child exited; waiting for a file change to restart")
			<-restart
			fmt.Fprintln(os.Stderr, "gebweb dev: restarting")
		}
	}
}

// runBuild shells out to `geblang build` for the actual bundling.
func runBuild(args []string) int {
	if hasHelpFlag(args) {
		printBuildHelp(os.Stdout)
		return 0
	}
	if !fileExists("geblang.yaml") {
		fmt.Fprintln(os.Stderr, "gebweb build: no geblang.yaml in current directory")
		return 1
	}
	entry := "src/main.gb"
	out := "build/app"
	for i, a := range args {
		if a == "--entry" && i+1 < len(args) {
			entry = args[i+1]
		}
		if a == "--out" && i+1 < len(args) {
			out = args[i+1]
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}
	cmd := exec.Command("geblang", "build", "--entry", entry, "--out", out, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}
	fmt.Printf("built %s\n", out)
	return 0
}

// runRoutes runs the app with GEBWEB_PRINT_ROUTES=1 so the user's
// startup code prints the route table and exits. The scaffolded
// main.gb does this when the env var is set; user apps can adopt
// the same convention with the `gebweb.printRoutesAndExit(app)`
// helper.
func runRoutes(args []string) int {
	if hasHelpFlag(args) {
		printRoutesHelp(os.Stdout)
		return 0
	}
	if !fileExists("geblang.yaml") {
		fmt.Fprintln(os.Stderr, "gebweb routes: no geblang.yaml in current directory")
		return 1
	}
	entry := "src/main.gb"
	for i, a := range args {
		if a == "--entry" && i+1 < len(args) {
			entry = args[i+1]
		}
	}
	cmd := exec.Command("geblang", entry)
	cmd.Env = append(os.Environ(), "GEBWEB_PRINT_ROUTES=1")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb routes: %v\n", err)
		return 1
	}
	return 0
}

// runWorker executes the project entry script with GEBWEB_RUN=worker
// in the environment. The user's main.gb is expected to branch on
// that env var and call gebweb.runWorker(app) /
// gebweb.runMessageWorker(app).
//
// Filtering flags translate into env vars the Geblang-side facade
// consumes when building runWorker / runMessageWorker options:
//
//   - --job <name>     -> appended to GEBWEB_WORKER_JOBS (comma list)
//   - --handle <name>  -> appended to GEBWEB_WORKER_HANDLES (comma list)
//   - --jobs-only      -> GEBWEB_WORKER_KIND=jobs
//   - --messaging-only -> GEBWEB_WORKER_KIND=messaging
//
// Different worker processes can specialise on different work pools
// by combining these flags, e.g. one server runs `gebweb worker
// --jobs-only --job email --job sms` and another runs `gebweb worker
// --messaging-only --handle orders`.
func runWorker(args []string) int {
	if hasHelpFlag(args) {
		printWorkerHelp(os.Stdout)
		return 0
	}
	if !fileExists("geblang.yaml") {
		fmt.Fprintln(os.Stderr, "gebweb worker: no geblang.yaml in current directory")
		return 1
	}
	entry := "src/main.gb"
	var jobs, handles []string
	kind := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--entry":
			if i+1 < len(args) {
				entry = args[i+1]
				i++
			}
		case "--job":
			if i+1 < len(args) {
				jobs = append(jobs, args[i+1])
				i++
			}
		case "--handle":
			if i+1 < len(args) {
				handles = append(handles, args[i+1])
				i++
			}
		case "--jobs-only":
			kind = "jobs"
		case "--messaging-only":
			kind = "messaging"
		default:
			fmt.Fprintf(os.Stderr, "gebweb worker: unknown flag %q\n", a)
			printWorkerHelp(os.Stderr)
			return 2
		}
	}
	env := append(os.Environ(), "GEBWEB_RUN=worker")
	if kind != "" {
		env = append(env, "GEBWEB_WORKER_KIND="+kind)
	}
	if len(jobs) > 0 {
		env = append(env, "GEBWEB_WORKER_JOBS="+strings.Join(jobs, ","))
	}
	if len(handles) > 0 {
		env = append(env, "GEBWEB_WORKER_HANDLES="+strings.Join(handles, ","))
	}
	cmd := exec.Command("geblang", entry)
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb worker: %v\n", err)
		return 1
	}
	return 0
}

// runGenerate emits boilerplate files. Supported kinds:
//
//	controller <Name>
//	dto <Name>
//	repository <Name>
//	resource <Name>      // emits controller + dto + repository + test
func runGenerate(args []string) int {
	if hasHelpFlag(args) {
		printGenerateHelp(os.Stdout)
		return 0
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gebweb generate <controller|dto|repository|resource> <Name>")
		return 2
	}
	kind, name := args[0], args[1]
	if kind == "resource" {
		return runGenerateResource(name)
	}
	var content, path string
	switch kind {
	case "controller":
		path = filepath.Join("src", lcfirst(name)+"_controller.gb")
		content = scaffoldController(name)
	case "dto":
		path = filepath.Join("src", lcfirst(name)+"_dto.gb")
		content = scaffoldDTO(name)
	case "repository":
		path = filepath.Join("src", lcfirst(name)+"_repository.gb")
		content = scaffoldRepository(name)
	default:
		fmt.Fprintf(os.Stderr, "gebweb generate: unknown kind %q (controller, dto, repository, resource)\n", kind)
		return 2
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Fprintf(os.Stderr, "gebweb generate: %s already exists\n", path)
		return 1
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s\n", path)
	return 0
}

func scaffoldController(name string) string {
	plural := strings.ToLower(name) + "s"
	return fmt.Sprintf(`import gebweb;

class %sController {
    @Get("/%s")
    func list(): list<any> {
        return [];
    }

    @Get("/%s/{id}")
    func get(int id): dict<string, any> {
        return {"id": id};
    }
}
`, name, plural, plural)
}

func scaffoldDTO(name string) string {
	return fmt.Sprintf(`class %sDTO {
    string name;
    ?string description;

    func %sDTO(string name) {
        this.name = name;
        this.description = null;
    }
}
`, name, name)
}

func scaffoldRepository(name string) string {
	return fmt.Sprintf(`import gebweb;

class %sRepository {
    dict<string, dict<string, any>> store;

    func %sRepository() {
        this.store = {};
    }

    func findAll(): list<any> {
        list<any> out = [];
        for (key in this.store.keys()) {
            out = out.push(this.store[key]);
        }
        return out;
    }

    func findById(string id): ?dict<string, any> {
        if (!this.store.contains(id)) {
            return null;
        }
        return this.store[id];
    }

    func save(string id, dict<string, any> value): void {
        this.store[id] = value;
    }
}
`, name, name)
}

// runGenerateResource emits a controller + DTO + repository + test
// scaffold in one go. Pairs nicely with @ApiResource auto-CRUD.
func runGenerateResource(name string) int {
	lc := lcfirst(name)
	files := []struct {
		path    string
		content string
	}{
		{filepath.Join("src", lc+"_dto.gb"), scaffoldDTO(name)},
		{filepath.Join("src", lc+"_repository.gb"), scaffoldRepository(name)},
		{filepath.Join("src", lc+"_controller.gb"), scaffoldResourceController(name)},
		{filepath.Join("tests", lc+"_resource_test.gb"), scaffoldResourceTest(name)},
	}
	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			fmt.Fprintf(os.Stderr, "gebweb generate resource: %s already exists\n", f.path)
			return 1
		}
	}
	if err := os.MkdirAll("tests", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate resource: %v\n", err)
		return 1
	}
	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb generate resource: %v\n", err)
			return 1
		}
		fmt.Printf("wrote %s\n", f.path)
	}
	return 0
}

func scaffoldResourceController(name string) string {
	plural := strings.ToLower(name) + "s"
	return fmt.Sprintf(`import gebweb;

@ApiResource("/%s")
class %sController {
    static func repository(): %sRepository {
        return %sRepository();
    }
}
`, plural, name, name, name)
}

func scaffoldResourceTest(name string) string {
	plural := strings.ToLower(name) + "s"
	return fmt.Sprintf(`import test;
import gebweb;

class %sResourceTest extends test.Test {
    @test
    func listReturnsEmptyInitially(): void {
        let app = gebweb.app([%sController()]);
        let client = gebweb.TestClient(app);
        let r = client.get("/%s");
        this.assertEquals(200, r.status);
    }
}
`, name, name, plural)
}

func lcfirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(string(s[0])) + s[1:]
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
