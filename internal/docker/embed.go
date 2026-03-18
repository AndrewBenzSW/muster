package docker

import "embed"

//go:embed all:docker
var Assets embed.FS

// Assets contains embedded Docker-related files.
// Phase 2 will add Dockerfiles; this is Phase 0 scaffolding.
