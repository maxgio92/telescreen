// Package minitrue embeds the producer's agent skill so telescreen
// install can write it without the repo checkout.
package minitrue

import _ "embed"

// SkillMD is the skill telescreen install writes to
// ~/.claude/skills/minitrue/SKILL.md.
//
//go:embed SKILL.md
var SkillMD []byte
