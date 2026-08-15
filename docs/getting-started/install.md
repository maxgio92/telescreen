# Install

Get the binary and see the screen. No agents yet, no timers, nothing
enrolled.

## The binary

```
go install github.com/maxgio92/telescreen@main
```

Install from `@main` for now; prebuilt release archives return with the
next release. Once releases exist, `telescreen update` swaps the
installed binary for the latest one.

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
