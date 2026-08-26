package update

import (
	"path/filepath"
)

// Tools is the static registry of managed tools that can be checked for updates.
//
// InstallMethod controls which non-Homebrew upgrade strategy the executor uses:
//   - InstallBrew: explicitly managed via Homebrew
//   - InstallGoInstall: installed via `go install <GoImportPath>@version`
//   - InstallBinary: downloaded binary from GitHub Releases (atomic replace)
//
// On platforms where Homebrew is available, the executor selects brew only
// when Homebrew confirms it owns the specific tool. Otherwise this field is
// the fallback strategy.
var Tools = []ToolInfo{
	{
		Name:          "gentle-ai",
		Owner:         "desarrollohg01",
		Repo:          "hgtran-ai",
		DetectCmd:     nil, // version comes from build-time ldflags (app.Version)
		VersionPrefix: "v",
		// gentle-ai: Homebrew when the package is brew-owned, authenticated binary
		// release download on Linux/macOS, and `go install` on Windows, where no
		// official signed binary is published.
		InstallMethod: InstallBinary,
		// GoImportPath is what makes the Windows self-upgrade possible. It is
		// deliberately NOT a general opt-in to go-install: effectiveMethod routes
		// gentle-ai on Linux/macOS to InstallBinary regardless of this field, so
		// those platforms keep the minisign-verified release download.
		GoImportPath: "bitbucket.org/hgt_development/hgtran-ai/v2/cmd/gentle-ai",
	},
	{
		Name:              "engram",
		Owner:             "desarrollohg01",
		Repo:              "engram",
		DetectCmd:         []string{"engram", "version"},
		VersionPrefix:     "v",
		ReleaseTagPattern: `^v[0-9]+\.[0-9]+\.[0-9]+$`,
		// engram: Homebrew when the package is brew-owned, binary download elsewhere.
		InstallMethod: InstallBinary,
		// FallbackPaths covers the Windows stale-PATH scenario (and Linux ~/.local/bin
		// when not yet in PATH): AddToUserPath updates the registry/profile but the
		// current process does not see the change until a new shell session starts.
		FallbackPaths: func(homeDir, localAppData string) []string {
			var paths []string
			// Windows: %LOCALAPPDATA%\engram\bin\engram.exe
			if localAppData != "" {
				paths = append(paths, filepath.Join(localAppData, "engram", "bin", "engram.exe"))
			} else if homeDir != "" {
				// LOCALAPPDATA is not set (e.g. restricted environment or CI on Windows).
				// Derive the standard path from homeDir for parity with the installer.
				paths = append(paths, filepath.Join(homeDir, "AppData", "Local", "engram", "bin", "engram.exe"))
			}
			// Linux/macOS: ~/.local/bin/engram (when /usr/local/bin is not writable,
			// the binary installer places it here, which may not be in PATH yet).
			if homeDir != "" {
				paths = append(paths, filepath.Join(homeDir, ".local", "bin", "engram"))
			}
			return paths
		},
	},
	{
		Name:          "hga",
		Owner:         "desarrollohg01",
		Repo:          "hgtran-guardian-angel",
		DetectCmd:     []string{"hga", "--version"},
		VersionPrefix: "v",
		// hga: Homebrew when the package is brew-owned, install.sh script elsewhere.
		// HGA does not publish pre-built release binary assets — only source archives.
		// Using InstallScript runs curl | bash via the project's install.sh.
		InstallMethod: InstallScript,
		// FallbackPaths covers the Windows stale-PATH scenario: hga installs a
		// PowerShell shim to ~/bin/hga.ps1, and the bash script to ~/.local/bin/hga.
		// Both locations may not be in PATH immediately after install.
		FallbackPaths: func(homeDir, localAppData string) []string {
			var paths []string
			if homeDir != "" {
				// Windows: ~/bin/hga.ps1 (PowerShell shim, callable as "hga" in PS)
				paths = append(paths, filepath.Join(homeDir, "bin", "hga.ps1"))
				// Linux/macOS: ~/.local/bin/hga
				paths = append(paths, filepath.Join(homeDir, ".local", "bin", "hga"))
				// Linux/macOS: ~/bin/hga
				paths = append(paths, filepath.Join(homeDir, "bin", "hga"))
			}
			return paths
		},
	},
	{
		Name:          "opencode-subagent-statusline",
		Owner:         "Joaquinvesapa",
		Repo:          "sub-agent-statusline",
		VersionPrefix: "v",
		InstallMethod: InstallOpenCodePlugin,
		NpmPackage:    "opencode-subagent-statusline",
	},
	{
		Name:          "opencode-sdd-engram-manage",
		Owner:         "j0k3r-dev-rgl",
		Repo:          "sdd-engram-plugin",
		VersionPrefix: "v",
		InstallMethod: InstallOpenCodePlugin,
		NpmPackage:    "opencode-sdd-engram-manage",
	},
}
