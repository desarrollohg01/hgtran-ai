package engram

import (
	"github.com/desarrollohg01/hgtran-ai/v2/internal/installcmd"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/model"
	"github.com/desarrollohg01/hgtran-ai/v2/internal/system"
)

func InstallCommand(profile system.PlatformProfile) ([][]string, error) {
	return installcmd.NewResolver().ResolveComponentInstall(profile, model.ComponentEngram)
}
