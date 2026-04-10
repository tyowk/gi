package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tyowk/gi/gipack"
	"github.com/tyowk/gi/interpreter"
	"github.com/tyowk/gi/lexer"
	"github.com/tyowk/gi/parser"
	"github.com/tyowk/gi/sema"
)

const version = "1.0.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		requireArg("run", "<file.gi>")
		runFile(os.Args[2])
	case "check":
		requireArg("check", "<file.gi>")
		checkFile(os.Args[2])
	case "tokens":
		requireArg("tokens", "<file.gi>")
		printTokens(os.Args[2])
	case "pack":
		packCmd(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("gi version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		err := gipack.RunScript(os.Args[1], os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	}
}

func requireArg(cmd, usage string) {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: gi %s %s\n", cmd, usage)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`GI Programming Language v` + version + `

Usage:
  gi run <file.gi>              Interpret and run a .gi source file
  gi pack <subcommand>          Package manager (gipack)
  gi version                    Print version
  gi help                       Print this help
  gi <script> [args]            Run a script from gipack.json

Pack subcommands:
  gi pack init                  Create gipack.json in current directory
  gi pack install               Install all dependencies from gipack.json
  gi pack install <user/repo>   Install a package from GitHub
  gi pack add <user/repo>       Add a dependency (same as install)
  gi pack add --dev <user/repo> Add a dev dependency
  gi pack remove <name>         Uninstall a package
  gi pack update [name]         Pull latest commits (all or one)
  gi pack list                  List installed packages
  gi pack info <name>           Show package details
  gi pack run <script> [args]   Run a script from gipack.json
`)
}

func checkFile(path string) {
	src, err := readGi(path)
	if err != nil {
		die(err.Error())
	}

	lx := lexer.New(src)
	p := parser.New(lx.Tokenize())
	prog := p.Parse()

	hasErrors := false
	if len(p.Errors()) > 0 {
		hasErrors = true
		fmt.Fprintln(os.Stderr, "parse errors:")
		for _, e := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s:%s\n", path, e)
		}
	}

	analyzer := sema.New()
	analyzer.Analyze(prog)
	if analyzer.HasErrors() {
		hasErrors = true
		fmt.Fprintln(os.Stderr, "semantic errors:")
		for _, e := range analyzer.ErrorStrings() {
			fmt.Fprintf(os.Stderr, "  %s:%s\n", path, e)
		}
	}

	if hasErrors {
		os.Exit(1)
	}
	fmt.Printf("✓ %s — no errors\n", path)
}

func packCmd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gi pack <subcommand>")
		fmt.Fprintln(os.Stderr, "run 'gi help' for available subcommands")
		os.Exit(1)
	}

	sub := args[0]
	rest := args[1:]

	var err error
	switch sub {
	case "init":
		err = gipack.Init()
	case "install":
		spec := ""
		if len(rest) > 0 {
			spec = rest[0]
		}
		err = gipack.Install(spec)
	case "add":
		if len(rest) == 0 {
			die("usage: gi pack add [--dev] <user/repo>")
		}
		dev := false
		spec := ""
		for _, arg := range rest {
			if arg == "--dev" || arg == "-D" {
				dev = true
				continue
			}
			if spec == "" {
				spec = arg
			}
		}
		if spec == "" {
			die("usage: gi pack add [--dev] <user/repo>")
		}
		err = gipack.Add(spec, dev)
	case "remove", "uninstall", "rm":
		if len(rest) == 0 {
			die("usage: gi pack remove <name>")
		}
		err = gipack.Remove(rest[0])
	case "update", "upgrade":
		name := ""
		if len(rest) > 0 {
			name = rest[0]
		}
		err = gipack.Update(name)
	case "list", "ls":
		err = gipack.List()
	case "info", "show":
		if len(rest) == 0 {
			die("usage: gi pack info <name>")
		}
		err = gipack.Info(rest[0])
	case "run":
		if len(rest) == 0 {
			die("usage: gi pack run <script> [args]")
		}
		err = gipack.RunScript(rest[0], rest[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown pack subcommand: %q\n", sub)
		fmt.Fprintln(os.Stderr, "run 'gi help' for available subcommands")
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runFile(path string) {
	src, err := readGi(path)
	if err != nil {
		die(err.Error())
	}

	lx := lexer.New(src)
	p := parser.New(lx.Tokenize())
	prog := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "parse errors:")
		for _, e := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s:%s\n", path, e)
		}
		os.Exit(1)
	}

	analyzer := sema.New()
	analyzer.Analyze(prog)
	if analyzer.HasErrors() {
		fmt.Fprintln(os.Stderr, "semantic errors:")
		for _, e := range analyzer.ErrorStrings() {
			fmt.Fprintf(os.Stderr, "  %s:%s\n", path, e)
		}
		os.Exit(1)
	}

	baseDir, _ := filepath.Abs(filepath.Dir(path))
	interp := interpreter.New(baseDir)
	if err := interp.Run(prog); err != nil {
		fmt.Fprintln(os.Stderr, "runtime error:", err)
		os.Exit(1)
	}
}

func printTokens(path string) {
	src, err := readGi(path)
	if err != nil {
		die(err.Error())
	}
	for _, t := range lexer.New(src).Tokenize() {
		fmt.Printf("%-12s  %-20q  (line %d, col %d)\n", t.Type, t.Literal, t.Line, t.Col)
	}
}

func readGi(path string) (string, error) {
	if !strings.HasSuffix(path, ".gi") {
		return "", fmt.Errorf("file must have .gi extension: %q", path)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read %q: %v", path, err)
	}
	return string(src), nil
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "error:", msg)
	os.Exit(1)
}
