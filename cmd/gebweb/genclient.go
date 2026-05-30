package main

// API client generator: turns an OpenAPI 3.x spec into a Geblang
// class wrapping the stdlib http module. One method per operation,
// data classes for component schemas, auth via a config dict passed
// to the constructor.
//
// The spec is loaded with the YAML parser (which accepts JSON since
// JSON is a YAML subset). All emission is plain string assembly into
// a strings.Builder; the generator never imports the Geblang
// runtime so this stays a fast build-time tool.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

/* -- Spec model ----------------------------------------------------- */

type apiSpec struct {
	OpenAPI    string                `yaml:"openapi"`
	Info       apiInfo               `yaml:"info"`
	Servers    []apiServer           `yaml:"servers"`
	Paths      map[string]*pathItem  `yaml:"paths"`
	Components apiComponents         `yaml:"components"`
	Security   []map[string][]string `yaml:"security"`
}

type apiInfo struct {
	Title       string `yaml:"title"`
	Version     string `yaml:"version"`
	Description string `yaml:"description"`
}

type apiServer struct {
	URL         string `yaml:"url"`
	Description string `yaml:"description"`
}

type apiComponents struct {
	Schemas         map[string]*apiSchema         `yaml:"schemas"`
	SecuritySchemes map[string]*apiSecurityScheme `yaml:"securitySchemes"`
}

type apiSecurityScheme struct {
	Type         string `yaml:"type"`
	Scheme       string `yaml:"scheme"` // bearer / basic (for type=http)
	BearerFormat string `yaml:"bearerFormat"`
	In           string `yaml:"in"`   // header / query / cookie (for type=apiKey)
	Name         string `yaml:"name"` // header/cookie/query parameter name
	Description  string `yaml:"description"`
}

type pathItem struct {
	Get        *apiOperation `yaml:"get"`
	Post       *apiOperation `yaml:"post"`
	Put        *apiOperation `yaml:"put"`
	Patch      *apiOperation `yaml:"patch"`
	Delete     *apiOperation `yaml:"delete"`
	Head       *apiOperation `yaml:"head"`
	Options    *apiOperation `yaml:"options"`
	Parameters []*apiParam   `yaml:"parameters"`
}

type apiOperation struct {
	OperationID string                `yaml:"operationId"`
	Summary     string                `yaml:"summary"`
	Description string                `yaml:"description"`
	Parameters  []*apiParam           `yaml:"parameters"`
	RequestBody *apiRequestBody       `yaml:"requestBody"`
	Responses   map[string]*apiResp   `yaml:"responses"`
	Security    []map[string][]string `yaml:"security"`
	Tags        []string              `yaml:"tags"`
	Deprecated  bool                  `yaml:"deprecated"`
}

type apiParam struct {
	Name        string     `yaml:"name"`
	In          string     `yaml:"in"` // path / query / header / cookie
	Required    bool       `yaml:"required"`
	Schema      *apiSchema `yaml:"schema"`
	Description string     `yaml:"description"`
	Ref         string     `yaml:"$ref"`
}

type apiRequestBody struct {
	Description string                  `yaml:"description"`
	Required    bool                    `yaml:"required"`
	Content     map[string]*apiMediaSet `yaml:"content"`
	Ref         string                  `yaml:"$ref"`
}

type apiResp struct {
	Description string                  `yaml:"description"`
	Content     map[string]*apiMediaSet `yaml:"content"`
	Ref         string                  `yaml:"$ref"`
}

type apiMediaSet struct {
	Schema *apiSchema `yaml:"schema"`
}

type apiSchema struct {
	Ref                  string                `yaml:"$ref"`
	Type                 string                `yaml:"type"`
	Format               string                `yaml:"format"`
	Properties           map[string]*apiSchema `yaml:"properties"`
	Required             []string              `yaml:"required"`
	Items                *apiSchema            `yaml:"items"`
	Enum                 []interface{}         `yaml:"enum"`
	Nullable             bool                  `yaml:"nullable"`
	AdditionalProperties interface{}           `yaml:"additionalProperties"`
	OneOf                []*apiSchema          `yaml:"oneOf"`
	AnyOf                []*apiSchema          `yaml:"anyOf"`
	AllOf                []*apiSchema          `yaml:"allOf"`
	Description          string                `yaml:"description"`
}

