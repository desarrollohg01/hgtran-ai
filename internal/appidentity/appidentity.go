// Package appidentity holds the canonical name of this tool.
//
// The name reaches PATH lookups, self-update tool matching, doctor checks,
// release-policy validation and user-facing output. Before this package
// existed it was a string literal repeated across every one of those call
// sites, which is why renaming the binary could not be verified by the
// compiler: a literal that was missed still built, still passed vet, and
// failed only at runtime.
//
// Routing every behavioural comparison through one constant turns the next
// rename into a one-line change and turns a missed site into a build error.
// It follows the precedent set by internal/statepath for the state root.
package appidentity

// Name is the binary, command and self-update tool name.
const Name = "hgtran-ai"

// LegacyName is the name this tool shipped under before the fork.
//
// It is kept, and MUST be kept distinct from Name, so migration paths can
// recognise state, binaries and directories left behind by that version.
// Nothing new is ever created under it.
const LegacyName = "gentle-ai"

// Candidates returns the names this tool may be known by on a user's machine,
// current first.
//
// Anything that resolves a binary, a directory or a PATH entry the tool did not
// create in this run MUST try them in this order. After the rename the current
// name is the correct one to write, but an install that predates it has the
// pre-fork name on disk and nothing else — so a lookup that knows only one name
// is a lookup that fails for every existing user.
//
// Order is load-bearing, not cosmetic: mid-migration a machine can carry both
// binaries at once, and resolving to the older one would keep using the very
// install the rename set out to replace.
func Candidates() []string {
	return []string{Name, LegacyName}
}

// ResolveExisting returns the first candidate that exists reports true for, and
// whether any did.
//
// It takes a predicate rather than touching the filesystem so the same shape
// serves a PATH lookup, a directory probe and a registry match, and so its own
// tests need no temporary directories. Callers decide what a legacy hit means:
// the established treatment is to use it, report it, and never move anything on
// the user's behalf — see internal/app.writeLegacyStateNotice for why acting
// automatically is the wrong default on Windows.
func ResolveExisting(exists func(name string) bool) (string, bool) {
	if exists == nil {
		return "", false
	}
	for _, name := range Candidates() {
		if exists(name) {
			return name, true
		}
	}
	return "", false
}
