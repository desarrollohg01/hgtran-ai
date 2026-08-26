// Package statepath resolves every path under the per-user state root.
//
// The root used to be a private const in internal/state, which is why eighteen
// production sites ended up hardcoding the literal instead of asking for it:
// internal/backup, internal/opencode and internal/agents/pi do not import
// internal/state, and none of them was going to start importing the whole state
// model just to obtain a string.
//
// This package is a leaf — standard library only — so anything may depend on it
// without risking an import cycle. It is the single point of change that the
// const promised and never was.
package statepath

import (
	"os"
	"path/filepath"
)

const (
	// DirName is the per-user state root, relative to the home directory.
	DirName = ".hgtran-ai"

	// LegacyDirName is the root used before the identity rename. It is kept
	// only so an install predating the rename can be detected and reported;
	// nothing reads or writes through it.
	LegacyDirName = ".gentle-ai"

	stateFileName = "state.json"
)

// Root returns the per-user state root.
func Root(homeDir string) string { return filepath.Join(homeDir, DirName) }

// StateFile returns the path to the persisted install state.
func StateFile(homeDir string) string { return filepath.Join(Root(homeDir), stateFileName) }

// Backups returns the root under which backup snapshots are stored.
func Backups(homeDir string) string { return filepath.Join(Root(homeDir), "backups") }

// Cache returns the root for regenerable cached data.
func Cache(homeDir string) string { return filepath.Join(Root(homeDir), "cache") }

// ModelVariantsCache returns the OpenCode model-variants cache file. The
// embedded TypeScript plugin at internal/assets/opencode/plugins/model-variants.ts
// writes this same path from outside the Go binary, so the two must agree.
func ModelVariantsCache(homeDir string) string {
	return filepath.Join(Cache(homeDir), "model-variants.json")
}

// ReviewContexts returns the root for persisted review transaction contexts.
func ReviewContexts(homeDir string) string {
	return filepath.Join(Root(homeDir), "review-contexts")
}

// PiCodeGraphManifest returns the Pi code-graph manifest path.
func PiCodeGraphManifest(homeDir string) string {
	return filepath.Join(Root(homeDir), "pi-codegraph.json")
}

// LegacyRoot returns the state root used before the identity rename.
func LegacyRoot(homeDir string) string { return filepath.Join(homeDir, LegacyDirName) }

// LegacyStateFile returns the install state path under the pre-rename root.
func LegacyStateFile(homeDir string) string {
	return filepath.Join(LegacyRoot(homeDir), stateFileName)
}

// OrphanedLegacyState reports whether an install predating the identity rename
// would be ignored: the old root holds a state file and the current one does not.
//
// It keys on state.json rather than on directory existence, because existence is
// a falsifiable signal. Three eager MkdirAll calls inside this binary create the
// root as a side effect — uninstall.NewService does it during construction, even
// on a dry run — and the embedded OpenCode plugin creates the cache subdirectory
// on every editor start. Only state.Write produces state.json, and only that
// means "an installation lives here".
//
// It only reads. Callers use it to tell the user, never to redirect writes: the
// rename is complete, and a stale root is a thing to report, not to adopt.
func OrphanedLegacyState(homeDir string) bool {
	if _, err := os.Stat(StateFile(homeDir)); err == nil {
		return false
	}
	_, err := os.Stat(LegacyStateFile(homeDir))
	return err == nil
}
