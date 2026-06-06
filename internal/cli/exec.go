package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/storage"
)

func (a *app) newExecCommand() *cli.Command {
	return &cli.Command{
		Name:            "exec",
		Usage:           "Run a command with a Qt version's environment",
		ArgsUsage:       "<version> [--] <command> [args...]",
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
		},
		Action: a.runExec,
	}
}

func (a *app) runExec(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) < 2 {
		return newHintError("missing arguments",
			"Usage:\n"+
				"  qvm exec <version> [--] <command> [args...]\n\n"+
				"Examples:\n"+
				"  qvm exec 6.8.3 -- qmake --version\n"+
				"  qvm exec 6.8.3 -- cmake -S . -B build")
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	qt, err := resolveInstalledQt(reg, args[0], cmd.String("arch"), "exec")
	if err != nil {
		return err
	}

	return execChild(ctx, args[1:], mergedEnv(buildQtEnv(qt.InstallDir, os.Getenv)))
}

// execChild runs args[0] with args[1:] in the given environment, with stdio
// passed through untouched and the child's exit code propagated as our own.
func execChild(ctx context.Context, args, env []string) error {
	code, err := runChild(ctx, args, env)
	if err != nil {
		return err
	}
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

// runChild starts and waits for the child, returning its exit code. The
// command is resolved against the PATH inside env (not the parent PATH),
// so "qvm exec 6.8.3 qmake" finds the activated Qt's qmake.
func runChild(ctx context.Context, args, env []string) (int, error) {
	resolved, err := lookInEnvPath(args[0], envValue(env, "PATH"), envValue(env, "PATHEXT"))
	if err != nil {
		return 0, err
	}

	c := exec.CommandContext(ctx, resolved, args[1:]...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = env

	// Leave Ctrl+C to the child: ignore SIGINT at the Go level, and on Windows
	// additionally suppress CTRL_C_EVENT delivery to this process once the
	// child is spawned. Go's CTRL_C_EVENT dispatch (even with [signal.Ignore])
	// can corrupt stdin of interactive children that Ctrl+C a grandchild.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)
	if startErr := c.Start(); startErr != nil {
		return 0, fmt.Errorf("starting %s: %w", args[0], startErr)
	}
	ignoreCtrlC(true)
	defer ignoreCtrlC(false)

	if waitErr := c.Wait(); waitErr != nil {
		var ee *exec.ExitError
		if errors.As(waitErr, &ee) {
			return childExitCode(ee), nil
		}
		return 0, fmt.Errorf("running %s: %w", args[0], waitErr)
	}
	return 0, nil
}

// mergedEnv returns the parent environment with overrides applied. On Windows
// keys are matched case-insensitively (PATH vs Path).
func mergedEnv(overrides map[string]string) []string {
	norm := func(s string) string {
		if runtime.GOOS == "windows" {
			return strings.ToUpper(s)
		}
		return s
	}
	replaced := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		replaced[norm(k)] = struct{}{}
	}

	parent := os.Environ()
	keep := make([]string, 0, len(parent)+len(overrides))
	for _, e := range parent {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			continue
		}
		if _, ok := replaced[norm(e[:eq])]; ok {
			continue
		}
		keep = append(keep, e)
	}
	for k, v := range overrides {
		keep = append(keep, k+"="+v)
	}
	return keep
}

// envValue returns the value of name within env. Keys are matched
// case-insensitively on Windows and exactly elsewhere.
func envValue(env []string, name string) string {
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if k == name || (runtime.GOOS == "windows" && strings.EqualFold(k, name)) {
			return v
		}
	}
	return ""
}

// lookInEnvPath resolves file against the directories in pathEnv, honoring
// PATHEXT on Windows. Names containing a path separator are returned as-is.
func lookInEnvPath(file, pathEnv, pathExt string) (string, error) {
	if strings.ContainsAny(file, `\/`) {
		return file, nil
	}
	exts := []string{""}
	if runtime.GOOS == "windows" {
		if pathExt == "" {
			pathExt = ".COM;.EXE;.BAT;.CMD"
		}
		// Lowercased so resolved paths match the typical on-disk casing;
		// the lookup itself is case-insensitive on Windows regardless.
		for x := range strings.SplitSeq(pathExt, ";") {
			x = strings.TrimSpace(x)
			if x != "" {
				exts = append(exts, strings.ToLower(x))
			}
		}
	}
	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			full := filepath.Join(dir, file+ext)
			if info, err := os.Stat(full); err == nil && !info.IsDir() && isExecutable(info) {
				return full, nil
			}
		}
	}
	return "", withHint(
		fmt.Errorf("%q not found in PATH", file),
		"The command is resolved against the activated Qt environment's PATH.",
	)
}

// isExecutable reports whether info describes an executable file. Windows has
// no executable bit; PATHEXT filtering in lookInEnvPath stands in for it.
func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
