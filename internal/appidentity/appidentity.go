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
