# Install

Get the binary and see the screen. No agents yet, no timers, nothing
enrolled.

## The binary

### Homebrew (macOS and Linux)

```
brew install maxgio92/tap/telescreen
```

### Go

```
go install github.com/maxgio92/telescreen@latest
```

Debian, Fedora, and Alpine packages and plain tar.gz archives sit on
the [latest release](https://github.com/maxgio92/telescreen/releases/latest);
the README's [Install section](../../README.md#install) has the
per-distro commands. Later, `telescreen update` swaps the installed
binary for the newest release in place.

## First contact

```
telescreen demo
```

This seeds one sample record into the tube and opens the screen. Move
it with `t` (take), `u` (up), `f` (file it), read it in the detail
pane, feed it to the memory hole with `x` `x`. Quit with `q`.

The screen alone is a complete, offline program: it reads files,
renames files, and never touches the network.

## Next

[Enroll the agents](enroll.md) to get the real feed and the drafting,
or jump straight to [your first record](first-record.md) on the demo
seed.
