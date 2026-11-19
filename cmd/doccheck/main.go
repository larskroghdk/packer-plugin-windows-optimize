// Command doccheck enforces the Go part of INVARIANTS.md I19.
//
// Every function and method needs a doc comment that starts with its name and
// has "Author: Lars Krogh" and "Version: x.y.z" lines, plus a "Returns:"
// section when it returns an error or more than one value. Exported types and
// non-test packages need doc comments. Any other comment is rejected, except
// //go: directives. Generated files are skipped.
//
// Usage:
//
//	go run ./cmd/doccheck [dir ...]
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	authorLine  = regexp.MustCompile(`(?m)^Author: Lars Krogh$`)
	versionLine = regexp.MustCompile(`(?m)^Version: \d+\.\d+\.\d+$`)
	returnsLine = regexp.MustCompile(`(?m)^Returns:$`)
)

// main checks the directories given as arguments, or the current directory,
// and prints every problem. It exits with status 1 when there are problems
// and 2 when a file cannot be read or parsed.
//
// Author: Lars Krogh
// Version: 1.0.0
func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = []string{"."}
	}
	problems, err := check(roots)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if len(problems) > 0 {
		fmt.Println("Go comments must follow INVARIANTS.md I19:")
		for _, p := range problems {
			fmt.Println(p)
		}
		os.Exit(1)
	}
}

// check walks roots and checks every non-generated Go file, then reports each
// non-test package that has no package comment in any of its files.
//
// Returns:
//   - []string: The problems, sorted, as "position: message".
//   - error: A directory cannot be walked or a file cannot be parsed.
//
// Author: Lars Krogh
// Version: 1.0.0
func check(roots []string) ([]string, error) {
	fset := token.NewFileSet()
	var problems []string
	type pkg struct{ dir, name string }
	documented := map[pkg]bool{}
	packages := map[pkg]string{}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
			if err != nil {
				return err
			}
			if ast.IsGenerated(file) {
				return nil
			}
			key := pkg{filepath.Dir(path), file.Name.Name}
			if _, ok := packages[key]; !ok {
				packages[key] = path
			}
			if file.Doc != nil {
				documented[key] = true
			}
			problems = append(problems, checkFile(fset, file)...)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	for key, path := range packages {
		if !documented[key] && !strings.HasSuffix(key.name, "_test") {
			problems = append(problems, fmt.Sprintf("%s: package %s has no package comment", filepath.Dir(path), key.name))
		}
	}
	sort.Strings(problems)
	return problems, nil
}

// checkFile checks the function, method and exported type doc comments of
// file, and reports every comment that is neither a doc comment nor a //go:
// directive.
//
// Author: Lars Krogh
// Version: 1.0.0
func checkFile(fset *token.FileSet, file *ast.File) []string {
	var problems []string
	report := func(pos token.Pos, format string, args ...any) {
		problems = append(problems, fset.Position(pos).String()+": "+fmt.Sprintf(format, args...))
	}
	allowed := map[*ast.CommentGroup]bool{}
	if file.Doc != nil {
		allowed[file.Doc] = true
	}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			name := funcName(d)
			if d.Doc == nil {
				report(d.Pos(), "%s has no doc comment", name)
				continue
			}
			allowed[d.Doc] = true
			text := d.Doc.Text()
			if !strings.HasPrefix(text, d.Name.Name+" ") {
				report(d.Doc.Pos(), "doc comment of %s must start with %q", name, d.Name.Name)
			}
			if !authorLine.MatchString(text) || !versionLine.MatchString(text) {
				report(d.Doc.Pos(), "doc comment of %s needs \"Author: Lars Krogh\" and \"Version: x.y.z\" lines", name)
			}
			if needsReturns(d.Type) && !returnsLine.MatchString(text) {
				report(d.Doc.Pos(), "doc comment of %s needs a \"Returns:\" section", name)
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				allowed[d.Doc] = true
			}
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts := spec.(*ast.TypeSpec)
				if !ts.Name.IsExported() {
					continue
				}
				doc := ts.Doc
				if doc == nil && len(d.Specs) == 1 {
					doc = d.Doc
				}
				if doc == nil {
					report(ts.Pos(), "exported type %s has no doc comment", ts.Name.Name)
					continue
				}
				allowed[doc] = true
				if !strings.HasPrefix(doc.Text(), ts.Name.Name+" ") && !strings.HasPrefix(doc.Text(), "A "+ts.Name.Name+" ") && !strings.HasPrefix(doc.Text(), "An "+ts.Name.Name+" ") {
					report(doc.Pos(), "doc comment of type %s must start with its name", ts.Name.Name)
				}
			}
		}
	}
	for _, group := range file.Comments {
		if allowed[group] || isDirectiveOnly(group) {
			continue
		}
		report(group.Pos(), "comment is not a doc comment: %s", strings.TrimSpace(group.List[0].Text))
	}
	return problems
}

// funcName returns the name used in reports: Name for functions and
// Receiver.Name for methods.
//
// Author: Lars Krogh
// Version: 1.0.0
func funcName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return d.Name.Name
	}
	recv := d.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name + "." + d.Name.Name
	}
	return d.Name.Name
}

// isDirectiveOnly reports whether every line of the comment group is a
// //go: directive, such as //go:embed or //go:build.
//
// Author: Lars Krogh
// Version: 1.0.0
func isDirectiveOnly(group *ast.CommentGroup) bool {
	for _, c := range group.List {
		if !strings.HasPrefix(c.Text, "//go:") {
			return false
		}
	}
	return true
}

// needsReturns reports whether a function returns an error or more than one
// value, which requires a "Returns:" section in its doc comment.
//
// Author: Lars Krogh
// Version: 1.0.0
func needsReturns(t *ast.FuncType) bool {
	if t.Results == nil {
		return false
	}
	count := 0
	for _, field := range t.Results.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		count += n
		if ident, ok := field.Type.(*ast.Ident); ok && ident.Name == "error" {
			return true
		}
	}
	return count > 1
}
