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

// Keys finished at run time: the literal is the part before the value, and each value
// that can follow it is listed here. A new prefix fails the test until it is added.
var dynamicTranslationKeys = map[string][]string{
	"plugins.problem.": {"kimi", "hermes", "devin"}, // automationProblem's Agents
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

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
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
				values, known := dynamicTranslationKeys[key]
				if !known {
					t.Errorf("%s: %q is completed at run time; list its values in dynamicTranslationKeys", fset.Position(lit.Pos()), key)
					return true
				}
				keys = nil
				for _, v := range values {
					keys = append(keys, key+v)
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
