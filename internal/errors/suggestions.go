package errors

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
)

// SuggestClose returns fuzzy-matched close alternatives to query from candidates.
// Returns up to maxResults results.
func SuggestClose(query string, candidates []string, maxResults int) []string {
	if len(candidates) == 0 {
		return nil
	}
	matches := fuzzy.Find(query, candidates)
	sort.Stable(matches)
	results := make([]string, 0, maxResults)
	for i, m := range matches {
		if i >= maxResults {
			break
		}
		results = append(results, m.Str)
	}
	return results
}

// SuggestArch produces suggestions when an arch string is unknown.
func SuggestArch(bad string, available []string) *QvmError {
	suggestions := SuggestClose(bad, available, 3)
	err := Newf(CodeUnknownArch, "arch %q is not available", bad)
	cmds := make([]string, 0, len(suggestions)+1)
	for _, s := range suggestions {
		cmds = append(cmds, fmt.Sprintf("  %s", s))
	}
	if len(cmds) > 0 {
		err.Message += "\n\nDid you mean one of:\n" + joinLines(cmds)
	}
	return err
}

// SuggestModule produces suggestions when a module name is unknown.
func SuggestModule(bad string, available []string) *QvmError {
	suggestions := SuggestClose(bad, available, 3)
	err := Newf(CodeUnknownModule, "module %q is not known", bad)
	if len(suggestions) > 0 {
		cmds := make([]string, len(suggestions))
		for i, s := range suggestions {
			cmds[i] = fmt.Sprintf("  %s", s)
		}
		err.Message += "\n\nDid you mean:\n" + joinLines(cmds)
	} else {
		err.Message += "\n\nRun 'qvm search " + bad + "' to search for modules."
	}
	return err
}

// SuggestVersion produces suggestions when a version string is unknown.
func SuggestVersion(bad string, available []string) *QvmError {
	suggestions := SuggestClose(bad, available, 3)
	err := Newf(CodeUnknownVersion, "Qt version %q is not available", bad)
	if len(suggestions) > 0 {
		cmds := make([]string, len(suggestions))
		for i, s := range suggestions {
			cmds[i] = fmt.Sprintf("  qvm install qt@%s", s)
		}
		err.Message += "\n\nDid you mean:\n" + joinLines(cmds)
	} else {
		err.Message += "\n\nRun 'qvm list --all' to see available versions."
	}
	return err
}

func joinLines(lines []string) string {
	return strings.Join(lines, "\n") + "\n"
}
