<p align="center">
  <a href="https://github.com/maxgio92/telescreen/actions/workflows/ci.yml"><img src="https://github.com/maxgio92/telescreen/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/maxgio92/telescreen/releases/latest"><img src="https://img.shields.io/github/v/release/maxgio92/telescreen" alt="Latest release"></a>
</p>

<p align="center"><img src="assets/telescreen.png" alt="telescreen" width="640"></p>

telescreen is a terminal dashboard for your work notifications: a
producer polls Slack, GitHub, and Linear and files each event as a
record, one plain file; you triage in a TUI; an agent writes a draft
reaction for each stance you dictate; nothing posts without your
double-key approval.

One binary.<br>
Files are the only interface.<br>
Agents draft, only you approve.

[Install](#install) ·
[Documentation](docs/hub.md) ·
[Getting started](docs/getting-started/install.md) ·
[Design](docs/design/speakwrite.md)

<p align="center"><img src="assets/screenshot.png" alt="the telescreen dashboard" width="900"></p>

Your notifications live in three inboxes that all want you now: a Slack
thread here, a review request there, a Linear ticket assigned while you
were reading the other two. You switch contexts, lose the thread, and
answer the loudest one instead of the oldest one. What you want is one
local queue you can audit with `cat`, and agent drafts that never post
a word on their own. That is this.

## Try it

The dashboard alone, no agents, no timers, nothing enrolled:

```
go install github.com/maxgio92/telescreen@latest
telescreen demo
```

That is the screen with one record in the tube: move it with `t`, `u`,
`f`, read it in the detail pane, delete it. Real records and draft
reactions need the agents enrolled (the Install section below); they
run on systemd user units and require the claude CLI.

## Install

<p>
  <img src="https://img.shields.io/badge/homebrew-macOS_and_Linux-2e2a24?style=flat&logo=homebrew" alt="Homebrew">
  <img src="https://img.shields.io/badge/go-install-00ADD8?style=flat&logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/fedora-rpm-51A2DA?style=flat&logo=fedora&logoColor=white" alt="Fedora">
  <img src="https://img.shields.io/badge/debian-deb-A81D33?style=flat&logo=debian&logoColor=white" alt="Debian">
  <img src="https://img.shields.io/badge/alpine-apk-0D597F?style=flat&logo=alpinelinux&logoColor=white" alt="Alpine">
</p>

One binary carries the screen, the agents, the actor, and the
installer. Pick your method; adjust version and arch in the package
URLs (`amd64` or `arm64`).

### Homebrew (macOS and Linux)

```
brew install maxgio92/tap/telescreen
```

### Go

```
go install github.com/maxgio92/telescreen@latest
```

### Fedora and RHEL

dnf installs straight from a URL:

```
dnf install https://github.com/maxgio92/telescreen/releases/latest/download/telescreen_0.1.1_linux_amd64.rpm
```

### Debian and Ubuntu

apt needs the file on disk first:

```
curl -sLO https://github.com/maxgio92/telescreen/releases/latest/download/telescreen_0.1.1_linux_amd64.deb
apt install ./telescreen_0.1.1_linux_amd64.deb
```

### Alpine

The package is unsigned, so apk needs the flag:

```
wget -q https://github.com/maxgio92/telescreen/releases/latest/download/telescreen_0.1.1_linux_amd64.apk
apk add --allow-untrusted ./telescreen_0.1.1_linux_amd64.apk
```

Plain tar.gz archives sit on the
[latest release](https://github.com/maxgio92/telescreen/releases/latest).
Later, `telescreen update` swaps the installed binary for the newest
release in place.

Then enroll the whole stack, or one component at a time:

```
telescreen install                 # minitrue, speakwrite, thinkpol
telescreen install thinkpol        # one component
telescreen install --dry-run       # print the plan without writing
telescreen install --force         # restore the shipped skills over your edits
```

The installer carries everything it enrolls: it writes the agent
skills to `~/.claude/skills/`, the systemd user units to
`~/.config/systemd/user/` (ExecStart points at the installing binary),
seeds missing component keys in `~/.config/telescreen.yaml` without
touching your edits, creates the state dirs, and enables the units.
Identity lives in `~/.config/minitrue.env` (SLACK_USER_ID, GH_LOGIN,
LINEAR_ASSIGNEE, REPO). From source, for development or by choice, `make install`
builds the binary into `~/.local/bin` and runs its installer.

## Documentation

The same pages render as a website at
[telescreen.maxgio.me](https://telescreen.maxgio.me/).

- [Documentation index](docs/hub.md)
- [Getting started](docs/getting-started/install.md)
- [Enroll the agents](docs/getting-started/enroll.md)
- [Your first record](docs/getting-started/first-record.md)
- [Configuration guide](docs/guides/configuration.md)
- [Configuration reference](docs/reference/configuration.md)
- [Write a producer](docs/guides/write-a-producer.md)
- [Swap the actor](docs/guides/swap-the-actor.md)
- [Troubleshooting](docs/guides/troubleshooting.md)
- [Queue contract](docs/contracts/recdep.md)
- [Actor contract](docs/contracts/thinkpol.md)
- [Design: speakwrite](docs/design/speakwrite.md)
- [Design: the names](docs/design/names.md)
- [Vocabulary](docs/reference/vocabulary.md)
- [CLI reference](docs/reference/telescreen.md)

## How it works

Five components, files as every edge:

```mermaid
flowchart TB
    minitrue["minitrue<br/>(producer: polls the sources)"] -->|files records| recdep["recdep<br/>(the queue)"]
    recdep -->|renders| telescreen["telescreen<br/>(the screen)"]
    telescreen -->|intents, approvals| recdep
    recdep -->|intents| speakwrite["speakwrite<br/>(writes draft reactions)"]
    speakwrite -->|drafts| recdep
    recdep -->|approvals| thinkpol["thinkpol<br/>(actor: posts approved drafts)"]
    thinkpol -->|published records| recdep
```

Files are the only interface between them. No sockets, no database, no
shared process: the queue is a directory of markdown files, and each
component reads or writes exactly its own part of it.

The life of one record:

1. An event lands in `tube/` as a file: the producer polled Slack,
   GitHub, or Linear and filed it.
2. You take it to `desk/`: seen, the next move is yours.
3. You dictate a stance with `s`: your editor opens on an intent,
   you write what you think in plain words.
4. The speakwrite agent writes the draft into the record; the row
   turns `[draft]`.
5. You approve with `p` `p`: the double keypress is the recorded
   consent.
6. The actor posts the draft upstream, stamps the record with the
   comment URL, and files it to `upsub/`.

The drawers, one directory per state:

| Drawer | Meaning |
|---|---|
| `tube/` | landed, unseen |
| `desk/` | seen, the next move is yours |
| `upsub/` | you acted, the other side owes the next move |
| `files/` | closed |

## Components

| Component | Does |
|---|---|
| minitrue | the producer: polls the sources, files the records |
| recdep | the queue: one drawer per state |
| telescreen | the screen: this TUI |
| speakwrite | the agent that writes draft reactions: you dictate, it writes |
| thinkpol | the actor: posts approved drafts, nothing else |
| memoryhole | permanent delete: nothing returns |
| tube, desk, upsub, files | the drawers |

The names come from Orwell's 1984, the world where the machinery
watches the human; here the machinery works for you. Why each name is
a precise metaphor: [Design: the names](docs/design/names.md). The
domain terms (record, queue, intent, draft, approval, and the rest)
are defined once in the [Vocabulary](docs/reference/vocabulary.md).

## Usage

```
telescreen          # the screen
telescreen --once   # print per-drawer counts and exit
telescreen export --output json   # every record in the four drawers as one JSON document
telescreen verify   # lint the queue against the record grammar; exit 1 on findings
```

The full CLI reference, including the install, minitrue, speakwrite,
thinkpol, and version subcommands, lives in the
[CLI reference](docs/reference/telescreen.md). `telescreen verify`
lints against the [Queue contract](docs/contracts/recdep.md).

### Keys

| Key | Effect |
|---|---|
| `tab`/`shift+tab`, `1`-`5` | switch view (tube, desk, upsub, files, memoryhole) |
| `j`/`k`, arrows, wheel | navigate; click selects a row or a tab |
| `enter` | read the full record; inside: `j`/`k` line, `space`/`pgup`/`pgdn` page, `g`/`G` ends, `s`/`p p`/`D` act, `q` closes |
| `o` | open the record's URL |
| `y` | copy the URL (wl-copy, fallback xclip) |
| `t` | take (tube to desk) |
| `u` | up (tube or desk to upsub: you answered, their move) |
| `f` | file it (any open drawer to files) |
| `b` | back, one drawer |
| `s` | dictate into the speakwrite (tube, desk, upsub) |
| `r` | quick reply, a small in-TUI popup writing the same intent; inside: `enter` newline, `ctrl+s` submit, `ctrl+e` escalate to the editor, `esc` cancel |
| `p` `p` | approve a draft for posting (records with a matching publisher: GitHub PRs, Slack threads, Linear issues) |
| `D` | discard a draft |
| `x` `x` | delete the record permanently (files only; the first press asks by name) |
| `q` | quit |

The detail pane shows the selected record: the content line, the
preview, then the labeled path (one `cat` away, or one agent handle),
url, and seen lines. A long record outgrows the pane; `enter` opens
the reader, a full-screen scrollable view of the same text.

Under no circumstances does this screen watch you back. That would be
doubleplusungood.
