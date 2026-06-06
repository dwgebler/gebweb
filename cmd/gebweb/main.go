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
	"strconv"
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
	case "docker":
		os.Exit(runDocker(args))
	case "routes":
		os.Exit(runRoutes(args))
	case "generate", "gen":
		os.Exit(runGenerate(args))
	case "migrate":
		os.Exit(runMigrate(args))
	case "worker":
		os.Exit(runWorker(args))
	case "secrets":
		os.Exit(runSecrets(args))
	case "version", "--version":
		fmt.Printf("gebweb %s\n", version)
		os.Exit(0)
	case "licenses":
		fmt.Fprint(os.Stdout, licenseText)
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
	case "docker":
		printDockerHelp(w)
	case "routes":
		printRoutesHelp(w)
	case "generate", "gen":
		printGenerateHelp(w)
	case "generate-client", "client":
		printGenerateClientHelp(w)
	case "migrate":
		printMigrateHelp(w)
	case "worker":
		printWorkerHelp(w)
	case "secrets":
		printSecretsHelp(w)
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
	fmt.Fprintln(w, "gebweb - the Gebweb framework CLI")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "usage: gebweb <command> [args...]")
	fmt.Fprintln(w, "       gebweb help [command]")
	fmt.Fprintln(w, "       gebweb <command> --help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scaffold a project:")
	fmt.Fprintln(w, "  new <name>                  Create a new Gebweb project at ./<name>/.")
	fmt.Fprintln(w, "  generate <kind> <Name>      Scaffold a controller, DTO, repository,")
	fmt.Fprintln(w, "                              resource bundle, or HTTP client (from an")
	fmt.Fprintln(w, "                              OpenAPI 3.x spec).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run during development:")
	fmt.Fprintln(w, "  dev                         Start the app with hot-reload on src/ changes.")
	fmt.Fprintln(w, "  routes                      List the registered routes (method + path +")
	fmt.Fprintln(w, "                              handler).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Operate against the database and secrets:")
	fmt.Fprintln(w, "  migrate <up|down|status|create>")
	fmt.Fprintln(w, "                              Apply / roll back / inspect SQL migrations.")
	fmt.Fprintln(w, "  secrets <init|edit|set|get|list>")
	fmt.Fprintln(w, "                              Manage the encrypted secrets vault that")
	io.WriteString(w, "                              `%secret(...)%` markers in config/services.yaml\n")
	fmt.Fprintln(w, "                              resolve through.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Ship to production:")
	fmt.Fprintln(w, "  build                       Produce a single-binary release via")
	fmt.Fprintln(w, "                              `geblang build` (assets + templates embedded).")
	fmt.Fprintln(w, "  docker                      Generate a Dockerfile and compose.yaml.")
	fmt.Fprintln(w, "  worker                      Run the background-job + messaging worker.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Meta:")
	fmt.Fprintln(w, "  help [command]              Show help for a command (this page when bare).")
	fmt.Fprintln(w, "  version                     Print the gebweb CLI version.")
	fmt.Fprintln(w, "  licenses                    Print third-party attribution notices.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run `gebweb help <command>` or `gebweb <command> --help` for details.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Typical workflow:")
	fmt.Fprintln(w, "  gebweb new myapp && cd myapp")
	fmt.Fprintln(w, "  gebweb dev")
	fmt.Fprintln(w, "  gebweb generate controller User")
	fmt.Fprintln(w, "  gebweb migrate create add_users")
	fmt.Fprintln(w, "  gebweb migrate up")
	fmt.Fprintln(w, "  gebweb build --out app && ./app")
}

func printNewHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb new [<name>] [--type app|api] [--db <driver>]")
	fmt.Fprintln(w, "                  [--docker] [--port <port>] [--yes]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scaffold a new Gebweb project under ./<name>/. Run interactively,")
	fmt.Fprintln(w, "the command prompts for any option not given as a flag; pass --yes")
	fmt.Fprintln(w, "(or pipe stdin) to take defaults non-interactively.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --type app|api   app: server-rendered (templates + assets pipeline);")
	fmt.Fprintln(w, "                   api: JSON-only. Default app.")
	fmt.Fprintln(w, "  --db <driver>    sqlite (default) | postgres | pgvector | mysql.")
	fmt.Fprintln(w, "  --docker         Also generate a Dockerfile and compose.yaml.")
	fmt.Fprintln(w, "  --port <port>    Port wired into .env / Docker. Default 8080.")
	fmt.Fprintln(w, "  --yes, -y        Accept defaults for unspecified options (no prompts).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Created files include geblang.yaml, .env, src/main.gb, a sample")
	fmt.Fprintln(w, "controller + model + repository, a TestClient suite, and (for app)")
	fmt.Fprintln(w, "a template and a CSS/TS asset, plus Docker files when --docker.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "After scaffolding:")
	fmt.Fprintln(w, "  cd <name>")
	fmt.Fprintln(w, "  gebweb dev            # hot-reloading dev server")
	fmt.Fprintln(w, "  geblang test src/     # run the starter test suite")
}

func printDevHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb dev [--entry <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run the entry script under a file watcher. The watcher walks")
	fmt.Fprintln(w, "src/ recursively (and any nested subpackages) and restarts the")
	fmt.Fprintln(w, "process on any .gb, .yaml, .html, or template-file change.")
	fmt.Fprintln(w, "Repeated changes within a short window are coalesced into a")
	fmt.Fprintln(w, "single restart.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "The entry script binds whatever address it serves on; the")
	fmt.Fprintln(w, "watcher itself does not impose a port. Press Ctrl+C to stop.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --entry <path>    Entry file. Default: src/main.gb.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Notes:")
	fmt.Fprintln(w, "  - Hidden directories (`.git`, `.geblang-cache`, etc.) are")
	fmt.Fprintln(w, "    skipped automatically.")
	fmt.Fprintln(w, "  - For production, prefer `gebweb build` over running `gebweb dev`")
	fmt.Fprintln(w, "    behind a process manager; the dev loop is optimised for fast")
	fmt.Fprintln(w, "    iteration, not durability.")
}

func printBuildHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb build [--entry <path>] [--out <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Produce a single-binary release of the project. Bundles the")
	fmt.Fprintln(w, "entry script and every reachable module (project source plus")
	fmt.Fprintln(w, "stdlib + dependency packages) into a self-contained executable")
	fmt.Fprintln(w, "that does not need geblang or any runtime files at deploy time.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Assets and templates are processed and embedded too. With an")
	fmt.Fprintln(w, "`assets:` block in geblang.yaml, entry points are bundled and")
	fmt.Fprintln(w, "minified (JS/TS/JSX via esbuild, SASS via dart-sass, CSS via")
	fmt.Fprintln(w, "esbuild); HTML templates are minified; and the compiled output,")
	fmt.Fprintln(w, "templates/, and public/ are embedded so the binary is")
	fmt.Fprintln(w, "self-contained. The app reads them via sys.bundleDir().")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --entry <path>    Entry file. Default: src/main.gb.")
	fmt.Fprintln(w, "  --out <path>      Output binary path. Default: build/app.")
	fmt.Fprintln(w, "  --no-minify       Skip minification (assets and templates).")
	fmt.Fprintln(w, "  --no-sass         Skip SASS compilation when dart-sass is absent.")
	fmt.Fprintln(w, "  --no-swagger      Skip embedding the SwaggerUI assets.")
	fmt.Fprintln(w, "  --docker          Also generate a Dockerfile and compose.yaml.")
	fmt.Fprintln(w, "  --db <driver>     DB service for --docker: sqlite (default) |")
	fmt.Fprintln(w, "                    postgres | pgvector | mysql.")
	fmt.Fprintln(w, "  --port <port>     Port for --docker. Default 8080.")
	fmt.Fprintln(w, "  --force           Overwrite existing Docker files (with --docker).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gebweb build                            # build/app")
	fmt.Fprintln(w, "  gebweb build --out ./dist/myapp")
	fmt.Fprintln(w, "  gebweb build --entry src/cmd/api.gb --out api")
	fmt.Fprintln(w, "  gebweb build --no-minify                # faster, readable output")
}

func printRoutesHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb routes [--entry <path>]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Print the project's registered routes (method, path, controller,")
	fmt.Fprintln(w, "handler) without starting the HTTP server.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "How it works: the command runs the entry script with the env var")
	fmt.Fprintln(w, "GEBWEB_PRINT_ROUTES=1 set. The scaffolded main.gb branches on the")
	fmt.Fprintln(w, "var and exits via `gebweb.printRoutesAndExit(app)`. Hand-rolled")
	fmt.Fprintln(w, "main.gb files can adopt the same convention:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    if (sys.getenv(\"GEBWEB_PRINT_ROUTES\") == \"1\") {")
	fmt.Fprintln(w, "        gebweb.printRoutesAndExit(app);")
	fmt.Fprintln(w, "    }")
	fmt.Fprintln(w, "    gebweb.serve(app, \"127.0.0.1:8080\");")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --entry <path>    Entry file. Default: src/main.gb.")
}

func printGenerateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb generate <kind> <Name>")
	fmt.Fprintln(w, "       gebweb generate client <spec-path> <Name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Scaffold a boilerplate file (or set of files) under src/. <Name>")
	fmt.Fprintln(w, "must be PascalCase; the output filename is derived from it.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Kinds:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  controller <Name>")
	fmt.Fprintln(w, "      Writes src/<name>_controller.gb: a class with @Get(\"/<name>\")")
	fmt.Fprintln(w, "      list and @Get(\"/<name>/{id}\") show handler stubs returning")
	fmt.Fprintln(w, "      dict<string, any>.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  dto <Name>")
	fmt.Fprintln(w, "      Writes src/<name>_dto.gb: a class with two example typed")
	fmt.Fprintln(w, "      fields (`string name`, `int age`) you replace.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  repository <Name>")
	fmt.Fprintln(w, "      Writes src/<name>_repository.gb: a class with findAll,")
	fmt.Fprintln(w, "      findById, and save method stubs returning placeholder data.")
	fmt.Fprintln(w, "      Pair with the matching DTO class.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  resource <Name>")
	fmt.Fprintln(w, "      Writes the full bundle: controller + DTO + repository plus")
	fmt.Fprintln(w, "      a test file that drives the routes through TestClient. The")
	fmt.Fprintln(w, "      controller is decorated with @ApiResource so the framework")
	fmt.Fprintln(w, "      auto-generates LIST / GET / POST / PUT / PATCH / DELETE")
	fmt.Fprintln(w, "      handlers from the repository.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  client <spec-path> <Name>")
	fmt.Fprintln(w, "      Generate a Geblang HTTP client from an OpenAPI 3.x spec.")
	fmt.Fprintln(w, "      Run `gebweb help generate-client` for the full reference.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gebweb generate controller User")
	fmt.Fprintln(w, "  gebweb generate dto User")
	fmt.Fprintln(w, "  gebweb generate resource Product")
	fmt.Fprintln(w, "  gebweb generate client ./openapi.yaml StripeClient")
}

func printGenerateClientHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb generate client <spec-path> <Name>")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Generate a Geblang HTTP client class from an OpenAPI 3.x spec.")
	fmt.Fprintln(w, "The output is plain Geblang you can read, edit, and re-generate.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Arguments:")
	fmt.Fprintln(w, "  <spec-path>   Path to the OpenAPI document. YAML or JSON accepted")
	fmt.Fprintln(w, "                (JSON is parsed via the YAML parser, which supports")
	fmt.Fprintln(w, "                JSON as a subset). Local file only; URLs not fetched.")
	fmt.Fprintln(w, "  <Name>        PascalCase prefix for the generated class. Letters")
	fmt.Fprintln(w, "                and digits only.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Output:")
	fmt.Fprintln(w, "  src/<name>_client.gb        (lowercase first letter of <Name>)")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "The command refuses to overwrite an existing file; remove it first")
	fmt.Fprintln(w, "or regenerate to a different <Name>.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "What gets generated:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  DTOs")
	fmt.Fprintln(w, "    One Geblang class per `components.schemas` entry. Properties")
	fmt.Fprintln(w, "    use Geblang typed fields (`string`, `int`, `bool`, `?T` for")
	fmt.Fprintln(w, "    nullable, `list<T>` for arrays, nested class refs for object")
	fmt.Fprintln(w, "    schemas). $ref / allOf / oneOf / anyOf are followed when")
	fmt.Fprintln(w, "    possible; unsupported shapes fall back to `any`.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Operations")
	fmt.Fprintln(w, "    One method per OpenAPI operation. Method name is the")
	fmt.Fprintln(w, "    `operationId` when set, otherwise built from the HTTP method")
	fmt.Fprintln(w, "    and path. Path / query / header / cookie parameters become")
	fmt.Fprintln(w, "    typed method arguments. Request bodies bind to the matching")
	fmt.Fprintln(w, "    DTO class. The return type is taken from the 2xx response")
	fmt.Fprintln(w, "    schema; non-2xx responses throw `errors.HttpException`.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Authentication")
	fmt.Fprintln(w, "    The constructor takes a config dict. Recognised keys are")
	fmt.Fprintln(w, "    derived from `components.securitySchemes`:")
	fmt.Fprintln(w, "      bearer:  {\"token\":   \"<jwt>\"}     -> Authorization: Bearer <jwt>")
	fmt.Fprintln(w, "      basic:   {\"basic\":   \"<b64>\"}     -> Authorization: Basic <b64>")
	fmt.Fprintln(w, "      apiKey:  {\"apiKey\":  \"<key>\"}     -> header / query / cookie")
	fmt.Fprintln(w, "                                            named by the spec")
	fmt.Fprintln(w, "    Multiple schemes are supported; the client picks the one(s)")
	fmt.Fprintln(w, "    listed under each operation's `security` clause (or the")
	fmt.Fprintln(w, "    global default).")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  Server URL")
	fmt.Fprintln(w, "    The first entry in `servers` becomes the default base URL.")
	fmt.Fprintln(w, "    Override per-instance via the config dict's \"baseUrl\" key.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gebweb generate client ./openapi.yaml StripeClient")
	fmt.Fprintln(w, "  gebweb generate client ./vendor/petstore.json PetStoreClient")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Using the generated client:")
	fmt.Fprintln(w, "  import stripeClient as sc;")
	fmt.Fprintln(w, "  let client = sc.StripeClient({\"baseUrl\": \"https://api.stripe.com\",")
	fmt.Fprintln(w, "                                  \"token\":   \"sk_live_...\"});")
	fmt.Fprintln(w, "  let customer = client.createCustomer(sc.CustomerDto(\"a@b.com\"));")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Notes:")
	fmt.Fprintln(w, "  - The generated file is marked with a `do not edit` banner. If")
	fmt.Fprintln(w, "    you need to customise behaviour, edit the spec or post-process")
	fmt.Fprintln(w, "    the output rather than hand-patching it.")
	fmt.Fprintln(w, "  - The generator never opens a network connection; pass a local")
	fmt.Fprintln(w, "    file path or pipe the spec to disk first.")
}

func printMigrateHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb migrate <subcommand> [args]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run versioned SQL migrations against the database identified by")
	fmt.Fprintln(w, "the DATABASE_URL environment variable. Supports sqlite, postgres,")
	fmt.Fprintln(w, "and mysql; the driver is selected from the URL scheme.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Migration files live under ./migrations/ and use a delimiter")
	fmt.Fprintln(w, "convention to separate the up and down SQL:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    -- +gebweb up")
	fmt.Fprintln(w, "    CREATE TABLE users (id TEXT PRIMARY KEY, email TEXT NOT NULL);")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    -- +gebweb down")
	fmt.Fprintln(w, "    DROP TABLE users;")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Applied versions are tracked in a `gebweb_schema_migrations` table")
	fmt.Fprintln(w, "the runner creates automatically.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintln(w, "  create <name>             Scaffold a new timestamped migration")
	fmt.Fprintln(w, "                            file under ./migrations/. The file name")
	fmt.Fprintln(w, "                            is `<timestamp>_<name>.sql`.")
	fmt.Fprintln(w, "  up [--target <version>]   Apply every pending migration in order.")
	fmt.Fprintln(w, "                            With --target, stops after the named")
	fmt.Fprintln(w, "                            version (inclusive).")
	fmt.Fprintln(w, "  down [--target <version>] Roll back the most recent migration. With")
	fmt.Fprintln(w, "                            --target, rolls back to (but not past)")
	fmt.Fprintln(w, "                            the named version.")
	fmt.Fprintln(w, "  status                    List applied versions, then pending ones.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Environment:")
	fmt.Fprintln(w, "  DATABASE_URL    Required. Examples:")
	fmt.Fprintln(w, "                    sqlite:./app.db")
	fmt.Fprintln(w, "                    sqlite::memory:")
	fmt.Fprintln(w, "                    postgres://user:pass@localhost/db")
	fmt.Fprintln(w, "                    mysql://user:pass@localhost/db")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gebweb migrate create add_users")
	fmt.Fprintln(w, "  gebweb migrate up")
	fmt.Fprintln(w, "  gebweb migrate up --target 20260101_120000")
	fmt.Fprintln(w, "  gebweb migrate down")
	fmt.Fprintln(w, "  gebweb migrate status")
}