func loadSpec(path string) (*apiSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s apiSpec
	if err := yaml.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &s, nil
}

/* -- Generator entry point ----------------------------------------- */

func runGenerateClient(args []string) int {
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gebweb generate client <spec.yaml|spec.json> <Name>")
		return 2
	}
	specPath, name := args[0], args[1]
	if !validClientName(name) {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %q is not a valid class-name prefix (PascalCase, letters and digits only)\n", name)
		return 2
	}
	spec, err := loadSpec(specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %v\n", err)
		return 1
	}
	out, err := emitClient(spec, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %v\n", err)
		return 1
	}
	outPath := filepath.Join("src", lcfirst(name)+"_client.gb")
	if _, err := os.Stat(outPath); err == nil {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %s already exists\n", outPath)
		return 1
	}
	if err := os.MkdirAll("src", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outPath, []byte(out), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gebweb generate client: %v\n", err)
		return 1
	}
	fmt.Printf("wrote %s\n", outPath)
	return 0
}

func validClientName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsUpper(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

/* -- Emitter -------------------------------------------------------- */

type emitter struct {
	spec       *apiSpec
	name       string
	baseURL    string
	authNeeds  map[string]bool // bearer / basic / apiKey-header / apiKey-query / apiKey-cookie
	apiKeyName string          // first encountered apiKey header/query/cookie name
	apiKeyIn   string
	dtoNames   map[string]bool // tracks generated DTO class names so we can reference + dedupe
	out        *strings.Builder
}

func emitClient(spec *apiSpec, name string) (string, error) {
	e := &emitter{
		spec:      spec,
		name:      name,
		authNeeds: map[string]bool{},
		dtoNames:  map[string]bool{},
		out:       &strings.Builder{},
	}
	if len(spec.Servers) > 0 {
		e.baseURL = spec.Servers[0].URL
	}
	e.analyseAuth()
	e.writeHeader()
	e.writeImports()
	e.writeTrimHelper()
	e.writeDTOs()
	e.writeClientClass()
	return e.out.String(), nil
}

func (e *emitter) writeHeader() {
	fmt.Fprintf(e.out, "/**\n")
	fmt.Fprintf(e.out, " * Generated by `gebweb generate client`. Do not edit by hand;\n")
	fmt.Fprintf(e.out, " * regenerate from the source OpenAPI spec instead.\n")
	if e.spec.Info.Title != "" || e.spec.Info.Version != "" {
		fmt.Fprintf(e.out, " *\n")
		if e.spec.Info.Title != "" {
			fmt.Fprintf(e.out, " * Source: %s\n", e.spec.Info.Title)
		}
		if e.spec.Info.Version != "" {
			fmt.Fprintf(e.out, " * Version: %s\n", e.spec.Info.Version)
		}
	}
	fmt.Fprintf(e.out, " */\n\n")
}

func (e *emitter) writeImports() {
	fmt.Fprintf(e.out, "import http;\n")
	fmt.Fprintf(e.out, "import json;\n")
	if e.authNeeds["basic"] {
		fmt.Fprintf(e.out, "import encoding;\n")
	}
	fmt.Fprintf(e.out, "\n")
}

/* -- Auth analysis -------------------------------------------------- */

func (e *emitter) analyseAuth() {
	schemes := e.spec.Components.SecuritySchemes
	if schemes == nil {
		return
	}
	// Stable order so the generated constructor reads deterministically.
	names := make([]string, 0, len(schemes))
	for n := range schemes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sch := schemes[n]
		switch sch.Type {
		case "http":
			switch strings.ToLower(sch.Scheme) {
			case "bearer":
				e.authNeeds["bearer"] = true
			case "basic":
				e.authNeeds["basic"] = true
			}
		case "apiKey":
			switch strings.ToLower(sch.In) {
			case "header":
				e.authNeeds["apiKey-header"] = true
				if e.apiKeyName == "" {
					e.apiKeyName = sch.Name
					e.apiKeyIn = "header"
				}
			case "query":
				e.authNeeds["apiKey-query"] = true
				if e.apiKeyName == "" {
					e.apiKeyName = sch.Name
					e.apiKeyIn = "query"
				}
			case "cookie":
				e.authNeeds["apiKey-cookie"] = true
				if e.apiKeyName == "" {
					e.apiKeyName = sch.Name
					e.apiKeyIn = "cookie"
				}
			}
		case "oauth2", "openIdConnect":
			// Treat as a bearer token holder.
			e.authNeeds["bearer"] = true
		}
	}
}

