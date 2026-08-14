// Package speakwrite embeds the drafting runner's agent skill so
// telescreen install can write it without the repo checkout.
package speakwrite

import _ "embed"

// SkillMD is the skill telescreen install writes to
// ~/.claude/skills/speakwrite/SKILL.md.
//
//go:embed SKILL.md
var SkillMD []byte
