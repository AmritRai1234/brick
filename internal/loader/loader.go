// Package loader resolves `import` statements and merges a program and its
// dependencies into a single AST.
//
// Brick has no separate compilation: an import pulls another file's top-level
// declarations (functions, structs, enums) into the same program, and the
// compiler then treats it as one unit. Each file is loaded once, no matter how
// many times it is imported, and import cycles are reported as an error.
package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"brick/internal/ast"
	"brick/internal/parser"
)

// Load reads path, resolves its imports (depth-first) and returns the merged
// program. Imported declarations come before the importing file's own.
func Load(path string) (*ast.Program, error) {
	l := &loader{
		done:    map[string]bool{},
		loading: map[string]bool{},
		prog:    &ast.Program{},
	}
	if err := l.load(path); err != nil {
		return nil, err
	}
	return l.prog, nil
}

type loader struct {
	done    map[string]bool // absolute paths already merged
	loading map[string]bool // absolute paths on the current import chain
	prog    *ast.Program
}

func (l *loader) load(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if l.done[abs] {
		return nil
	}
	if l.loading[abs] {
		return fmt.Errorf("%s: import cycle", path)
	}
	l.loading[abs] = true
	defer delete(l.loading, abs)

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	prog, err := parser.Parse(string(data))
	if err != nil {
		return fmt.Errorf("%s: %v", path, err)
	}

	// Resolve imports first, so a file's dependencies are merged before it.
	dir := filepath.Dir(path)
	for _, s := range prog.Stmts {
		im, ok := s.(*ast.ImportStmt)
		if !ok {
			continue
		}
		if err := l.load(filepath.Join(dir, importFile(im.Path))); err != nil {
			return err
		}
	}

	// Merge this file's declarations, dropping the import statements.
	for _, s := range prog.Stmts {
		if _, isImport := s.(*ast.ImportStmt); isImport {
			continue
		}
		l.prog.Stmts = append(l.prog.Stmts, s)
	}
	l.done[abs] = true
	return nil
}

// importFile turns an import path into a filename: `import "util"` and
// `import "util.brick"` both mean util.brick next to the importing file.
func importFile(path string) string {
	if strings.HasSuffix(path, ".brick") {
		return path
	}
	return path + ".brick"
}
