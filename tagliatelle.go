// Package tagliatelle a linter that handle struct tags.
package tagliatelle

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"reflect"
	"strings"

	"github.com/ldez/tagliatelle/filedtype"

	"github.com/ettle/strcase"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Config the tagliatelle configuration.
type Config struct {
	Rules        map[string]string
	UseFieldName bool
}

// New creates an analyzer.
func New(config Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "tagliatelle",
		Doc:  "Checks the struct tags.",
		Run: func(pass *analysis.Pass) (interface{}, error) {
			if len(config.Rules) == 0 {
				return nil, nil
			}

			return run(pass, config)
		},
		Requires: []*analysis.Analyzer{
			inspect.Analyzer,
		},
	}
}

func run(pass *analysis.Pass, config Config) (interface{}, error) {
	isp, ok := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	if !ok {
		return nil, errors.New("missing inspect analyser")
	}

	nodeFilter := []ast.Node{
		(*ast.StructType)(nil),
	}

	isp.Preorder(nodeFilter, func(n ast.Node) {
		node, ok := n.(*ast.StructType)
		if !ok {
			return
		}

		for _, field := range node.Fields.List {
			analyze(pass, config, node, field)
		}
	})

	return nil, nil
}

func analyze(pass *analysis.Pass, config Config, n *ast.StructType, field *ast.Field) {
	if n.Fields == nil || n.Fields.NumFields() < 1 {
		// skip empty structs
		return
	}

	if field.Tag == nil {
		// skip when no struct tag
		return
	}

	fieldName, err := getFieldName(field)
	if err != nil {
		pass.Reportf(n.Pos(), "unable to get field name: %v", err)
		return
	}
	fieldTypes := getFieldTypes(field.Type)
	if len(fieldTypes) < 1 {
		bytes, _ := json.Marshal(field.Type)
		errString := fmt.Errorf("unexpected eror: type %T: %s", field.Type, string(bytes))
		pass.Reportf(n.Pos(), "unable to get field type: %v", errString)
		return
	}

	for key, convName := range config.Rules {
		if convName == "" {
			continue
		}
		baseLint(pass, config, n, field.Tag, fieldName, key, convName)
		//gin.Lint(pass, field.Tag, fieldTypes, key, convName)
	}
}

func baseLint(pass *analysis.Pass, config Config, n *ast.StructType, tag *ast.BasicLit, fieldName, key, convName string) {
	value, ok := lookupTagValue(tag, key)
	if !ok {
		// skip when no struct tag for the key
		return
	}

	if value == "-" {
		// skip when skipped :)
		return
	}

	if value == "" {
		// skip empty value, it can change in the future
		return
	}

	converter, err := getConverter(convName)
	if err != nil {
		pass.Reportf(n.Pos(), "%s(%s): %v", key, convName, err)
		return
	}

	expected := value
	if config.UseFieldName {
		expected = fieldName
	}

	if value != converter(expected) {
		pass.Reportf(tag.Pos(), "%s(%s): got '%s' want '%s'", key, convName, value, converter(expected))
	}
}

func getFieldName(field *ast.Field) (string, error) {
	var name string
	for _, n := range field.Names {
		if n.Name != "" {
			name = n.Name
		}
	}

	if name != "" {
		return name, nil
	}

	return getTypeName(field.Type)
}

func getTypeName(exp ast.Expr) (string, error) {
	switch typ := exp.(type) {
	case *ast.Ident:
		return typ.Name, nil
	case *ast.StarExpr:
		return getTypeName(typ.X)
	case *ast.SelectorExpr:
		return getTypeName(typ.Sel)
	default:
		bytes, _ := json.Marshal(exp)
		return "", fmt.Errorf("unexpected eror: type %T: %s", typ, string(bytes))
	}
}

func getFieldTypes(exp ast.Expr) []filedtype.FiledType {
	switch typ := exp.(type) {
	case *ast.Ident:
		return []filedtype.FiledType{filedtype.Parse(typ.Name)}
	case *ast.StarExpr:
		return append([]filedtype.FiledType{filedtype.Ptr}, getFieldTypes(typ.X)...)
	case *ast.ArrayType:
		return append([]filedtype.FiledType{filedtype.Array}, getFieldTypes(typ.Elt)...)
	case *ast.SliceExpr:
		return append([]filedtype.FiledType{filedtype.Slice}, getFieldTypes(typ.X)...)
	case *ast.MapType:
		return append([]filedtype.FiledType{filedtype.Map}, getFieldTypes(typ.Value)...)
	case *ast.SelectorExpr:
		return getFieldTypes(typ.X)
	default:
		return nil
	}
}

func lookupTagValue(tag *ast.BasicLit, key string) (string, bool) {
	raw := strings.Trim(tag.Value, "`")

	value, ok := reflect.StructTag(raw).Lookup(key)
	if !ok {
		return value, ok
	}

	values := strings.Split(value, ",")

	if len(values) < 1 {
		return "", true
	}

	return values[0], true
}

func getConverter(c string) (func(s string) string, error) {
	switch c {
	case "camel":
		return strcase.ToCamel, nil
	case "pascal":
		return strcase.ToPascal, nil
	case "kebab":
		return strcase.ToKebab, nil
	case "snake":
		return strcase.ToSnake, nil
	case "goCamel":
		return strcase.ToGoCamel, nil
	case "goPascal":
		return strcase.ToGoPascal, nil
	case "goKebab":
		return strcase.ToGoKebab, nil
	case "goSnake":
		return strcase.ToGoSnake, nil
	case "upper":
		return strings.ToUpper, nil
	case "lower":
		return strings.ToLower, nil
	default:
		return nil, fmt.Errorf("unsupported case: %s", c)
	}
}