/* -- DTO emission --------------------------------------------------- */

func (e *emitter) writeDTOs() {
	if e.spec.Components.Schemas == nil {
		return
	}
	names := make([]string, 0, len(e.spec.Components.Schemas))
	for n := range e.spec.Components.Schemas {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		sch := e.spec.Components.Schemas[n]
		if sch == nil {
			continue
		}
		className := pascal(n)
		// Only emit a class for object-shaped schemas; everything else
		// flows through as primitives / containers at the use site.
		if !isObjectSchema(sch) {
			continue
		}
		e.dtoNames[className] = true
		e.writeDTO(className, sch)
	}
}

func (e *emitter) writeDTO(className string, sch *apiSchema) {
	if sch.Description != "" {
		fmt.Fprintf(e.out, "/** %s */\n", oneLine(sch.Description))
	}
	fmt.Fprintf(e.out, "export class %s {\n", className)
	required := map[string]bool{}
	for _, r := range sch.Required {
		required[r] = true
	}
	// Property declaration order: alphabetical (stable).
	props := make([]string, 0, len(sch.Properties))
	for p := range sch.Properties {
		props = append(props, p)
	}
	sort.Strings(props)
	for _, p := range props {
		field := sch.Properties[p]
		t := e.geblangType(field, !required[p])
		fmt.Fprintf(e.out, "    %s %s;\n", t, fieldName(p))
	}
	fmt.Fprintf(e.out, "}\n\n")
}

func isObjectSchema(s *apiSchema) bool {
	if s == nil {
		return false
	}
	if s.Type == "object" {
		return true
	}
	if s.Type == "" && len(s.Properties) > 0 {
		return true
	}
	return false
}

/* -- Type mapping --------------------------------------------------- */

func (e *emitter) geblangType(s *apiSchema, optional bool) string {
	base := e.geblangBase(s)
	if optional || (s != nil && s.Nullable) {
		if !strings.HasPrefix(base, "?") {
			base = "?" + base
		}
	}
	return base
}

func (e *emitter) geblangBase(s *apiSchema) string {
	if s == nil {
		return "any"
	}
	if s.Ref != "" {
		return pascal(refName(s.Ref))
	}
	if len(s.OneOf) > 0 || len(s.AnyOf) > 0 {
		return "any"
	}
	if len(s.AllOf) > 0 {
		// Best-effort: if it's a single $ref allOf, use the ref; otherwise
		// fall back to `any`. Real spec merging is a bigger feature.
		if len(s.AllOf) == 1 && s.AllOf[0].Ref != "" {
			return pascal(refName(s.AllOf[0].Ref))
		}
		return "any"
	}
	switch s.Type {
	case "string":
		return "string"
	case "integer":
		return "int"
	case "number":
		// `format: float` -> float; default to decimal so precision survives.
		if strings.EqualFold(s.Format, "float") || strings.EqualFold(s.Format, "double") {
			return "float"
		}
		return "decimal"
	case "boolean":
		return "bool"
	case "array":
		inner := e.geblangBase(s.Items)
		return "list<" + inner + ">"
	case "object", "":
		return "dict<string, any>"
	}
	return "any"
}

