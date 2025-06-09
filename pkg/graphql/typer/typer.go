package typer

import (
	"fmt"
	"strings"

	"github.com/graph-gophers/graphql-go/introspection"
)

func (t *Type) TypeIsObject(name string) *Type {

	if t.Named == name {
		return t
	}

	if t.Nullable != nil {
		return t.TypeIsObject(name)
	}

	if t.List != nil {
		return t.TypeIsObject(name)
	}

	for _, v := range t.Object {
		if v.Field.TypeIsObject(name) != nil {
			return v.Field
		}
	}
	return nil
}

// Type is a representation of a graphql primative (nullable, scalar, list, object or enum).
type Type struct {
	Nullable   *Type
	Scalar     Kind
	List       *Type
	Named      string
	Interface  bool
	Implements []string
	Object     map[string]ObjectField
	Enum       []string
}

// Kind is the base value of a scalar.
type Kind struct {
	Value string
}

// ObjectField is a field of a graphql object. This is the underlying field, as well as any arguments to the
// function to fetch the field.
type ObjectField struct {
	Arguments []Argument
	Field     *Type
}

// Argument is a graphql function argument that has a name and a type.
// TODO(should also provide optional default).
type Argument struct {
	Name string
	Type *Type
}

// ToGoType returns the go type of the graphql type as a string.
// Objects are always pointers to the Type suffixed with Resolver.
func (t *Type) ToGoType() string {

	if t.Named != "" {
		return "*" + t.Named + "Resolver"
	} else if t.List != nil {
		return "[]" + t.List.ToGoType()
	} else if t.Nullable != nil {
		inner := t.Nullable.ToGoType()
		if inner[0] != '*' {
			return "*" + inner
		}
		return inner
	} else if t == nil {
		return "invalid nil type"
	} else if t.Enum != nil {
		return "string"
	} else {

		switch t.Scalar.Value {
		case "String":
			return "string"
		case "Float":
			return "float64"
		case "ID":
			return "graphql.ID"
		case "Int":
			return "int32"
		case "Boolean":
			return "bool"
		}
		return t.Scalar.Value
	}
}

// TypeTracker is used to cache type names and references to them. It's sole purpose is to
// resolver recusive references to graphql types.
type TypeTracker struct {
	atoms map[string]*Type
}

// resolveType attempts to rescusively resolve the graphql type a type. Nullable is a boxing around an internal type, so we have to unbox
// null tell the inner type that it's parent was nullable
func (tt *TypeTracker) resolveType(nullable bool, t *introspection.Type) *Type {

	switch t.Kind() {
	case "NON_NULL":
		return tt.resolveType(false, t.OfType())

	case "SCALAR":
		kindValue := "invalid scalar"
		if t.Name() != nil {
			kindValue = *t.Name()
		}
		r := &Type{
			Scalar: Kind{
				Value: kindValue,
			},
		}
		tt.atoms[kindValue] = r
		if nullable {
			return &Type{
				Nullable: r,
			}
		}
		return r

		// Note(paul-didati): a INPUT_OBJECT is the same as an object but can not have interface or union fields nor can it define arguments. This
		// is part of the grapqhl spec our type manager does need to care about this distiction.
		// https://stackoverflow.com/questions/41743253/whats-the-point-of-input-type-in-graphql
	case "OBJECT", "INTERFACE", "INPUT_OBJECT":

		var interfaces []string
		if t.Interfaces() != nil {
			for _, i := range *t.Interfaces() {
				if i.Name() != nil {
					interfaces = append(interfaces, *i.Name())
				}
			}
		}

		obj := &Type{
			Implements: interfaces,
			Interface:  t.Kind() == "INTERFACE",
		}

		if t.Name() != nil {
			obj.Named = *t.Name()
			r, ok := tt.atoms[*t.Name()]
			if !ok {
				tt.atoms[*t.Name()] = obj
			} else {
				return r
			}
		}

		fields := make(map[string]ObjectField)

		if t.Fields(nil) != nil {
			for _, field := range *t.Fields(nil) {
				var fieldArgs []Argument
				for _, arg := range field.Args() {
					fieldArgs = append(fieldArgs, Argument{
						Name: strings.Title(arg.Name()),
						Type: tt.resolveType(true, arg.Type()),
						// TODO default?
					})
				}

				fieldType := tt.resolveType(true, field.Type())
				fields[field.Name()] = ObjectField{
					Field:     fieldType,
					Arguments: fieldArgs,
				}
			}

			obj.Object = fields
		}
		if nullable {
			return &Type{
				Nullable: obj,
			}
		}
		return obj

	case "LIST":
		t := &Type{
			List: tt.resolveType(true, t.OfType()),
		}
		if nullable {
			return &Type{
				Nullable: t,
			}
		}
		return t

	case "ENUM":

		var valSet []string
		values := t.EnumValues(nil)
		if values != nil {
			for _, val := range *values {
				valSet = append(valSet, val.Name())
			}
		}

		t := &Type{
			Enum: valSet,
		}

		if nullable {
			return &Type{
				Nullable: t,
			}
		}
		return t

	case "UNION":
		fmt.Println("union", t)
	default:
		fmt.Println("unknown kind", t.Kind())
	}

	return nil
}

// ParseTypes gets our type representation from a graphql schema.
func ParseTypes(input []*introspection.Type) ([]*Type, map[string]*Type) {

	tt := TypeTracker{
		atoms: make(map[string]*Type),
	}

	var types []*Type

	for _, t := range input {

		// Skip interal types that are always in a graphql schema.
		if t.Name() == nil {
			continue
		} else if strings.HasPrefix(*t.Name(), "_") {
			continue
		} else {
			switch *t.Name() {
			case "String", "Boolean", "Float", "Int", "ID":
				continue
			}
		}

		r := tt.resolveType(true, t)

		// All top level objects are nullable in a graphql schema.
		types = append(types, r)
	}

	return types, tt.atoms
}

func StripDefaultTypes(types []*Type) []*Type {
	for i := len(types); i > 0; i-- {

	}
	return types
}
