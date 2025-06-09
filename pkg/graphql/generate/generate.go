package generate

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"plugin"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/paul-didati/please-delete-me/pkg/graphql/resolutions"
	"github.com/paul-didati/please-delete-me/pkg/graphql/typer"

	_ "embed"
)

//go:embed generated.go.tmpl
var templateSource string

var last string

// Generator is the struct the templator uses to build the template.
type Generator struct {
	Resolvers []Resolver
}

// Resolver represents the Go Type produced for each resolver.
type Resolver struct {
	Name       string
	Interface  bool
	Implements []Implement
	Fields     []Field
}

// Implement is the Mapping from Object to interface it implements.
type Implement struct {
	To   string
	From string
}

// Field is a method on a resolver.
type Field struct {
	Object       bool
	Nullable     bool
	List         bool
	Args         []Arg
	Field        string
	HasArg       bool
	ResponseType string
}

// Arg is one argument to the args struct in a method.
type Arg struct {
	Name string
	Type string
}

// TypesToTmpl converts a set of graphql Types into a template struct.
func TypesToTmpl(types []*typer.Type) Generator {

	g := Generator{}

	var queue []*typer.Type

	queue = append(queue, types...)

	for len(queue) > 0 {

		val := queue[0]
		queue = queue[1:]

		if val.Nullable != nil {
			queue = append(queue, val.Nullable)
		}

		if val.Named == "" {
			continue
		}

		var imps []Implement
		for _, i := range val.Implements {
			imps = append(imps, Implement{
				To:   val.Named,
				From: i,
			})
		}

		r := Resolver{
			Name:       val.Named,
			Interface:  val.Interface,
			Implements: imps,
		}

		for key, field := range val.Object {

			f := Field{
				Object:       field.Field.Named != "",
				List:         field.Field.List != nil,
				Nullable:     field.Field.Nullable != nil,
				Field:        strings.Title(key),
				ResponseType: field.Field.ToGoType(),
			}

			f.HasArg = len(field.Arguments) > 0

			for _, arg := range field.Arguments {
				f.Args = append(f.Args, Arg{
					Name: arg.Name,
					Type: arg.Type.ToGoType(),
				})
			}

			r.Fields = append(r.Fields, f)
		}

		g.Resolvers = append(g.Resolvers, r)
	}

	return g
}

// ProduceSchema takes a tmpl struct with Grapqhl type information.
// First it creates a file gen.go containing the templated Go code.
// Then it os.Execs a go build command to build the plugin. Then it loads the Create symbol
// in the generated shared object file. Then it calls Create producing the graphql.Schema with generated resolvers.
func ProduceResolvers(ctx context.Context, g Generator, path string, r resolutions.Resolver) (any, error) {

	now := time.Now().Unix()
	dirName := fmt.Sprintf("%d", now)
	path = filepath.Join(path, dirName)
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, err
	}

	data, err := gen(g)
	if err != nil {
		return nil, err
	}

	file := filepath.Join(path, "generated.go")

	if err = os.WriteFile(file, data.Bytes(), os.ModePerm); err != nil {
		return nil, err
	}

	err = build(ctx, path, file)
	if err != nil {
		return nil, err
	}

	file = filepath.Join(path, "generated.so")
	return load(file, r)
}

func replaceFirstLine(filename, newFirstLine string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	// Skip first line and read the rest
	if scanner.Scan() {
		lines = append(lines, newFirstLine) // Replace first line
	}

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Write back to file
	return os.WriteFile(filename, []byte(strings.Join(lines, "\n")), 0644)
}

// gen templates the template file with the generator struct.
func gen(g Generator) (*bytes.Buffer, error) {

	tmpl, err := template.New("generated.go.tmpl").Parse(templateSource)
	if err != nil {
		return nil, err
	}

	data := bytes.NewBuffer(nil)

	if err = tmpl.Execute(data, g); err != nil {
		return nil, err
	}
	return data, nil
}

// build os.Execs a go plugin build at the given path.
func build(ctx context.Context, path, file string) error {

	output := "generated.so"
	output = filepath.Join(path, output)

	fmt.Printf("Go version: %s\n", runtime.Version())
	fmt.Printf("GOOS: %s, GOARCH: %s\n", runtime.GOOS, runtime.GOARCH)

	cmd := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildmode=plugin", "-o", output, file)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// load opens the shareed object file and attempts to call the symbol Create. Which returns the rooted resolvers.
// those resolvers are provided and validated with the schema, returning a useable graphql schema.
func load(path string, r resolutions.Resolver) (any, error) {

	p, err := plugin.Open(path)
	if err != nil && !strings.Contains(err.Error(), "plugin already loaded") {
		return nil, err
	}

	builder, err := p.Lookup("Create")
	if err != nil {
		return nil, err
	}

	root := builder.(func(resolutions.Resolver) any)(r)

	return root, nil
}
