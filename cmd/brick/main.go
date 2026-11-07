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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"brick/internal/ast"
	"brick/internal/checker"
	"brick/internal/codegen"
	"brick/internal/format"
	"brick/internal/interp"
	"brick/internal/lexer"
	"brick/internal/loader"
	"brick/internal/lsp"
	"brick/internal/native"
	"brick/internal/parser"
)

const version = "0.3.0"

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
		fmt.Print(ast.Dump(mustLoad(file)))
	case "check":
		_, _, file, _ := parseArgs(rest)
		mustCheck(mustLoad(file))
		fmt.Printf("%s: ok\n", file)
	case "asm":
		_, _, file, _ := parseArgs(rest)
		prog := mustLoad(file)
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
	case "lsp":
		if err := lsp.Run(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "brick: lsp: %v\n", err)
			os.Exit(1)
		}
	case "fmt", "format":
		os.Exit(fmtCmd(rest))
	case "interp":
		_, _, file, progArgs := parseArgs(rest)
		prog := mustLoad(file)
		ck := mustCheck(prog)
		if err := interp.Run(prog, ck, interp.Options{
			Args:   progArgs,
			Stdout: os.Stdout,
			Stderr: os.Stderr,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "brick: %v\n", err)
			os.Exit(1)
		}
	case "diff":
		target, _, file, progArgs := parseArgs(rest)
		os.Exit(diffRun(file, orAuto(target), progArgs))
	case "run":
		target, _, file, progArgs := parseArgs(rest)
		kind, src := lower(file, orAuto(target), true)
		bin := tempPath("brick-bin-*")
		defer os.Remove(bin)
		linkBinary(kind, src, bin, file)
		execBinary(bin, progArgs)
	case "build":
		target, out, file, _ := parseArgs(rest)
		if out == "" {
			out = strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		}
		kind, src := lower(file, orAuto(target), true)
		linkBinary(kind, src, out, file)
		fmt.Printf("built %s (%s backend)\n", out, kind)
	default:
		fmt.Fprintf(os.Stderr, "brick: unknown command %q\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

// fmtCmd implements the formatter command:
//
//	brick fmt <file>            rewrite <file> in place
//	brick fmt --stdout <file>   print the formatted source
//	brick fmt --check <file>    exit 1 if <file> is not already formatted
func fmtCmd(args []string) int {
	stdout, check := false, false
	var file string
	for _, a := range args {
		switch a {
		case "--stdout", "-":
			stdout = true
		case "--check":
			check = true
		case "--":
			// ignore
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "brick: unknown fmt option %q\n", a)
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "brick: fmt expected a .brick file")
		return 2
	}
	src := readFile(file)
	out := format.Source(src)
	switch {
	case stdout:
		fmt.Print(out)
		return 0
	case check:
		if out != src {
			fmt.Fprintf(os.Stderr, "%s: not formatted\n", file)
			return 1
		}
		fmt.Printf("%s: ok\n", file)
		return 0
	default:
		if out == src {
			fmt.Printf("%s: already formatted\n", file)
			return 0
		}
		writeFile(file, out)
		fmt.Printf("%s: reformatted\n", file)
		return 0
	}
}

// diffRun implements differential testing (§10): run the program on the
// interpreter and as a compiled binary, and compare their output. A mismatch
// means one of the two has a bug. Returns a process exit code.
func diffRun(file, target string, progArgs []string) int {
	prog := mustLoad(file)
	if programHasExtern(prog) {
		fmt.Printf("skip: %s calls extern C functions (interpreter cannot run C)\n", file)
		return 0
	}
	ck := mustCheck(prog)

	// Interpret.
	var iout, ierr bytes.Buffer
	iErr := interp.Run(prog, ck, interp.Options{Args: progArgs, Stdout: &iout, Stderr: &ierr})

	// Compile and run.
	kind, src := lower(file, target, false)
	bin := tempPath("brick-diff-*")
	defer os.Remove(bin)
	linkBinary(kind, src, bin, file)
	cmd := exec.Command(bin, progArgs...)
	var cout, cerr bytes.Buffer
	cmd.Stdout = &cout
	cmd.Stderr = &cerr
	cErr := cmd.Run()

	agree := iout.String() == cout.String()
	if agree && iErr == nil && cErr == nil {
		fmt.Printf("agree: %s (via %s backend)\n", file, kind)
		return 0
	}
	fmt.Fprintf(os.Stderr, "DISAGREE: %s\n", file)
	if iErr != nil {
		fmt.Fprintf(os.Stderr, "  interpreter error: %v\n", iErr)
	}
	if cErr != nil {
		fmt.Fprintf(os.Stderr, "  compiled error:    %v\n", cErr)
	}
	if !agree {
		fmt.Fprintf(os.Stderr, "--- interpreter stdout ---\n%s", iout.String())
		fmt.Fprintf(os.Stderr, "--- compiled stdout ----\n%s", cout.String())
	}
	return 1
}

func programHasExtern(prog *ast.Program) bool {
	for _, s := range prog.Stmts {
		if fd, ok := s.(*ast.FuncDecl); ok && fd.Extern {
			return true
		}
	}
	return false
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
  brick fmt   <file.brick>             reformat in place (--stdout, --check)
  brick lsp                            run the language server (stdio)
  brick interp <file.brick> [args...]  run on the tree-walking interpreter
  brick diff  <file.brick> [args...]   compile+run and interpret, compare output
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
	prog := mustLoad(file)
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

// mustLoad parses a file, resolves its imports and returns the merged program.
func mustLoad(file string) *ast.Program {
	prog, err := loader.Load(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
	return prog
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
// Any C files sitting next to the .brick source (sourceFile) are compiled and
// linked too, which is how a program calls into C via `extern` (see
// examples/extern.brick alongside examples/extern.c).
func linkBinary(kind, src, out, sourceFile string) {
	extra := siblingCFiles(sourceFile)
	switch kind {
	case "c":
		cfile := tempPath("brick-*.c")
		defer os.Remove(cfile)
		writeFile(cfile, src)
		args := []string{"-std=c11", "-O2", "-Wall", "-Wno-unused-parameter", cfile}
		args = append(args, extra...)
		args = append(args, "-o", out)
		run(cc(), args...)
	case "native":
		asmfile := tempPath("brick-*.s")
		defer os.Remove(asmfile)
		objfile := tempPath("brick-*.o")
		defer os.Remove(objfile)
		writeFile(asmfile, src)
		run("as", asmfile, "-o", objfile)
		args := append([]string{objfile}, extra...)
		args = append(args, "-o", out)
		run(cc(), args...)
	default:
		die("unknown backend " + kind)
	}
}

// siblingCFiles returns the .c files in the same directory as the Brick source,
// so `extern` functions they define resolve at link time.
func siblingCFiles(sourceFile string) []string {
	dir := filepath.Dir(sourceFile)
	matches, err := filepath.Glob(filepath.Join(dir, "*.c"))
	if err != nil {
		return nil
	}
	return matches
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
