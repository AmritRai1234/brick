// Command brick compiles Brick source to a native executable.
//
// Two backends are available:
//
//	--target c       emit C, let gcc/clang optimize and assemble it
//	--target native  emit x86-64 assembly directly (Brick's own backend)
//	--target auto    try native, fall back to C when a feature is unsupported
//
// Usage:
//
//	brick run   <file.brick> [args...]
//	brick build <file.brick> [-o out] [--target c|native|auto]
//	brick emit  <file.brick> [--target c|native]
//	brick asm   <file.brick>              print the native assembly
//	brick check <file.brick>
//	brick tokens <file.brick>
//	brick ast   <file.brick>
//	brick version
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"brick/internal/ast"
	"brick/internal/checker"
	"brick/internal/codegen"
	"brick/internal/lexer"
	"brick/internal/native"
	"brick/internal/parser"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	rest := os.Args[2:]

	switch cmd {
	case "version", "-v", "--version":
		fmt.Printf("brick %s\n", version)
	case "help", "-h", "--help":
		usage()
	case "tokens":
		_, _, file, _ := parseArgs(rest)
		src := readFile(file)
		for _, t := range lexer.New(src).Tokens() {
			fmt.Printf("%3d:%-3d %-9s %q\n", t.Line, t.Col, t.Type.String(), t.Literal)
		}
	case "ast":
		_, _, file, _ := parseArgs(rest)
		fmt.Print(ast.Dump(mustParse(readFile(file))))
	case "check":
		_, _, file, _ := parseArgs(rest)
		mustCheck(mustParse(readFile(file)))
		fmt.Printf("%s: ok\n", file)
	case "asm":
		_, _, file, _ := parseArgs(rest)
		prog := mustParse(readFile(file))
		ck := mustCheck(prog)
		s, err := native.Generate(prog, ck)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		fmt.Print(s)
	case "emit":
		target, _, file, _ := parseArgs(rest)
		if target == "" || target == "auto" {
			target = "c"
		}
		kind, src := lower(file, target, false)
		_ = kind
		fmt.Print(src)
	case "run":
		target, _, file, progArgs := parseArgs(rest)
		kind, src := lower(file, orAuto(target), true)
		bin := tempPath("brick-bin-*")
		defer os.Remove(bin)
		linkBinary(kind, src, bin)
		execBinary(bin, progArgs)
	case "build":
		target, out, file, _ := parseArgs(rest)
		if out == "" {
			out = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		kind, src := lower(file, orAuto(target), true)
		linkBinary(kind, src, out)
		fmt.Printf("built %s (%s backend)\n", out, kind)
	default:
		fmt.Fprintf(os.Stderr, "brick: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

func orAuto(t string) string {
	if t == "" {
		return "c" // the C backend is the default, and the supported path
	}
	return t
}

func usage() {
	fmt.Print(`brick - a small, fast, readable systems language

Usage:
  brick run   <file.brick> [args...]   compile, link, and run
  brick build <file.brick> [-o out]    compile to a native executable
  brick emit  <file.brick>             print the generated C
  brick asm   <file.brick>             print the native x86-64 assembly
  brick check <file.brick>             type-check only
  brick tokens <file.brick>            print the token stream
  brick ast   <file.brick>             print the parsed tree
  brick version

Options:
  --target c|native|auto   choose the backend (default: c)
  -o <name>                output name for 'build'

Backends:
  c        emit C; gcc/clang optimize and assemble it. The default, and the
           only backend that covers the whole language.
  native   experimental: emit x86-64 assembly directly. A subset only; slower
           than c until it grows an optimizer.
  auto     try native, fall back to c.

Set BRICK_CC to choose the C compiler used for assembling/linking.
`)
}

// ---------- argument parsing ----------

func parseArgs(args []string) (target, out, file string, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--target":
			i++
			if i >= len(args) {
				die("--target needs a value")
			}
			target = args[i]
		case strings.HasPrefix(a, "--target="):
			target = strings.TrimPrefix(a, "--target=")
		case a == "-o" || a == "--output":
			i++
			if i >= len(args) {
				die("-o needs a name")
			}
			out = args[i]
		case a == "--":
			rest = append(rest, args[i+1:]...)
			i = len(args)
		case strings.HasPrefix(a, "-"):
			die("unknown option " + a)
		case file == "":
			file = a
		default:
			rest = append(rest, a)
		}
	}
	if file == "" {
		die("expected a .brick file")
	}
	return
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "brick: "+msg)
	os.Exit(2)
}

// ---------- pipeline ----------

func lower(file, target string, warn bool) (kind, src string) {
	prog := mustParse(readFile(file))
	ck := mustCheck(prog)
	switch target {
	case "c":
		return "c", codegen.Generate(prog, ck)
	case "native":
		s, err := native.Generate(prog, ck)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return "native", s
	case "auto":
		s, err := native.Generate(prog, ck)
		if err == nil {
			return "native", s
		}
		if _, ok := err.(*native.UnsupportedError); ok {
			if warn {
				fmt.Fprintf(os.Stderr, "brick: %v\n", err)
				fmt.Fprintln(os.Stderr, "brick: falling back to the C backend")
			}
			return "c", codegen.Generate(prog, ck)
		}
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	die("unknown target " + target)
	return
}

// ---------- helpers ----------

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "brick: %v\n", err)
		os.Exit(1)
	}
	return string(data)
}

func mustParse(src string) *ast.Program {
	prog, err := parser.Parse(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return prog
}

func mustCheck(prog *ast.Program) *checker.Checker {
	ck, err := checker.Check(prog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return ck
}

func cc() string {
	if c := os.Getenv("BRICK_CC"); c != "" {
		return c
	}
	for _, c := range []string{"cc", "gcc", "clang"} {
		if _, err := exec.LookPath(c); err == nil {
			return c
		}
	}
	return "cc"
}

func tempPath(pattern string) string {
	f, err := os.CreateTemp("", pattern)
	if err != nil {
		fmt.Fprintf(os.Stderr, "brick: %v\n", err)
		os.Exit(1)
	}
	p := f.Name()
	f.Close()
	os.Remove(p) // let the tool create it
	return p
}

func writeFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "brick: %v\n", err)
		os.Exit(1)
	}
}

// linkBinary produces an executable at out from either C or assembly source.
func linkBinary(kind, src, out string) {
	switch kind {
	case "c":
		cfile := tempPath("brick-*.c")
		defer os.Remove(cfile)
		writeFile(cfile, src)
		run(cc(), "-std=c11", "-O2", "-Wall", "-Wno-unused-parameter", cfile, "-o", out)
	case "native":
		asmfile := tempPath("brick-*.s")
		defer os.Remove(asmfile)
		objfile := tempPath("brick-*.o")
		defer os.Remove(objfile)
		writeFile(asmfile, src)
		run("as", asmfile, "-o", objfile)
		run(cc(), objfile, "-o", out)
	default:
		die("unknown backend " + kind)
	}
}

func execBinary(bin string, args []string) {
	cmd := exec.Command(bin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "brick: %v\n", err)
		os.Exit(1)
	}
}

func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "brick: %s failed (exit %d)\n", name, ee.ExitCode())
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "brick: %v\n", err)
		os.Exit(1)
	}
}
