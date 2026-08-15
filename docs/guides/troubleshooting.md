# Troubleshooting

All logs live next to the drawers, under
`${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`: `produce.log` for the
producer, `draft.log` for the drafting runner, `publish.log` for the
actor. Start there; the record files themselves are the audit trail.

| Symptom | Likely cause | Fix |
|---|---|---|
| No records appear | The minitrue timer is not enrolled, or the identity in `~/.config/minitrue.env` is missing | `systemctl --user status minitrue.timer`; check `produce.log` for the run's own account; set SLACK_USER_ID, GH_LOGIN, REPO per [enroll.md](../getting-started/enroll.md) |
| A draft never lands | The speakwrite path unit is not enrolled, or an intent is stuck | `systemctl --user status speakwrite.path`; look in `recdep/intents/` for the leftover `.intent`; check `draft.log` |
| Publish fails | A token is missing from `~/.config/thinkpol.env` | `publish.log` names the reason per approval; add SLACK_TOKEN or LINEAR_API_KEY, or authenticate `gh` for GitHub |
| `telescreen verify` reports findings | A producer wrote nonconforming files, or file modes drifted | Each finding names the file and the broken rule from [recdep.md](../contracts/recdep.md); grammar findings mean fix the producer, mode warnings mean `chmod 600` the file (verify warns, it never chmods) |

A disappeared approval is always explained somewhere: the actor logs
one line per approval it consumes, success or not.
