package reviewtransaction

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// currentBranch is a test-local convenience wrapper around the same Git
// plumbing gate.go itself would use to resolve the current branch name. The
// production helper it used to mirror was retired along with the Native
// Gate / push-target subsystem, but selectPrePRBoundary and publicationRemote
// tests still need a way to name the checked-out branch, so it lives here
// instead of being reintroduced into production code.
func currentBranch(ctx context.Context, repo string) string {
	output, _ := runGit(ctx, repo, nil, nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	return strings.TrimSpace(string(output))
}

func trimGit(value string) string {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}

func configurePublicationRemote(t *testing.T, repo, branch string) string {
	t.Helper()
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitSnapshot(t, repo, "clone", "--bare", repo, remote)
	gitSnapshot(t, repo, "remote", "add", "origin", remote)
	gitSnapshot(t, repo, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	return remote
}

// TestSelectPrePRBoundaryUsesExactExplicitOrPublicationDefaultCommit pins
// selectPrePRBoundary's two selection sources: an explicit advertised
// remote-branch selector resolves to that remote's exact commit, and an
// empty selector falls back to the publication remote's default branch.
func TestSelectPrePRBoundaryUsesExactExplicitOrPublicationDefaultCommit(t *testing.T) {
	repo := initSnapshotRepo(t)
	baseCommit := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "branch", "main", baseCommit)
	remote := configurePublicationRemote(t, repo, "main")
	gitSnapshot(t, repo, "--git-dir", remote, "branch", "reviewed-base", baseCommit)

	tests := []struct {
		name     string
		selector string
		want     PrePRBoundarySelection
	}{
		{
			name:     "explicit current chained base",
			selector: "origin/reviewed-base",
			want: PrePRBoundarySelection{
				Source: PrePRBoundaryExplicit, Selector: "origin/reviewed-base", Commit: baseCommit, MergeBase: baseCommit, Remote: "origin", RemoteRef: "refs/heads/reviewed-base",
			},
		},
		{
			name: "publication default without selector",
			want: PrePRBoundarySelection{
				Source: PrePRBoundaryPublicationDefault, Selector: "refs/heads/main", Commit: trimGit(gitSnapshot(t, repo, "rev-parse", "main")), MergeBase: baseCommit, Remote: "origin", RemoteRef: "refs/heads/main",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectPrePRBoundary(context.Background(), repo, tt.selector)
			got.RemoteIdentity = ""
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("selectPrePRBoundary(%q) = %#v, want %#v", tt.selector, got, tt.want)
			}
		})
	}
}

// TestDefaultBoundariesSeparatePrePRTargetFromPrePushTracking pins that the
// default (no-selector) PRE-PR boundary resolves from the publication
// remote's default branch even when the checked-out branch's own tracking
// config points elsewhere. The upstream test paired this with a PRE-PUSH
// boundary assertion via buildPushTarget; that half targeted the retired
// Native Gate / push-target subsystem and is gone from both current and
// upstream, so only the selectPrePRBoundary half is restored here.
func TestDefaultBoundariesSeparatePrePRTargetFromPrePushTracking(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	gitSnapshot(t, repo, "branch", "main", base)
	gitSnapshot(t, repo, "branch", "feature", base)
	configurePublicationRemote(t, repo, "main")
	gitSnapshot(t, repo, "checkout", "feature")
	gitSnapshot(t, repo, "config", "branch.feature.remote", "origin")
	gitSnapshot(t, repo, "config", "branch.feature.merge", "refs/heads/feature")

	prePR, err := selectPrePRBoundary(context.Background(), repo, "")
	if err != nil || prePR.RemoteRef != "refs/heads/main" {
		t.Fatalf("default PRE-PR boundary = %#v, %v", prePR, err)
	}
}

func TestSelectPrePRBoundaryRejectsAbsolutePathLikeSelector(t *testing.T) {
	repo := initSnapshotRepo(t)
	if _, err := selectPrePRBoundary(context.Background(), repo, filepath.Join(repo, "outside")); err == nil {
		t.Fatal("absolute path-like selector was accepted")
	}
}

func TestSelectPrePRBoundaryRejectsAdvertisedUnrelatedSameTree(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := currentBranch(context.Background(), repo)
	remote := configurePublicationRemote(t, repo, branch)
	writeSnapshotFile(t, repo, "delivery.txt", "delivery\n")
	gitSnapshot(t, repo, "add", "delivery.txt")
	gitSnapshot(t, repo, "commit", "-m", "delivery")
	wantTree := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}"))
	if _, err := selectPrePRBoundary(context.Background(), repo, "origin/"+branch); err != nil {
		t.Fatalf("valid chained base rejected: %v", err)
	}

	gitSnapshot(t, repo, "checkout", "--orphan", "unrelated")
	gitSnapshot(t, repo, "commit", "-m", "unrelated same tree")
	if got := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD^{tree}")); got != wantTree {
		t.Fatalf("unrelated tree = %s, want %s", got, wantTree)
	}
	gitSnapshot(t, repo, "push", remote, "HEAD:refs/heads/unrelated")
	gitSnapshot(t, repo, "checkout", branch)
	if _, err := selectPrePRBoundary(context.Background(), repo, "origin/unrelated"); err == nil || !strings.Contains(err.Error(), "0 merge bases") {
		t.Fatalf("advertised unrelated same-tree base error = %v", err)
	}
}

