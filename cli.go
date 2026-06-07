package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

type app struct {
	stdout io.Writer
	stderr io.Writer
	client *http.Client
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	loadedDotenv := loadDotenv(".env")
	a := app{stdout: stdout, stderr: stderr, client: http.DefaultClient}
	return a.run(ctx, args, loadedDotenv)
}

func (a app) run(ctx context.Context, args []string, loadedDotenv map[string]bool) int {
	if len(args) == 0 {
		printUsage(a.stdout)
		return exitOK
	}
	if args[0] == "--version" || args[0] == "-version" {
		fmt.Fprintln(a.stdout, version)
		return exitOK
	}

	switch args[0] {
	case "version":
		format, rest, err := parseFormat(args[1:])
		if err != nil {
			return a.writeCLIError(err, "json")
		}
		if len(rest) != 0 {
			return a.usageError("version takes no positional arguments", format)
		}
		return a.cmdVersion(format)
	case "config":
		return a.cmdConfig(args[1:], loadedDotenv)
	case "auth":
		return a.cmdAuth(ctx, args[1:], loadedDotenv)
	case "doctor":
		return a.cmdDoctor(ctx, args[1:], loadedDotenv)
	case "resolve":
		return a.cmdResolve(ctx, args[1:], loadedDotenv)
	case "help", "--help", "-h":
		printUsage(a.stdout)
		return exitOK
	default:
		return a.usageError("unknown command: "+args[0], "text")
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "plane-cli is a Plane.so CLI.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  plane-cli --version")
	fmt.Fprintln(w, "  plane-cli version [--format text|json]")
	fmt.Fprintln(w, "  plane-cli config get [--format text|json]")
	fmt.Fprintln(w, "  plane-cli config set <base_url|workspace_slug> <value> [--format text|json]")
	fmt.Fprintln(w, "  plane-cli auth status [--format text|json]")
	fmt.Fprintln(w, "  plane-cli doctor [--for-agent] [--format text|json]")
	fmt.Fprintln(w, "  plane-cli resolve <PROJECT-123> [--format text|json] [--no-cache]")
}

func parseFormat(args []string) (string, []string, *cliError) {
	format := "text"
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--format":
			if i+1 >= len(args) {
				return format, out, newError("UNSUPPORTED_FORMAT", "--format requires a value.", "Use --format text or --format json.", false, "plane-cli version --format json")
			}
			format = args[i+1]
			i++
		case strings.HasPrefix(arg, "--format="):
			format = strings.TrimPrefix(arg, "--format=")
		default:
			out = append(out, arg)
		}
	}
	if format != "text" && format != "json" {
		return format, out, newError("UNSUPPORTED_FORMAT", "Unsupported output format: "+format, "Use --format text or --format json.", false, "plane-cli version --format json")
	}
	return format, out, nil
}

func hasFlag(args []string, name string) ([]string, bool) {
	out := make([]string, 0, len(args))
	found := false
	for _, arg := range args {
		if arg == name {
			found = true
			continue
		}
		out = append(out, arg)
	}
	return out, found
}

func (a app) usageError(message, format string) int {
	return a.writeCLIError(newError("USAGE_ERROR", message, "Run plane-cli help for usage.", false, "plane-cli help"), format)
}

func (a app) writeCLIError(err *cliError, format string) int {
	if format == "json" {
		writeJSON(a.stdout, errorEnvelope(err))
	} else {
		fmt.Fprintf(a.stderr, "%s: %s\n", err.Code, err.Message)
		if err.Fix != "" {
			fmt.Fprintf(a.stderr, "fix: %s\n", err.Fix)
		}
	}
	if err.Code == "USAGE_ERROR" || err.Code == "UNSUPPORTED_FORMAT" {
		return exitUsage
	}
	return exitError
}

func getenv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", false
	}
	return value, true
}