func refName(ref string) string {
	// `#/components/schemas/User` -> `User`
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

/* -- Client class emission ------------------------------------------ */

func (e *emitter) writeTrimHelper() {
	fmt.Fprintf(e.out, "/* Strip a single trailing slash from a base URL so methods don't\n")
	fmt.Fprintf(e.out, " * accidentally emit double-slash URLs when their path is `/foo`. */\n")
	fmt.Fprintf(e.out, "func trimTrailingSlash(string s): string {\n")
	fmt.Fprintf(e.out, "    if (s.length() > 0 && s.substring(s.length() - 1, s.length()) == \"/\") {\n")
	fmt.Fprintf(e.out, "        return s.substring(0, s.length() - 1);\n")
	fmt.Fprintf(e.out, "    }\n")
	fmt.Fprintf(e.out, "    return s;\n")
	fmt.Fprintf(e.out, "}\n\n")
}

func (e *emitter) writeClientClass() {
	fmt.Fprintf(e.out, "/**\n")
	fmt.Fprintf(e.out, " * HTTP client for %s.\n", e.spec.Info.Title)
	if e.baseURL != "" {
		fmt.Fprintf(e.out, " * Default base URL: %s\n", e.baseURL)
	}
	fmt.Fprintf(e.out, " *\n")
	fmt.Fprintf(e.out, " * Construct with the base URL plus an optional auth config dict.\n")
	e.writeAuthDocs()
	fmt.Fprintf(e.out, " */\n")
	fmt.Fprintf(e.out, "export class %sClient {\n", e.name)
	fmt.Fprintf(e.out, "    string baseUrl;\n")
	if e.authNeeds["bearer"] {
		fmt.Fprintf(e.out, "    ?string bearerToken;\n")
	}
	if e.authNeeds["basic"] {
		fmt.Fprintf(e.out, "    ?string basicUser;\n")
		fmt.Fprintf(e.out, "    ?string basicPassword;\n")
	}
	if e.authNeeds["apiKey-header"] || e.authNeeds["apiKey-query"] || e.authNeeds["apiKey-cookie"] {
		fmt.Fprintf(e.out, "    ?string apiKey;\n")
	}
	fmt.Fprintf(e.out, "\n")
	e.writeConstructor()
	e.writeHeaderHelper()
	e.writeMethods()
	fmt.Fprintf(e.out, "}\n")
}

func (e *emitter) writeAuthDocs() {
	if len(e.authNeeds) == 0 {
		fmt.Fprintf(e.out, " * No authentication is required by this API.\n")
		return
	}
	if e.authNeeds["bearer"] {
		fmt.Fprintf(e.out, " *   bearerToken: string -> Authorization: Bearer <token>\n")
	}
	if e.authNeeds["basic"] {
		fmt.Fprintf(e.out, " *   basicUser + basicPassword: string -> Authorization: Basic <base64>\n")
	}
	if e.authNeeds["apiKey-header"] {
		fmt.Fprintf(e.out, " *   apiKey: string -> %s header\n", e.apiKeyName)
	}
	if e.authNeeds["apiKey-query"] {
		fmt.Fprintf(e.out, " *   apiKey: string -> ?%s=... query parameter\n", e.apiKeyName)
	}
	if e.authNeeds["apiKey-cookie"] {
		fmt.Fprintf(e.out, " *   apiKey: string -> Cookie %s=...\n", e.apiKeyName)
	}
}

func (e *emitter) writeConstructor() {
	fmt.Fprintf(e.out, "    func %sClient(string baseUrl, dict<string, any> auth = {}) {\n", e.name)
	fmt.Fprintf(e.out, "        this.baseUrl = trimTrailingSlash(baseUrl);\n")
	if e.authNeeds["bearer"] {
		fmt.Fprintf(e.out, "        this.bearerToken = auth.contains(\"bearerToken\") ? (auth[\"bearerToken\"] as string) : null;\n")
	}
	if e.authNeeds["basic"] {
		fmt.Fprintf(e.out, "        this.basicUser = auth.contains(\"basicUser\") ? (auth[\"basicUser\"] as string) : null;\n")
		fmt.Fprintf(e.out, "        this.basicPassword = auth.contains(\"basicPassword\") ? (auth[\"basicPassword\"] as string) : null;\n")
	}
	if e.authNeeds["apiKey-header"] || e.authNeeds["apiKey-query"] || e.authNeeds["apiKey-cookie"] {
		fmt.Fprintf(e.out, "        this.apiKey = auth.contains(\"apiKey\") ? (auth[\"apiKey\"] as string) : null;\n")
	}
	fmt.Fprintf(e.out, "    }\n\n")
}

func (e *emitter) writeHeaderHelper() {
	fmt.Fprintf(e.out, "    /* Build the per-request header dict including any configured auth. */\n")
	fmt.Fprintf(e.out, "    func buildHeaders(dict<string, any> extra): dict<string, any> {\n")
	fmt.Fprintf(e.out, "        dict<string, any> h = {};\n")
	if e.authNeeds["bearer"] {
		fmt.Fprintf(e.out, "        if (this.bearerToken != null) {\n")
		fmt.Fprintf(e.out, "            h[\"Authorization\"] = \"Bearer \" + (this.bearerToken as string);\n")
		fmt.Fprintf(e.out, "        }\n")
	}
	if e.authNeeds["basic"] {
		fmt.Fprintf(e.out, "        if (this.basicUser != null && this.basicPassword != null) {\n")
		fmt.Fprintf(e.out, "            let creds = (this.basicUser as string) + \":\" + (this.basicPassword as string);\n")
		fmt.Fprintf(e.out, "            h[\"Authorization\"] = \"Basic \" + encoding.base64Encode(creds);\n")
		fmt.Fprintf(e.out, "        }\n")
	}
	if e.authNeeds["apiKey-header"] {
		fmt.Fprintf(e.out, "        if (this.apiKey != null) {\n")
		fmt.Fprintf(e.out, "            h[%q] = this.apiKey as string;\n", e.apiKeyName)
		fmt.Fprintf(e.out, "        }\n")
	}
	if e.authNeeds["apiKey-cookie"] {
		fmt.Fprintf(e.out, "        if (this.apiKey != null) {\n")
		fmt.Fprintf(e.out, "            h[\"Cookie\"] = %q + \"=\" + (this.apiKey as string);\n", e.apiKeyName)
		fmt.Fprintf(e.out, "        }\n")
	}
	fmt.Fprintf(e.out, "        for (k in extra.keys()) { h[k] = extra[k]; }\n")
	fmt.Fprintf(e.out, "        return h;\n")
	fmt.Fprintf(e.out, "    }\n\n")
}

/* -- Per-operation method emission --------------------------------- */

type opPlan struct {
	path        string
	method      string // GET / POST / ...
	methodName  string
	op          *apiOperation
	pathParams  []*apiParam
	queryParams []*apiParam
	headerParams []*apiParam
	bodyType    string // Geblang type for the request body, "" if none
	bodyJSON    bool   // whether content-type is application/json
	returnType  string // Geblang type for the response
}

func (e *emitter) writeMethods() {
	// Stable iteration order: sort paths, then by HTTP verb within each.
	paths := make([]string, 0, len(e.spec.Paths))
	for p := range e.spec.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	usedNames := map[string]int{}
	for _, p := range paths {
		item := e.spec.Paths[p]
		if item == nil {
			continue
		}
		for _, verb := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
			op := pickOp(item, verb)
			if op == nil {
				continue
			}
			plan := e.buildPlan(p, verb, op, item.Parameters)
			plan.methodName = e.uniqueMethodName(plan, usedNames)
			e.writeMethod(plan)
		}
	}
}

