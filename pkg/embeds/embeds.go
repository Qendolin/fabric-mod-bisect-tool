package embeds

import _ "embed"

//go:embed fabric_loader_dependencies.json
var embeddedOverrides []byte

// GetEmbeddedOverrides returns the content of the built-in dependency override file.
func GetEmbeddedOverrides() []byte {
	return embeddedOverrides
}
