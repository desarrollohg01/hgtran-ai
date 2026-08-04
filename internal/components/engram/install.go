package engram

import (
	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/installcmd"
	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"
	"bitbucket.org/hgt_development/hgtran-ai/v2/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentEngram)
}