func pickOp(item *pathItem, verb string) *apiOperation {
	switch verb {
	case "GET":
		return item.Get
	case "POST":
		return item.Post
	case "PUT":
		return item.Put
	case "PATCH":
		return item.Patch
	case "DELETE":
		return item.Delete
	case "HEAD":
		return item.Head
	case "OPTIONS":
		return item.Options
	}
	return nil
}

func (e *emitter) buildPlan(path, verb string, op *apiOperation, pathItemParams []*apiParam) *opPlan {
	plan := &opPlan{
		path:   path,
		method: verb,
		op:     op,
	}
	// Merge path-item-level params with operation-level params; operation
	// wins on name+in collisions.
	merged := mergeParams(pathItemParams, op.Parameters)
	for _, p := range merged {
		switch strings.ToLower(p.In) {
		case "path":
			plan.pathParams = append(plan.pathParams, p)
		case "query":
			plan.queryParams = append(plan.queryParams, p)
		case "header":
			plan.headerParams = append(plan.headerParams, p)
		}
	}
	if op.RequestBody != nil {
		if media, ok := op.RequestBody.Content["application/json"]; ok && media != nil && media.Schema != nil {
			plan.bodyType = e.geblangType(media.Schema, !op.RequestBody.Required)
			plan.bodyJSON = true
		} else {
			// Non-JSON body content surfaces as a raw string so the user
			// can hand-pack it.
			plan.bodyType = "string"
		}
	}
	plan.returnType = e.deriveReturnType(op)
	return plan
}

