package i18n

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestParseTellsChineseFromEverythingElse(t *testing.T) {
	for tag, want := range map[string]Lang{
		"zh": ZH, "zh-CN": ZH, "zh-Hans": ZH, "ZH-tw": ZH, "zh_CN": ZH,
		"en": EN, "en-US": EN, "": EN, "fr": EN, "zhx": EN,
	} {
		if got := Parse(tag); got != want {
			t.Errorf("Parse(%q) = %q, want %q", tag, got, want)
		}
	}
}

func TestMessagesAreTranslatedWithTheirArguments(t *testing.T) {
	restore := withCatalog(t, map[string]string{
		"profile %s uses %d folders": "配置文件 %s 使用 %d 个目录",
		"cannot read %s: %w":         "无法读取 %s：%w",
		"the folder is missing":      "目录不存在",
	})
	defer restore()

	if got := T(ZH, "profile %s uses %d folders", "work", 2); got != "配置文件 work 使用 2 个目录" {
		t.Errorf("T(zh) = %q", got)
	}
	if got := T(EN, "profile %s uses %d folders", "work", 2); got != "profile work uses 2 folders" {
		t.Errorf("T(en) = %q", got)
	}
	if got := T(ZH, "no translation for %s", "x"); got != "no translation for x" {
		t.Errorf("a message without a translation stays English: %q", got)
	}

	inner := Errorf("the folder is missing")
	err := Errorf("cannot read %s: %w", "~/work", inner)
	if err.Error() != "cannot read ~/work: the folder is missing" {
		t.Errorf("Error() = %q", err.Error())
	}
	if got := Text(ZH, err); got != "无法读取 ~/work：目录不存在" {
		t.Errorf("Text(zh) = %q, want the error and the one it wraps translated", got)
	}
	if !errors.Is(err, inner) {
		t.Error("Errorf with %w should wrap like fmt.Errorf")
	}
	if got := Text(ZH, fmt.Errorf("context: %w", inner)); got != "context: the folder is missing" {
		t.Errorf("a fmt.Errorf wrapper adds English text and stays as it is: %q", got)
	}
	if got := Text(ZH, fmt.Errorf("%w", inner)); got != "目录不存在" {
		t.Errorf("a fmt.Errorf wrapper that adds nothing shows the translation: %q", got)
	}
	if got := Text(ZH, M("profile %s uses %d folders", "a", 1)); got != "配置文件 a 使用 1 个目录" {
		t.Errorf("Text(Message) = %q", got)
	}
	if got := Text(ZH, 42); got != "42" {
		t.Errorf("Text(42) = %q", got)
	}
}

func TestRegisteringATranslationTwiceIsAMistake(t *testing.T) {
	restore := withCatalog(t, map[string]string{"once": "一次"})
	defer restore()
	defer func() {
		if recover() == nil {
			t.Error("registering the same message twice should panic")
		}
	}()
	register(map[string]string{"once": "又一次"})
}

func withCatalog(t *testing.T, entries map[string]string) func() {
	t.Helper()
	saved := catalogs[ZH]
	catalogs[ZH] = map[string]string{}
	register(entries)
	return func() { catalogs[ZH] = saved }
}

// translated are the calls whose format argument is a message to translate,
// and the index of that argument.
var translated = map[string]int{
	"i18n.T": 1, "i18n.M": 0, "i18n.Errorf": 0, // the package's functions
	".tr": 0, ".trErr": 0, // the app's helpers on Env
}

type found struct {
	format string
	where  string
}

// messagesInSource finds every message the code passes to be translated.
func messagesInSource(t *testing.T) []found {
	t.Helper()
	var out []found
	fset := token.NewFileSet()
	for _, root := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			if strings.Contains(filepath.ToSlash(path), "/internal/i18n/") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				name := "." + sel.Sel.Name
				if x, ok := sel.X.(*ast.Ident); ok && x.Name == "i18n" {
					name = "i18n." + sel.Sel.Name
				}
				i, ok := translated[name]
				if !ok || len(call.Args) <= i {
					return true
				}
				where := fset.Position(call.Pos()).String()
				if id, ok := call.Args[i].(*ast.Ident); ok && id.Name == "format" {
					return true // a helper passing its own format on (Env.tr)
				}
				format, ok := literal(call.Args[i])
				if !ok {
					t.Errorf("%s: %s needs a string literal as its message, so it can be translated", where, name)
					return true
				}
				out = append(out, found{format, where})
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// literal is the value of a string literal, or of literals joined with +.
func literal(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(v.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", false
		}
		left, lok := literal(v.X)
		right, rok := literal(v.Y)
		return left + right, lok && rok
	case *ast.ParenExpr:
		return literal(v.X)
	}
	return "", false
}

var verb = regexp.MustCompile(`%[-+# 0]*(?:\[\d+\])?(?:\d+|\*)?(?:\.(?:\d+|\*))?[a-zA-Z%]`)

// verbs are a format's placeholders in order; %w counts as %v.
func verbs(format string) []string {
	var out []string
	for _, v := range verb.FindAllString(format, -1) {
		if v == "%%" {
			continue
		}
		out = append(out, strings.Replace(v, "w", "v", 1))
	}
	return out
}

// Every message the code shows has a Chinese translation with the same
// placeholders in the same order, and no translation is left over.
func TestEveryMessageHasAChineseTranslation(t *testing.T) {
	used := map[string]bool{}
	var missing []string
	for _, f := range messagesInSource(t) {
		used[f.format] = true
		zh, ok := catalogs[ZH][f.format]
		if !ok {
			missing = append(missing, fmt.Sprintf("%s: %q", f.where, f.format))
			continue
		}
		if !slices.Equal(verbs(f.format), verbs(zh)) {
			t.Errorf("%s: the translation of %q has placeholders %v, want %v", f.where, f.format, verbs(zh), verbs(f.format))
		}
	}
	sort.Strings(missing)
	for _, m := range missing {
		t.Errorf("no Chinese translation: %s", m)
	}
	var unused []string
	for format := range catalogs[ZH] {
		if !used[format] {
			unused = append(unused, format)
		}
	}
	sort.Strings(unused)
	for _, u := range unused {
		t.Errorf("a Chinese translation no code uses: %q", u)
	}
}
