# telescreen

![telescreen](assets/telescreen.png)

telescreen is a terminal dashboard for your work notifications: a
producer polls Slack, GitHub, and Linear and files each event as a
record, one plain file; you triage in a TUI; an agent writes a draft
reaction for each stance you dictate; nothing posts without your
double-key approval.

One binary.<br>
Files are the only interface.<br>
Agents draft, only you approve.

## Install

Homebrew (macOS and Linux):

```
brew install maxgio92/tap/telescreen
```

Go:

```
go install github.com/maxgio92/telescreen@latest
```

Debian, Fedora, and Alpine packages and plain tar.gz archives sit on
the [latest release](https://github.com/maxgio92/telescreen/releases/latest).

## Where to go

- [Set it up](hub.md#set-it-up): install, enroll the agents, your
  first record.
- [Run it day to day](hub.md#run-it-day-to-day): configuration,
  producers, actors, troubleshooting.
- [Understand it deeply](hub.md#understand-it-deeply): the design
  and the contracts.
- [Reference](hub.md#reference): vocabulary, configuration, the
  full CLI.
