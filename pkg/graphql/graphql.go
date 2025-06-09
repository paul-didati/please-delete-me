package graphql

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/paul-didati/please-delete-me/pkg/graphql/generate"
	_ "github.com/paul-didati/please-delete-me/pkg/graphql/resolutions"
	"github.com/paul-didati/please-delete-me/pkg/graphql/typer"

	"github.com/graph-gophers/graphql-go"
)

type ScanResolver struct {
	Types  map[string]*typer.Type
	Fields []Field
}

type Field struct {
	Name           string
	On             string
	Type           reflect.Type
	UnderlyingType reflect.Type
	Args           any
	Path           string
	Leaf           bool
}

type NamedValue struct {
	Name  string
	Alias string
	Value any
}

type QueryRequest struct {
	Fields []NamedValue
	Args   []NamedValue
}

func ArgsAsNamedValue(a any) ([]NamedValue, bool) {

	v := reflect.ValueOf(a)

	if v.Kind() != reflect.Struct {
		return nil, false
	}

	args := make([]NamedValue, v.NumField())
	for i := 0; i < v.NumField(); i++ {

		name := v.Type().Field(i).Name

		var value any
		if v.Field(i).Kind() == reflect.Ptr {
			value = v.Field(i).Elem().Interface()
		} else {
			value = v.Field(i).Interface()
		}
		args[i] = NamedValue{
			Name:  name,
			Value: value,
		}
	}
	return args, true
}

func getUnderlying(t reflect.Type) reflect.Type {
	if t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		return getUnderlying(t.Elem())
	}
	return t
}

func resolvePathPart(fields []Field, entity string) int {

	for i := len(fields) - 1; i >= 0; i-- {
		if entity == fields[i].UnderlyingType.Name() {
			return i
		}
	}
	return -1
}

func (r *ScanResolver) Resolve(ctx context.Context, entity, field string, args any, zero any, object, list, nullable bool) (any, error) {

	path := field

	idx := resolvePathPart(r.Fields, entity)
	for idx >= 0 {
		path = r.Fields[idx].Name + "." + path
		idx = resolvePathPart(r.Fields[0:idx], r.Fields[idx].On)
	}

	r.Fields = append(r.Fields, Field{
		On:             entity,
		Name:           field,
		Args:           args,
		Type:           reflect.TypeOf(zero),
		UnderlyingType: getUnderlying(reflect.TypeOf(zero)),
		Path:           strings.ToLower(path),
		Leaf:           !object && !list,
	})

	// Because this is neither a object or a list we return the zero type because we won't ever attempt
	// to expand it's sub values.
	if !object && !list {
		return zero, nil
	}

	if object {
		_, ok := r.Types[field]
		if ok {
			objPtr := reflect.New(reflect.TypeOf(zero).Elem()).Interface()
			return objPtr, nil
		}
		return zero, fmt.Errorf("no such object %v", field)
	}

	if list {

		listType := reflect.TypeOf(zero).Elem()
		var pointer bool
		if listType.Kind() == reflect.Ptr {
			pointer = true
			listType = listType.Elem()
		}

		objPtr := reflect.MakeSlice(reflect.TypeOf(zero), 0, 0)
		innerVal := reflect.New(listType)
		if !pointer {
			innerVal.Elem()
		}

		objPtr = reflect.Append(objPtr, innerVal)
		return objPtr.Interface(), nil
	}

	return nil, fmt.Errorf("unimplemented")
}

func Query(ctx context.Context, schema *graphql.Schema, query string) error {
	out := schema.Exec(ctx, query, "", nil)
	if len(out.Errors) > 0 {
		return fmt.Errorf("grapqhl query errors: %v", out.Errors)
	}
	return nil
}

func BuildScannableSchema(ctx context.Context, schemaText, buildPath string) (*graphql.Schema, *ScanResolver, error) {

	schema, err := graphql.ParseSchema(schemaText, nil)
	if err != nil {
		return nil, nil, err
	}

	types, objectSet := typer.ParseTypes(schema.Inspect().Types())

	gen := generate.TypesToTmpl(types)

	r := &ScanResolver{
		Types: objectSet,
	}

	resolvers, err := generate.ProduceResolvers(ctx, gen, buildPath, r)
	if err != nil {
		return nil, nil, err
	}

	schema, err = graphql.ParseSchema(schemaText, resolvers)
	return schema, r, err
}
