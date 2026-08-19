package skillset

import "embed"

// FS is the complete shipped skills directory (router plus every capability).
//
//go:embed skills/*/SKILL.md
var FS embed.FS
