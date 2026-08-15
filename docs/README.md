# telescreen documentation

## What touches the network, what never does

The TUI never touches the network. Only the producer reads upstream,
only the actor writes upstream, and only after a recorded double-key
approval. The store is 0700/0600 plain files under
`~/.local/state/recdep`; every state change is a file rename you can
audit with `ls` and `cat`.

## Reading paths

### Set it up

1. [Install](getting-started/install.md) (2 min): the binary and the
   demo screen.
2. [Enroll the agents](getting-started/enroll.md) (4 min): what
   `telescreen install` writes where, identity, units.
3. [Your first record](getting-started/first-record.md) (3 min): take,
   dictate, draft, approve, on the demo record.

### Run it day to day

- [Configuration](guides/configuration.md) (4 min): every knob, and
  which file it lives in.
- [Write a producer](guides/write-a-producer.md) (3 min): feed the
  screen from anything.
- [Swap the actor](guides/swap-the-actor.md) (3 min): replace the
  posting binary with your own.
- [Troubleshooting](guides/troubleshooting.md) (2 min): symptom, cause,
  fix.

### Understand it deeply

- [The speakwrite design](design/speakwrite.md) (4 min): the drafting
  layer, flow, guardrails, rationale.
- [The queue contract](contracts/recdep.md) (4 min): normative; the
  grammar every component honors.
- [The actor contract](contracts/thinkpol.md) (4 min): normative; the
  publish procedure and the publisher table.
- [The names](design/names.md) (3 min): the 1984 lore, for pleasure.

## Reference

- [CLI reference](reference/telescreen.md): every command and flag,
  generated from the binary.
- Contracts: [recdep.md](contracts/recdep.md) for the queue,
  [thinkpol.md](contracts/thinkpol.md) for the actor.
- Files and paths: state under
  `${XDG_STATE_HOME:-$HOME/.local/state}/recdep/`; config in
  `~/.config/minitrue.env`, `~/.config/speakwrite.env`,
  `~/.config/thinkpol.env`, and `~/.config/recdep/config.yaml`; units
  in `~/.config/systemd/user/`; skills in `~/.claude/skills/`.

## Three things that surprise people

1. The screen works with no agents at all. Any process that writes
   conforming files is a producer; the contract is
   [recdep.md](contracts/recdep.md).
2. Nothing is ever posted without a double-key approval recorded on
   disk, and discarding a draft revokes a pending approval.
3. Deleting is real: the memory hole removes the file. There is no
   trash.
