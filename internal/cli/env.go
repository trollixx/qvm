package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/storage"
)

func (a *app) newEnvCommand() *cli.Command {
	return &cli.Command{
		Name:            "env",
		Usage:           "Print shell commands to activate a Qt version in the current shell",
		ArgsUsage:       "<version>",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
			&cli.StringFlag{
				Name:  "shell",
				Usage: "output format: powershell|pwsh|cmd|bash|sh|zsh|fish|nu (default: powershell on Windows, bash elsewhere)",
			},
		},
		Action: a.runEnv,
	}
}

func (a *app) runEnv(_ context.Context, cmd *cli.Command) error {
	arg := cmd.Args().Get(0)
	if arg == "" {
		return newHintError("missing argument",
			"Usage:\n"+
				"  qvm env <version> [--shell <fmt>]   Print environment exports\n\n"+
				"Examples:\n"+
				`  eval "$(qvm env 6.8.3 --shell bash)"`+"\n"+
				"  qvm env 6.8.3 --shell powershell | Out-String | Invoke-Expression")
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	return a.envQt(reg, arg, cmd.String("arch"), cmd.String("shell"))
}

func (a *app) envQt(reg *storage.Registry, version, arch, shell string) error {
	qt, err := resolveInstalledQt(reg, version, arch, "env")
	if err != nil {
		return err
	}
	env := buildQtEnv(qt.InstallDir, os.Getenv)
	return writeEnvScript(a.streams.Out, resolveShell(shell), env)
}

// resolveShell returns shell if non-empty, otherwise the platform default.
func resolveShell(shell string) string {
	if shell != "" {
		return shell
	}
	if runtime.GOOS == "windows" {
		return "powershell"
	}
	return "bash"
}

// buildQtEnv computes the environment changes that activate the Qt install at
// prefix: the prefix is prepended to CMAKE_PREFIX_PATH and <prefix>/bin to PATH.
// Existing values (read via getenv) are preserved, not clobbered.
func buildQtEnv(prefix string, getenv func(string) string) map[string]string {
	return map[string]string{
		"CMAKE_PREFIX_PATH": prependPathList(getenv("CMAKE_PREFIX_PATH"), prefix),
		"PATH":              prependPathList(getenv("PATH"), filepath.Join(prefix, "bin")),
	}
}

// prependPathList prepends entry to a path-list value using the platform
// separator, avoiding a dangling separator when the existing value is empty.
func prependPathList(existing, entry string) string {
	if existing == "" {
		return entry
	}
	return entry + string(os.PathListSeparator) + existing
}

// writeEnvScript renders env as shell statements for the requested shell.
func writeEnvScript(w io.Writer, shell string, env map[string]string) error {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	switch strings.ToLower(shell) {
	case "powershell", "pwsh":
		for _, k := range keys {
			fmt.Fprintf(w, "$env:%s = %s\n", k, psQuote(env[k]))
		}
	case "cmd", "bat":
		for _, k := range keys {
			fmt.Fprintf(w, "set %s=%s\n", k, env[k])
		}
	case "bash", "sh", "zsh":
		for _, k := range keys {
			fmt.Fprintf(w, "export %s=%s\n", k, shQuote(env[k]))
		}
	case "fish":
		for _, k := range keys {
			fmt.Fprintf(w, "set -gx %s %s\n", k, shQuote(env[k]))
		}
	case "nu", "nushell":
		return json.NewEncoder(w).Encode(env)
	default:
		return newHintError(
			fmt.Sprintf("unsupported shell %q", shell),
			"Supported values: powershell (pwsh), cmd (bat), bash (sh, zsh), fish, nu (nushell)",
		)
	}
	return nil
}

func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
