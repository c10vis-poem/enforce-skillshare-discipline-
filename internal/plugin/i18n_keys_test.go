package plugin

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Keys finished at run time: the literal is the part before the value, and the values are
// the cases of the function named here. A new prefix fails the test until it is added.
var dynamicTranslationKeys = map[string]string{
	"plugins.problem.": "automationProblem",
}

// switchCases lists the string cases of the switch in function name.
func switchCases(files []*ast.File, name string) []string {
	var cases []string
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != name {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				if c, ok := n.(*ast.CaseClause); ok {
					for _, e := range c.List {
						if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
							if v, err := strconv.Unquote(lit.Value); err == nil {
								cases = append(cases, v)
							}
						}
					}
				}
				return true
			})
		}
	}
	return cases
}

// The dashboard translates the messageKey, errorKey, noteKey, reasonKey and problemKey this
// package sends. Every one is a "plugins." string literal, so each must be in en.json, the
// file every other locale is held to; a key missing there shows as raw English or as the key.
func TestTranslationKeysExistInDashboard(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "ui", "src", "i18n", "locales", "en.json"))
	if err != nil {
		t.Fatal(err)
	}
	var en map[string]string
	if err := json.Unmarshal(raw, &en); err != nil {
		t.Fatal(err)
	}

	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, file)
	}

	checked := 0
	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(lit.Value)
			if err != nil || !strings.HasPrefix(key, "plugins.") {
				return true
			}
			keys := []string{key}
			if strings.HasSuffix(key, ".") {
				fn, known := dynamicTranslationKeys[key]
				if !known {
					t.Errorf("%s: %q is completed at run time; name the function that lists its values in dynamicTranslationKeys", fset.Position(lit.Pos()), key)
					return true
				}
				keys = nil
				for _, v := range switchCases(files, fn) {
					keys = append(keys, key+v)
				}
				if len(keys) == 0 {
					t.Errorf("%s: found no cases in %s to complete %q", fset.Position(lit.Pos()), fn, key)
				}
			}
			for _, k := range keys {
				checked++
				if _, ok := en[k]; !ok {
					t.Errorf("%s: translation key %q is not in ui/src/i18n/locales/en.json", fset.Position(lit.Pos()), k)
				}
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("found no translation keys; the check no longer sees them")
	}
}
