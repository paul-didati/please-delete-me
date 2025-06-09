package typer

import (
	"errors"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/graph-gophers/graphql-go"
)

func TestParseTypes(t *testing.T) {

	type Test struct {
		Name     string
		Schema   string
		Expect   []*Type
		Validate func(got []*Type) error
	}

	tests := []Test{
		{
			Name: "single-int",
			Schema: `type Query{
        a: Int
      }`,
			Expect: []*Type{
				{
					Nullable: &Type{
						Named: "Query",
						Object: map[string]ObjectField{
							"a": {
								Field: &Type{
									Nullable: &Type{
										Scalar: Kind{Value: "Int"},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			Name: "single-simple-type",
			Schema: `type Query{
			        a: Apple
			      }

			      type Apple {
			        s: String
			      }
			      `,
			Expect: []*Type{
				{
					Nullable: &Type{
						Named: "Apple",
						Object: map[string]ObjectField{
							"s": {
								Field: &Type{
									Nullable: &Type{
										Scalar: Kind{Value: "String"},
									},
								},
							},
						},
					},
				},
				{
					Nullable: &Type{
						Named: "Query",
						Object: map[string]ObjectField{
							"a": {
								Field: &Type{
									Named: "Apple",
									Object: map[string]ObjectField{
										"s": {
											Field: &Type{
												Nullable: &Type{
													Scalar: Kind{Value: "String"},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			Validate: func(got []*Type) error {
				// expect Apple type to be referenced in Query (Type pointers should match)
				if got[0].Nullable != got[1].Nullable.Object["a"].Field {
					return errors.New("Apple type not referenced in Query")
				}
				return nil
			},
		},
	}

	for _, test := range tests {

		schema := graphql.MustParseSchema(test.Schema, nil)

		types, _ := ParseTypes(schema.Inspect().Types())

		if ok := reflect.DeepEqual(types, test.Expect); !ok {
			spew.Dump("want", test.Expect)
			spew.Dump("got", types)
			t.Errorf("schema %s expected %v got %v", t.Name(), test.Expect, types)
		}

		if test.Validate != nil {
			if err := test.Validate(types); err != nil {
				t.Errorf("schema %s validation failed: %v", t.Name(), err)
			}
		}
	}
}
