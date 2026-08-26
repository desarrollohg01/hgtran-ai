package app

import (
	"fmt"
	"io"
	"os"

	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/statepath"
)

// writeLegacyStateNotice reports an install that predates the identity rename:
// its state lives under the old root, where nothing reads it any more.
//
// It only reports. The old root is not adopted, and nothing is moved on the
// user's behalf: on Windows a directory move fails outright if any file in the
// subtree has an open handle, and a second terminal or a running TUI is
// ordinary use, not an edge case. The user moves it themselves, once, with the
// tool closed — which is also the only version of this operation that can be
// verified before it happens.
//
// It never returns an error and never aborts. Aborting would brick `doctor`,
// the command someone reaches for precisely when something looks wrong.
func writeLegacyStateNotice(out io.Writer, homeDir string) {
	if !statepath.OrphanedLegacyState(homeDir) {
		return
	}
	_, _ = fmt.Fprintf(out,
		"Note: settings from a previous version were found in %s.\n"+
			"      This version reads %s instead, so they are being ignored.\n"+
			"      Close this tool and move the directory to keep them:\n"+
			"        mv %s %s\n\n",
		statepath.LegacyRoot(homeDir),
		statepath.Root(homeDir),
		statepath.LegacyRoot(homeDir),
		statepath.Root(homeDir),
	)
}

// noticeLegacyState wires the check into startup.
//
// It writes to stderr, not to the caller's stdout. The notice is diagnostic: it
// describes the environment, not the result of the command. Sending it to stdout
// put it inside `upgrade --tool engram` output and broke a test that asserts
// that filter shows nothing about the other tool — a fair complaint, since the
// same pollution would land in anything a user pipes or parses.
//
// A home directory that cannot be resolved earns no warning of its own: every
// other path in the binary is already broken at that point, and each reports
// its own failure.
func noticeLegacyState() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	writeLegacyStateNotice(os.Stderr, home)
}