func printWorkerHelp(w io.Writer) {
	fmt.Fprintln(w, "usage: gebweb worker [options]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run the project's background-job and messaging worker process.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "How it works: the command sets GEBWEB_RUN=worker and shells out")
	fmt.Fprintln(w, "to `geblang <entry>`. main.gb should branch on the env var and")
	fmt.Fprintln(w, "call `gebweb.runWorker(app)` and / or `gebweb.runMessageWorker(app)`")
	fmt.Fprintln(w, "instead of `gebweb.serve(...)`:")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    if (sys.getenv(\"GEBWEB_RUN\") == \"worker\") {")
	fmt.Fprintln(w, "        gebweb.runWorker(app);            # @Job handlers")
	fmt.Fprintln(w, "        gebweb.runMessageWorker(app);     # @OnMessage handlers")
	fmt.Fprintln(w, "    } else {")
	fmt.Fprintln(w, "        gebweb.serve(app, \"0.0.0.0:8080\");")
	fmt.Fprintln(w, "    }")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run multiple worker processes in parallel: job-row claims are")
	fmt.Fprintln(w, "atomic and broker consumers fan out at the broker level.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Filtering: the flags below let a single server drain a subset")
	fmt.Fprintln(w, "of work, so different machines can specialise (e.g. one server")
	fmt.Fprintln(w, "processes email-send jobs, another processes image-resize jobs).")
	fmt.Fprintln(w, "Flags translate into env vars (GEBWEB_WORKER_KIND,")
	fmt.Fprintln(w, "GEBWEB_WORKER_JOBS, GEBWEB_WORKER_HANDLES) that the runtime")
	fmt.Fprintln(w, "honours; main.gb doesn't have to thread anything through.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Options:")
	fmt.Fprintln(w, "  --entry <path>      Entry file. Default: src/main.gb.")
	fmt.Fprintln(w, "  --job <name>        Process only @Job(\"<name>\") handlers.")
	fmt.Fprintln(w, "                      Repeatable; jobs not in the list are skipped.")
	fmt.Fprintln(w, "  --handle <name>     Process only the @OnMessage(\"<name>\")")
	fmt.Fprintln(w, "                      handlers tied to this broker handle. Repeatable.")
	fmt.Fprintln(w, "  --jobs-only         Skip the messaging consumer entirely.")
	fmt.Fprintln(w, "                      Combine with --job to pin one server to a job pool.")
	fmt.Fprintln(w, "  --messaging-only    Skip the background-job loop entirely.")
	fmt.Fprintln(w, "                      Combine with --handle to pin one server to a")
	fmt.Fprintln(w, "                      broker handle.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  gebweb worker")
	fmt.Fprintln(w, "      Drain every job and every messaging handle.")
	fmt.Fprintln(w, "  gebweb worker --job email --job sms")
	fmt.Fprintln(w, "      Run only the `email` and `sms` background jobs.")
	fmt.Fprintln(w, "  gebweb worker --messaging-only --handle orders")
	fmt.Fprintln(w, "      Consume only the `orders` broker handle; skip jobs.")
	fmt.Fprintln(w, "  gebweb worker --jobs-only --job image-resize")
	fmt.Fprintln(w, "      Run only the `image-resize` job; skip messaging.")
}

// runNew scaffolds a new project. Layout:
//
//	<name>/
//	  geblang.yaml
//	  src/main.gb
//	  src/main_test.gb
//	  README.md
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

	// Compile assets once (unminified) so dev serves them from disk; the app
	// reads the source tree directly since sys.bundleDir() is empty in dev.
	if cfg, cfgErr := readAssetsConfig("geblang.yaml"); cfgErr != nil {
		fmt.Fprintf(os.Stderr, "gebweb dev: %v\n", cfgErr)
		return 1
	} else if cfg != nil {
		if err := compileEntryPoints(cfg, assetBuildOptions{minify: false}); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb dev: %v\n", err)
			return 1
		}
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
	opts := assetBuildOptions{minify: true}
	noSwagger := false
	withDocker := false
	dockerOpts := dockerOptions{db: "sqlite", port: 8080}
	for i, a := range args {
		if a == "--entry" && i+1 < len(args) {
			entry = args[i+1]
		}
		if a == "--out" && i+1 < len(args) {
			out = args[i+1]
		}
		if a == "--no-minify" {
			opts.minify = false
		}
		if a == "--no-sass" {
			opts.noSass = true
		}
		if a == "--no-swagger" {
			noSwagger = true
		}
		if a == "--docker" {
			withDocker = true
		}
		if a == "--db" && i+1 < len(args) {
			dockerOpts.db = args[i+1]
		}
		if a == "--port" && i+1 < len(args) {
			p, err := strconv.Atoi(args[i+1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "gebweb build: invalid --port %q\n", args[i+1])
				return 2
			}
			dockerOpts.port = p
		}
		if a == "--force" {
			dockerOpts.force = true
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}

	cfg, err := readAssetsConfig("geblang.yaml")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}
	resources, err := buildAssets(cfg, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}

	if !noSwagger {
		swaggerDir, err := vendorSwaggerUI(httpDownload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
			return 1
		}
		resources = append(resources, swaggerDir+"="+swaggerBundleDir)
	}

	buildArgs := []string{"build", "--entry", entryModuleName(entry), "--out", out}
	for _, r := range resources {
		buildArgs = append(buildArgs, "--resource", r)
	}
	buildArgs = append(buildArgs, ".")
	cmd := exec.Command("geblang", buildArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
		return 1
	}
	fmt.Printf("built %s\n", out)

	if withDocker {
		dockerOpts.binary = out
		if err := generateDocker(dockerOpts); err != nil {
			fmt.Fprintf(os.Stderr, "gebweb build: %v\n", err)
			return 1
		}
	}
	return 0
}

// entryModuleName converts an entry file path (e.g. "src/main.gb") to the
// canonical module name geblang build expects ("main"). A bare module name is
// returned unchanged.
func entryModuleName(entry string) string {
	e := filepath.ToSlash(entry)
	e = strings.TrimPrefix(e, "./")
	e = strings.TrimPrefix(e, "src/")
	e = strings.TrimSuffix(e, ".gb")
	return strings.ReplaceAll(e, "/", ".")
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
	// The `client` subcommand takes (spec, Name) rather than just Name.
	if len(args) >= 1 && args[0] == "client" {
		return runGenerateClient(args[1:])
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gebweb generate <controller|dto|repository|resource> <Name>")
		fmt.Fprintln(os.Stderr, "       gebweb generate client <spec.yaml|spec.json> <Name>")
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