func TestSelectPrePRBoundaryUsesRepositoryRootForSubdirectories(t *testing.T) {
	repo := initSnapshotRepo(t)
	want := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	branch := currentBranch(context.Background(), repo)
	configurePublicationRemote(t, repo, branch)
	subdirectory := filepath.Join(repo, "nested", "work")
	if err := os.MkdirAll(subdirectory, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, location := range []string{repo, subdirectory} {
		selection, err := selectPrePRBoundary(context.Background(), location, "origin/"+branch)
		if err != nil || selection.Source != PrePRBoundaryExplicit || selection.Commit != want {
			t.Fatalf("selectPrePRBoundary(%q) = %#v, %v", location, selection, err)
		}
	}
}

func TestPublicationRemoteUsesGitPushPrecedence(t *testing.T) {
	repo := initSnapshotRepo(t)
	branch := strings.TrimSpace(gitSnapshot(t, repo, "symbolic-ref", "--short", "HEAD"))
	defaultRemote := filepath.Join(t.TempDir(), "default.git")
	branchRemote := filepath.Join(t.TempDir(), "branch.git")
	gitSnapshot(t, repo, "clone", "--bare", repo, defaultRemote)
	gitSnapshot(t, repo, "clone", "--bare", repo, branchRemote)
	gitSnapshot(t, repo, "remote", "add", "default-push", defaultRemote)
	gitSnapshot(t, repo, "remote", "add", "branch-push", branchRemote)
	gitSnapshot(t, repo, "config", "remote.pushDefault", "default-push")
	gitSnapshot(t, repo, "config", "branch."+branch+".pushRemote", "branch-push")

	remote, configured, err := publicationRemote(context.Background(), repo)
	if err != nil || !configured || remote != "branch-push" {
		t.Fatalf("publicationRemote() = %q, %v, %v", remote, configured, err)
	}
}

func TestPublicationRemotePreservesTrackingFirstPushAndExplicitRefspecAuthority(t *testing.T) {
	for _, tt := range []struct {
		name      string
		configure func(t *testing.T, repo, branch string)
		want      string
	}{
		{
			name: "tracking branch", want: "tracking",
			configure: func(t *testing.T, repo, branch string) {
				gitSnapshot(t, repo, "remote", "add", "tracking", filepath.Join(t.TempDir(), "tracking.git"))
				gitSnapshot(t, repo, "config", "branch."+branch+".remote", "tracking")
			},
		},
		{
			name: "first push configuration", want: "publish",
			configure: func(t *testing.T, repo, branch string) {
				gitSnapshot(t, repo, "remote", "add", "publish", filepath.Join(t.TempDir(), "publish.git"))
				gitSnapshot(t, repo, "config", "branch."+branch+".pushRemote", "publish")
			},
		},
		{
			name: "explicit refspec retains origin", want: "origin",
			configure: func(t *testing.T, repo, _ string) {
				gitSnapshot(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "origin.git"))
				gitSnapshot(t, repo, "config", "remote.origin.push", "refs/heads/main:refs/heads/main")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := initSnapshotRepo(t)
			branch := strings.TrimSpace(gitSnapshot(t, repo, "symbolic-ref", "--short", "HEAD"))
			tt.configure(t, repo, branch)
			got, configured, err := publicationRemote(context.Background(), repo)
			if err != nil || !configured || got != tt.want {
				t.Fatalf("publicationRemote() = %q, %v, %v; want %q", got, configured, err, tt.want)
			}
		})
	}
}

// TestPublicationTargetBindsAdvertisedUpstreamSeparatelyFromPushRemote pins
// selectPrePRBoundary's advertised-remote resolution: an explicit
// "<remote>/<branch>" selector binds to that exact remote and commit, a bare
// branch name advertised on more than one remote is refused as ambiguous, and
// a selector nothing advertises (a stale local branch or a raw commit SHA) is
// refused outright. The upstream test paired these assertions with
// buildPushTarget/prePushTargetForRequest coverage; that half targeted the
// retired Native Gate / push-target subsystem and is gone from both current
// and upstream, so only the selectPrePRBoundary assertions are restored here.
func TestPublicationTargetBindsAdvertisedUpstreamSeparatelyFromPushRemote(t *testing.T) {
	repo := initSnapshotRepo(t)
	base := trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))
	upstream, origin := filepath.Join(t.TempDir(), "upstream.git"), filepath.Join(t.TempDir(), "origin.git")
	gitSnapshot(t, repo, "clone", "--bare", repo, upstream)
	gitSnapshot(t, repo, "clone", "--bare", repo, origin)
	gitSnapshot(t, repo, "--git-dir", upstream, "branch", "main", base)
	gitSnapshot(t, repo, "remote", "add", "upstream", upstream)
	gitSnapshot(t, repo, "remote", "add", "origin", origin)

	selection, err := selectPrePRBoundary(context.Background(), repo, "upstream/main")
	if err != nil || selection.Remote != "upstream" || selection.Commit != base {
		t.Fatalf("explicit chained base = %#v, %v", selection, err)
	}
	gitSnapshot(t, repo, "--git-dir", origin, "branch", "main", base)
	if _, err := selectPrePRBoundary(context.Background(), repo, "main"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous explicit selector error = %v", err)
	}
	gitSnapshot(t, repo, "branch", "stale-local", base)
	for _, selector := range []string{"stale-local", trimGit(gitSnapshot(t, repo, "rev-parse", "HEAD"))} {
		if _, err := selectPrePRBoundary(context.Background(), repo, selector); err == nil {
			t.Fatalf("unadvertised selector %q was accepted", selector)
		}
	}
}

func TestValidGitTreeRejectsNullOID(t *testing.T) {
	for _, value := range []string{strings.Repeat("0", 40), strings.Repeat("0", 64)} {
		if validGitTree(value) {
			t.Fatalf("validGitTree(%q) accepted the reserved null OID", value)
		}
	}
	if !validGitTree(strings.Repeat("0", 39) + "1") {
		t.Fatal("validGitTree rejected a valid non-null object ID")
	}
}