func mergeParams(base, overlay []*apiParam) []*apiParam {
	if len(overlay) == 0 {
		return base
	}
	out := make([]*apiParam, 0, len(base)+len(overlay))
	seen := map[string]bool{}
	key := func(p *apiParam) string { return strings.ToLower(p.In) + "|" + p.Name }
	for _, p := range overlay {
		out = append(out, p)
		seen[key(p)] = true
	}
	for _, p := range base {
		if !seen[key(p)] {
			out = append(out, p)
		}
	}
	return out
}

func (e *emitter) deriveReturnType(op *apiOperation) string {
	// Prefer 200, then 201/2xx, then default. Use the JSON schema.
	preferred := []string{"200", "201", "202", "default"}
	for _, code := range preferred {
		if r, ok := op.Responses[code]; ok && r != nil {
			if media, ok2 := r.Content["application/json"]; ok2 && media != nil && media.Schema != nil {
				return e.geblangType(media.Schema, false)
			}
		}
	}
	// Walk any other 2xx response.
	for code, r := range op.Responses {
		if !strings.HasPrefix(code, "2") || r == nil {
			continue
		}
		if media, ok := r.Content["application/json"]; ok && media != nil && media.Schema != nil {
			return e.geblangType(media.Schema, false)
		}
	}
	// Plain text / unknown body type / no content (204) -> string.
	return "string"
}

func (e *emitter) uniqueMethodName(plan *opPlan, used map[string]int) string {
	base := plan.op.OperationID
	if base == "" {
		base = deriveMethodName(plan.method, plan.path)
	}
	base = camelCase(base)
	candidate := base
	used[candidate]++
	if used[candidate] > 1 {
		candidate = fmt.Sprintf("%s%d", base, used[candidate])
	}
	return candidate
}

func deriveMethodName(verb, path string) string {
	verbPrefix := strings.ToLower(verb)
	segments := strings.Split(strings.Trim(path, "/"), "/")
	parts := []string{verbPrefix}
	for _, seg := range segments {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			parts = append(parts, "By"+pascal(seg[1:len(seg)-1]))
		} else {
			parts = append(parts, pascal(seg))
		}
	}
	return strings.Join(parts, "")
}

