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
	"slices"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/trollixx/qvm/internal/storage"
)

// archHint explains how to supply an architecture; reused by the arch
// parse errors below.
const archHint = "Provide an architecture, e.g. '-a win64_msvc2022_64'."

const execUsageHint = "Usage:\n" +
	"  qvm exec <command> [args...]\n" +
	"  qvm exec [<version>] -- <command> [args...]\n\n" +
	"Examples:\n" +
	"  qvm exec 6.8.3 -- qmake --version\n" +
	"  qvm exec 6.8.3 -- cmake -S . -B build\n" +
	"  qvm exec -- qmake --version    (uses the default version)\n\n" +
	"Tip: set a default version with 'qvm use <version>' to omit it here."

func (a *app) newExecCommand() *cli.Command {
	return &cli.Command{
		Name:      "exec",
		Usage:     "Run a command with a Qt version's environment",
		ArgsUsage: "[<version> --] <command> [args...]",
		// The child command and its flags must reach the child untouched, so we
		// disable cli's flag parsing and interpret exec's own arguments in
		// parseExecArgs instead. newArchFlag stays declared for the help text.
		SkipFlagParsing: true,
		CommandNotFound: showHelpOnNotFound,
		Flags: []cli.Flag{
			newArchFlag(),
		},
		Action: a.runExec,
	}
}

func (a *app) runExec(ctx context.Context, cmd *cli.Command) error {
	parsed, err := parseExecArgs(cmd.Args().Slice())
	if err != nil {
		return err
	}
	if parsed.help {
		return cli.ShowSubcommandHelp(cmd)
	}
	if len(parsed.child) == 0 {
		return newHintError("missing command", execUsageHint)
	}

	version, err := resolveVersionArg(parsed.version)
	if err != nil {
		return err
	}
	if version == "" {
		return newHintError("no default Qt version set",
			"Pass one before '--' (qvm exec <version> -- <command>) "+
				"or set a default with 'qvm use <version>'.")
	}

	registry, err := storage.NewRegistryManager()
	if err != nil {
		return fmt.Errorf("opening registry: %w", err)
	}

	reg, err := registry.Load()
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	qt, err := resolveInstalledQt(reg, version, parsed.arch, "exec")
	if err != nil {
		return err
	}

	return execChild(ctx, parsed.child, mergedEnv(buildQtEnv(qt.InstallDir, os.Getenv)))
}

// execArgs holds the result of interpreting exec's raw command line.
type execArgs struct {
	arch    string
	version string
	child   []string
	help    bool
}

// parseExecArgs interprets exec's arguments. Because flag parsing is disabled at
// the cli layer (SkipFlagParsing), exec owns its own option handling here.
//
// Leading -a/--arch (and -h/--help) are consumed first, like sudo/env accepting
// their own flags ahead of the child. "--" then separates an optional version
// from the child command: a single token before "--" is the version, everything
// after "--" is the child command and is passed through verbatim. Without "--",
// the remaining arguments are the child command and the configured default
// version applies. A version is never inferred from an argument's shape.
func parseExecArgs(args []string) (execArgs, error) {
	var parsed execArgs

	// Consume leading qvm flags, stopping at the first non-flag token or "--".
	i := 0
flags:
	for i < len(args) {
		arg := args[i]
		switch {
		case arg == "--":
			break flags
		case isHelpFlag(arg):
			return execArgs{help: true}, nil
		case arg == "-a" || arg == "--arch":
			if i+1 >= len(args) || args[i+1] == "--" {
				return execArgs{}, newHintError("missing value for "+arg, archHint)
			}
			parsed.arch = args[i+1]
			i += 2
		case strings.HasPrefix(arg, "--arch="), strings.HasPrefix(arg, "-a="):
			name, val, _ := strings.Cut(arg, "=")
			if val == "" {
				return execArgs{}, newHintError("missing value for "+name, archHint)
			}
			parsed.arch = val
			i++
		default:
			break flags
		}
	}

	rest := args[i:]
	sep := slices.Index(rest, "--")
	if sep < 0 {
		// No separator: the remaining arguments are the child command.
		parsed.child = rest
		return parsed, nil
	}

	version, child := rest[:sep], rest[sep+1:]
	parsed.child = child
	if len(version) > 1 {
		return execArgs{}, newHintError(
			fmt.Sprintf("unexpected arguments before '--': %s", strings.Join(version, " ")),
			"Only an optional version may precede '--'; put the command after it.\n\n"+execUsageHint)
	}
	if len(version) == 1 {
		parsed.version = version[0]
	}
	return parsed, nil
}

// isHelpFlag reports whether arg is exec's help flag.
func isHelpFlag(arg string) bool {
	return arg == "-h" || arg == "--help"
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
