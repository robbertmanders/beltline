package api_test

import (
	"fmt"
	"go/constant"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The wire contract as TypeScript. TestTypeScript type-checks this package and the core types it
// reaches with go/types, then writes Web/src/api/wire.gen.ts, the types the web app's requests and
// responses are checked against. Run it with BELTLINE_UPDATE_GOLDENS=1 after a wire change.
//
// Names: a type's TypeScript name is its Go name without "Wire", when that stays unique;
// unexported wire forms are capitalised. Fields follow their json tags: omitzero, and omitempty on
// anything but a struct, make a field optional; a pointer that is not omitted may be null; a named
// string type with constants is a union of them; time.Time and jsonx.Date are strings.
//
// A type with its own MarshalJSON needs an entry: tsOverrides gives its TypeScript text, and
// tsRedirects names the struct its encoding writes. Any other such type fails the test.

const tsOutputPath = "../../../../Web/src/api/wire.gen.ts"

// tsOverrides is the TypeScript text of a custom encoding, by Go type name. A core type it
// mentions is listed in tsRoots.
var tsOverrides = map[string]string{
	"Trigger": `({ kind: "issues" } & IssueFilter) | ({ kind: "reviews" } & ReviewFilter) | ({ kind: "jira" } & JiraQuery) | ` +
		`({ kind: "schedule" } & ScheduledTask) | { kind: "manual" }`,
	"Then": `{ do: ThenAction; to?: string | string[]; route?: string | string[]; withOutput?: boolean; max?: number; comment?: boolean }`,
	"ResourceRef": `{ issue: { repo: string; number: number } } | { pullRequest: { repo: string; number: number } } | ` +
		`{ ticket: { key: string } }`,
	"RouteSet":         `Record<string, string | string[]>`,
	"RunPauseDeferred": `{ merged: { _0: string[] } } | { conflict: { _0: string[] } }`,
	"FactoryTrackNode": `{ kind: "entrance" | "stage"; value: string } | { kind: "waypoint"; value: number }`,
	"MarkdownBlock": `{ type: "heading"; level: number; text: string } | { type: "item"; marker: string; text: string } | ` +
		`{ type: "code" | "paragraph"; text: string }`,
	"RunSnapshot": `Run & { actions: RunAction[] }`,
}

// tsRoots lists the types the generator starts from besides the exported types of this package:
// the responses handlers encode from other packages, and the core types the overrides name.
var tsRoots = []string{
	"github.com/robbertmanders/beltline/daemon/internal/core.InsightsEvolutionView",
	"github.com/robbertmanders/beltline/daemon/internal/core.InsightsView",
	"github.com/robbertmanders/beltline/daemon/internal/core.IssueFilter",
	"github.com/robbertmanders/beltline/daemon/internal/core.JiraQuery",
	"github.com/robbertmanders/beltline/daemon/internal/core.Run",
	"github.com/robbertmanders/beltline/daemon/internal/core.RunAction",
	"github.com/robbertmanders/beltline/daemon/internal/core.ReviewFilter",
	"github.com/robbertmanders/beltline/daemon/internal/core.ScheduledTask",
	"github.com/robbertmanders/beltline/daemon/internal/core.StageMenu",
	"github.com/robbertmanders/beltline/daemon/internal/core.StageLinePresentation",
	"github.com/robbertmanders/beltline/daemon/internal/core.ThenAction",
	"github.com/robbertmanders/beltline/daemon/internal/core.TriggerKind",
	"github.com/robbertmanders/beltline/daemon/internal/core.TriggerFormPresentation",
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator.AssistantConfiguration",
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator.BeltContext",
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator.ModelRunnerResource",
	"github.com/robbertmanders/beltline/daemon/internal/kit/coordinator.SessionsView",
	"github.com/robbertmanders/beltline/daemon/internal/server/routes.ArtifactLinkResponse",
	"github.com/robbertmanders/beltline/daemon/internal/server/routes.DesktopSuccess",
}

// tsRedirects names the struct a custom encoding writes, by Go type name (same package).
var tsRedirects = map[string]string{
	"Belt":  "Belt",
	"Stage": "stageWire",
}

// tsExtra declares TypeScript for wire values this package does not define: the event messages
// (the server's EventMessage and EventResource) and the version response.
var tsExtra = []string{
	`export const eventResourceValues = ["runs", "library", "settings", "state", "intake", "assistant", "afk", "models"] as const;`,
	`export type EventResource = (typeof eventResourceValues)[number];`,
	`export type EventMessage = { type: "hello"; apiVersion: number } | { type: "changed"; resource: EventResource; revision: number } | ` +
		`{ type: "notification"; title: string; body: string };`,
	`export type VersionResponse = { apiVersion: number; platform: "macos" | "linux"; version: string };`,
}

type tsType struct {
	base   string        // Go name
	name   string        // TypeScript name, set once every type is known
	st     *types.Struct // the members of a struct
	text   string        // override text, or "string" for a string alias
	values []string      // the members of an enum
}

type tsGen struct {
	types map[*types.Named]*tsType
	order []*types.Named
	errs  []string
}

// TestTypeScript checks that Web/src/api/wire.gen.ts matches the Go wire types.
func TestTypeScript(t *testing.T) {
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	pkg, err := imp.Import("github.com/robbertmanders/beltline/daemon/internal/server/api")
	if err != nil {
		t.Fatalf("type-check the api package: %v", err)
	}
	src, errs := generateTypeScript(imp, pkg)
	if len(errs) > 0 {
		t.Fatalf("cannot generate the TypeScript wire types:\n%s", strings.Join(errs, "\n"))
	}
	path := filepath.FromSlash(tsOutputPath)
	if os.Getenv("BELTLINE_UPDATE_GOLDENS") == "1" {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != src {
		t.Fatalf("%s is out of date with the Go wire types; run BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestTypeScript", tsOutputPath)
	}
}

// generateTypeScript returns the file contents, and the problems that stopped it.
func generateTypeScript(imp types.Importer, pkg *types.Package) (string, []string) {
	g := &tsGen{types: map[*types.Named]*tsType{}}
	for _, name := range pkg.Scope().Names() {
		if tn, ok := pkg.Scope().Lookup(name).(*types.TypeName); ok && tn.Exported() && !tn.IsAlias() {
			if n, ok := tn.Type().(*types.Named); ok {
				g.expr(n)
			}
		}
	}
	for _, path := range tsRoots {
		i := strings.LastIndex(path, ".")
		pkgPath, name := path[:i], path[i+1:]
		other, err := imp.Import(pkgPath)
		if err != nil {
			return "", []string{err.Error()}
		}
		n, ok := pkgScopeType(other, name)
		if !ok {
			return "", []string{path + " is not a named type"}
		}
		g.expr(n)
	}
	for i := 0; i < len(g.order); i++ { // a struct's fields may reach more types
		if e := g.types[g.order[i]]; e.st != nil {
			g.fields(e.st)
		}
	}
	g.nameAll()
	decls := make([]string, 0, len(g.order)+len(tsExtra))
	for _, n := range g.order {
		decls = append(decls, g.declare(g.types[n]))
	}
	decls = append(decls, tsExtra...)
	sort.Strings(decls)
	header := "// Generated from the Go wire types by TestTypeScript. Do not edit: run\n" +
		"// BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestTypeScript.\n\n"
	return header + strings.Join(decls, "\n"), g.errs
}

// nameAll gives every type its TypeScript name: the Go name without its Wire suffix where that is unique.
func (g *tsGen) nameAll() {
	count := map[string]int{}
	for _, n := range g.order {
		count[tsCandidate(g.types[n].base)]++
	}
	seen := map[string]string{}
	for _, n := range g.order {
		e := g.types[n]
		e.name = tsCandidate(e.base)
		if count[e.name] > 1 {
			e.name = e.base
		}
		if other, dup := seen[e.name]; dup {
			g.errs = append(g.errs, fmt.Sprintf("TypeScript name %s names both %s and %s", e.name, other, e.base))
		}
		seen[e.name] = e.base
	}
}

// tsCandidate is a Go type name's TypeScript name before collisions are checked.
func tsCandidate(base string) string {
	name := strings.ReplaceAll(base, "Wire", "")
	return strings.ToUpper(name[:1]) + name[1:]
}

// declare writes one type's declaration.
func (g *tsGen) declare(e *tsType) string {
	switch {
	case e.st != nil:
		var b strings.Builder
		fmt.Fprintf(&b, "export interface %s {\n", e.name)
		for _, f := range g.fields(e.st) {
			fmt.Fprintf(&b, "  %s\n", f)
		}
		b.WriteString("}\n")
		return b.String()
	case e.values != nil:
		quoted := make([]string, len(e.values))
		for i, v := range e.values {
			quoted[i] = strconv.Quote(v)
		}
		v := strings.ToLower(e.name[:1]) + e.name[1:]
		return fmt.Sprintf("export const %sValues = [%s] as const;\nexport type %s = (typeof %sValues)[number];\n",
			v, strings.Join(quoted, ", "), e.name, v)
	}
	return fmt.Sprintf("export type %s = %s;\n", e.name, e.text)
}

// expr returns the TypeScript type of a Go type, registering the named types it reaches.
func (g *tsGen) expr(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		switch {
		case t.Info()&types.IsString != 0:
			return "string"
		case t.Info()&types.IsBoolean != 0:
			return "boolean"
		case t.Info()&types.IsNumeric != 0:
			return "number"
		}
	case *types.Pointer:
		return g.expr(t.Elem()) + " | null"
	case *types.Slice:
		return wrapUnion(g.expr(t.Elem())) + "[]"
	case *types.Array:
		return wrapUnion(g.expr(t.Elem())) + "[]"
	case *types.Map:
		return "Record<string, " + g.expr(t.Elem()) + ">"
	case *types.Named:
		return g.named(t)
	case *types.Struct:
		return "{ " + strings.Join(g.fields(t), " ") + " }"
	}
	return "unknown"
}

func wrapUnion(s string) string {
	if strings.Contains(s, " | ") {
		return "(" + s + ")"
	}
	return s
}

// named returns a named type's TypeScript name, registering the ones that have a declaration of their own.
func (g *tsGen) named(n *types.Named) string {
	obj := n.Obj()
	if pkg := obj.Pkg(); pkg != nil {
		if pkg.Path() == "time" && obj.Name() == "Time" || strings.HasSuffix(pkg.Path(), "wire/jsonx") && obj.Name() == "Date" {
			return "string"
		}
	} else {
		return "unknown" // error
	}
	if e := g.types[n]; e != nil {
		return e.name
	}
	name := obj.Name()
	if name == "setOf" && n.TypeArgs().Len() == 1 { // a set is written as a sorted array of its members
		return wrapUnion(g.expr(n.TypeArgs().At(0))) + "[]"
	}
	override, hasOverride := tsOverrides[name]
	redirect, hasRedirect := tsRedirects[name]
	if hasMarshal(n) && !hasOverride && !hasRedirect {
		g.errs = append(g.errs, fmt.Sprintf("%s has its own MarshalJSON: add it to tsOverrides or tsRedirects", name))
	}
	if hasOverride && n.TypeArgs().Len() > 0 {
		return override // a generic instance is written where it is used
	}
	e := &tsType{base: name}
	switch u := n.Underlying().(type) {
	case *types.Struct:
		e.st = u
	case *types.Basic:
		if u.Info()&types.IsString == 0 || hasOverride {
			break
		}
		if e.values = g.enumValues(n); len(e.values) == 0 {
			e.text = "string"
		}
	default:
		if !hasOverride && !hasRedirect {
			return g.expr(u) // a named slice or map is written as its underlying type
		}
	}
	switch {
	case hasOverride:
		e.text, e.st, e.values = override, nil, nil
	case hasRedirect:
		target, ok := pkgScopeType(obj.Pkg(), redirect)
		if !ok {
			g.errs = append(g.errs, fmt.Sprintf("%s redirects to %s, which is not a struct in its package", name, redirect))
			return "unknown"
		}
		e.st, _ = target.Underlying().(*types.Struct)
	case e.st == nil && e.values == nil && e.text == "":
		return g.expr(n.Underlying())
	}
	g.types[n] = e
	g.order = append(g.order, n)
	return name
}

// fields returns a struct's TypeScript members in declaration order, flattening embedded structs as
// encoding/json does: a key the outer struct declares hides the embedded one.
func (g *tsGen) fields(st *types.Struct) []string {
	return g.fieldsHiding(st, nil)
}

func (g *tsGen) fieldsHiding(st *types.Struct, hidden map[string]bool) []string {
	own := map[string]bool{}
	for key := range hidden {
		own[key] = true
	}
	for i := 0; i < st.NumFields(); i++ {
		key, _, _ := strings.Cut(reflect.StructTag(st.Tag(i)).Get("json"), ",")
		if f := st.Field(i); !f.Anonymous() || key != "" {
			if key == "" {
				key = f.Name()
			}
			own[key] = true
		}
	}
	var out []string
	for i := 0; i < st.NumFields(); i++ {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		if tag == "-" {
			continue
		}
		key, opts, _ := strings.Cut(tag, ",")
		if f.Anonymous() && key == "" {
			if inner, ok := derefType(f.Type()).Underlying().(*types.Struct); ok {
				out = append(out, g.fieldsHiding(inner, own)...)
			}
			continue
		}
		if !f.Exported() {
			continue
		}
		if key == "" {
			key = f.Name()
		}
		if hidden[key] {
			continue
		}
		_, isStruct := f.Type().Underlying().(*types.Struct)
		optional := strings.Contains(opts, "omitzero") || strings.Contains(opts, "omitempty") && !isStruct
		var typ string
		if p, ok := f.Type().(*types.Pointer); ok && optional {
			typ = g.expr(p.Elem())
		} else {
			typ = g.expr(f.Type())
		}
		mark := ""
		if optional {
			mark = "?"
		}
		out = append(out, fmt.Sprintf("%s%s: %s;", propertyName(key), mark, typ))
	}
	return out
}

func derefType(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// propertyName quotes a JSON key that is not a plain identifier.
func propertyName(key string) string {
	for i, r := range key {
		letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		digit := i > 0 && r >= '0' && r <= '9'
		if !letter && !digit {
			return strconv.Quote(key)
		}
	}
	return key
}

// hasMarshal reports whether a type encodes itself with a MarshalJSON method.
func hasMarshal(n *types.Named) bool {
	return types.NewMethodSet(types.NewPointer(n)).Lookup(nil, "MarshalJSON") != nil
}

// enumValues lists a named string type's constants in declaration order.
func (g *tsGen) enumValues(n *types.Named) []string {
	scope := n.Obj().Pkg().Scope()
	var consts []*types.Const
	for _, name := range scope.Names() {
		if c, ok := scope.Lookup(name).(*types.Const); ok && types.Identical(c.Type(), n) && c.Val().Kind() == constant.String {
			consts = append(consts, c)
		}
	}
	slices.SortFunc(consts, func(a, b *types.Const) int { return int(a.Pos() - b.Pos()) })
	var values []string
	for _, c := range consts {
		if v := constant.StringVal(c.Val()); !slices.Contains(values, v) { // an alias constant repeats a value
			values = append(values, v)
		}
	}
	return values
}

func pkgScopeType(pkg *types.Package, name string) (*types.Named, bool) {
	tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
	if !ok {
		return nil, false
	}
	n, ok := tn.Type().(*types.Named)
	return n, ok
}