func (e *emitter) writeMethod(plan *opPlan) {
	op := plan.op
	// Docstring
	fmt.Fprintf(e.out, "    /**\n")
	if op.Summary != "" {
		fmt.Fprintf(e.out, "     * %s\n", oneLine(op.Summary))
	}
	if op.Description != "" {
		fmt.Fprintf(e.out, "     * %s\n", oneLine(op.Description))
	}
	if op.Summary == "" && op.Description == "" {
		fmt.Fprintf(e.out, "     * %s %s\n", plan.method, plan.path)
	}
	if op.Deprecated {
		fmt.Fprintf(e.out, "     * @deprecated\n")
	}
	fmt.Fprintf(e.out, "     */\n")

	// Signature
	params := e.buildParamList(plan)
	fmt.Fprintf(e.out, "    func %s(%s): %s {\n", plan.methodName, params, plan.returnType)

	// URL construction
	fmt.Fprintf(e.out, "        string path = %s;\n", e.pathExpr(plan))
	e.writeQueryString(plan)
	fmt.Fprintf(e.out, "        string url = this.baseUrl + path%s;\n", queryAppend(plan))

	// Header construction
	e.writeMethodHeaders(plan)

	// Body construction
	if plan.bodyType != "" {
		if plan.bodyJSON {
			fmt.Fprintf(e.out, "        string bodyStr = json.stringify(body);\n")
			fmt.Fprintf(e.out, "        headers[\"Content-Type\"] = \"application/json\";\n")
		} else {
			fmt.Fprintf(e.out, "        string bodyStr = body;\n")
		}
	}

	// HTTP call
	if plan.bodyType != "" {
		fmt.Fprintf(e.out, "        let r = http.requestWithOptions({\"method\": %q, \"url\": url, \"body\": bodyStr, \"headers\": headers});\n", plan.method)
	} else {
		fmt.Fprintf(e.out, "        let r = http.requestWithOptions({\"method\": %q, \"url\": url, \"headers\": headers});\n", plan.method)
	}

	// Status check + response decoding
	fmt.Fprintf(e.out, "        let status = r[\"status\"] as int;\n")
	fmt.Fprintf(e.out, "        if (status < 200 || status >= 300) {\n")
	fmt.Fprintf(e.out, "            throw RuntimeError(%q + \" failed with \" + (status as string) + \": \" + (r[\"body\"] as string));\n", plan.method+" "+plan.path)
	fmt.Fprintf(e.out, "        }\n")
	e.writeReturn(plan)

	fmt.Fprintf(e.out, "    }\n\n")
}

func (e *emitter) buildParamList(plan *opPlan) string {
	// Required path params first, then required others, then optional ones
	// with default = null. Geblang requires defaulted params to come last.
	var required []string
	var optional []string
	for _, p := range plan.pathParams {
		required = append(required, fmt.Sprintf("%s %s", e.geblangType(p.Schema, false), paramName(p.Name)))
	}
	all := append([]*apiParam{}, plan.queryParams...)
	all = append(all, plan.headerParams...)
	for _, p := range all {
		t := e.geblangType(p.Schema, !p.Required)
		if p.Required {
			required = append(required, fmt.Sprintf("%s %s", t, paramName(p.Name)))
		} else {
			optional = append(optional, fmt.Sprintf("%s %s = null", t, paramName(p.Name)))
		}
	}
	if plan.bodyType != "" {
		if strings.HasPrefix(plan.bodyType, "?") {
			optional = append(optional, fmt.Sprintf("%s body = null", plan.bodyType))
		} else {
			required = append(required, fmt.Sprintf("%s body", plan.bodyType))
		}
	}
	all2 := append(required, optional...)
	return strings.Join(all2, ", ")
}

func (e *emitter) pathExpr(plan *opPlan) string {
	if len(plan.pathParams) == 0 {
		return fmt.Sprintf("%q", plan.path)
	}
	// Replace {name} segments with concatenated path-param values.
	// We split the path into literal chunks and interpolation slots.
	remaining := plan.path
	parts := []string{}
	for {
		i := strings.Index(remaining, "{")
		if i < 0 {
			parts = append(parts, fmt.Sprintf("%q", remaining))
			break
		}
		if i > 0 {
			parts = append(parts, fmt.Sprintf("%q", remaining[:i]))
		}
		j := strings.Index(remaining, "}")
		if j < 0 || j < i {
			parts = append(parts, fmt.Sprintf("%q", remaining))
			break
		}
		name := remaining[i+1 : j]
		parts = append(parts, fmt.Sprintf("(%s as string)", paramName(name)))
		remaining = remaining[j+1:]
	}
	return strings.Join(parts, " + ")
}

func (e *emitter) writeQueryString(plan *opPlan) {
	if len(plan.queryParams) == 0 && !(e.authNeeds["apiKey-query"]) {
		fmt.Fprintf(e.out, "        string query = \"\";\n")
		return
	}
	fmt.Fprintf(e.out, "        list<string> qs = [];\n")
	if e.authNeeds["apiKey-query"] {
		fmt.Fprintf(e.out, "        if (this.apiKey != null) {\n")
		fmt.Fprintf(e.out, "            qs = qs.push(%q + \"=\" + (this.apiKey as string));\n", e.apiKeyName)
		fmt.Fprintf(e.out, "        }\n")
	}
	for _, p := range plan.queryParams {
		varName := paramName(p.Name)
		// Always check for null first because Geblang's `+ as string` on
		// a nullable would throw; required params still need value rendering.
		fmt.Fprintf(e.out, "        if (%s != null) {\n", varName)
		fmt.Fprintf(e.out, "            qs = qs.push(%q + \"=\" + (%s as string));\n", p.Name, varName)
		fmt.Fprintf(e.out, "        }\n")
	}
	fmt.Fprintf(e.out, "        string query = qs.length() > 0 ? \"?\" + qs.join(\"&\") : \"\";\n")
}

