package cli

import "bitbucket.org/hgt_development/hgtran-ai/v2/internal/model"

func isGentlemanConversationPersona(persona model.PersonaID) bool {
	return persona == model.PersonaGentleman || persona == model.PersonaGentlemanNeutralArtifacts
}