func queryAppend(plan *opPlan) string {
	if len(plan.queryParams) == 0 {
		return " + query"
	}
	return " + query"
}

func (e *emitter) writeMethodHeaders(plan *opPlan) {
	fmt.Fprintf(e.out, "        dict<string, any> extra = {};\n")
	for _, p := range plan.headerParams {
		varName := paramName(p.Name)
		fmt.Fprintf(e.out, "        if (%s != null) {\n", varName)
		fmt.Fprintf(e.out, "            extra[%q] = %s as string;\n", p.Name, varName)
		fmt.Fprintf(e.out, "        }\n")
	}
	fmt.Fprintf(e.out, "        dict<string, any> headers = this.buildHeaders(extra);\n")
}

func (e *emitter) writeReturn(plan *opPlan) {
	rt := plan.returnType
	if rt == "string" {
		fmt.Fprintf(e.out, "        return r[\"body\"] as string;\n")
		return
	}
	if strings.HasPrefix(rt, "list<") {
		fmt.Fprintf(e.out, "        return json.parse(r[\"body\"] as string) as %s;\n", rt)
		return
	}
	if strings.HasPrefix(rt, "dict<") {
		fmt.Fprintf(e.out, "        return json.parse(r[\"body\"] as string) as %s;\n", rt)
		return
	}
	if rt == "int" || rt == "float" || rt == "decimal" || rt == "bool" {
		fmt.Fprintf(e.out, "        return json.parse(r[\"body\"] as string) as %s;\n", rt)
		return
	}
	if rt == "any" {
		fmt.Fprintf(e.out, "        return json.parse(r[\"body\"] as string);\n")
		return
	}
	// Class type: use parseAs for typed deserialization.
	fmt.Fprintf(e.out, "        return json.parseAs(r[\"body\"] as string, %s);\n", rt)
}

/* -- Small helpers -------------------------------------------------- */

// pascal converts an arbitrary string to PascalCase identifier-safe.
func pascal(s string) string {
	parts := splitIdent(s)
	out := strings.Builder{}
	for _, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		out.WriteString(string(runes))
	}
	r := out.String()
	if r == "" {
		return "X"
	}
	if !unicode.IsLetter([]rune(r)[0]) {
		r = "X" + r
	}
	return r
}

func camelCase(s string) string {
	p := pascal(s)
	if p == "" {
		return "x"
	}
	runes := []rune(p)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func splitIdent(s string) []string {
	out := []string{}
	cur := strings.Builder{}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			cur.WriteRune(r)
		} else {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// fieldName maps an OpenAPI property name to a Geblang field name.
// Geblang field declarations must be identifiers, but we keep the
// original name where it already is one so JSON parsing works
// without rename.
func fieldName(s string) string {
	if isValidIdent(s) {
		return s
	}
	return camelCase(s)
}

func paramName(s string) string {
	if s == "" {
		return "arg"
	}
	if isValidIdent(s) {
		return s
	}
	return camelCase(s)
}

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return !isReserved(s)
}

func isReserved(s string) bool {
	switch s {
	case "let", "func", "class", "interface", "extends", "implements",
		"return", "if", "else", "while", "for", "in", "break", "continue",
		"true", "false", "null", "this", "parent", "import", "module",
		"export", "throw", "try", "catch", "finally", "as", "instanceof",
		"new", "static", "any", "void", "string", "int", "float", "decimal",
		"bool", "list", "set", "dict", "bytes", "callable":
		return true
	}
	return false
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}
