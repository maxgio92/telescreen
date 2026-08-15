## telescreen update

swap this binary for a released one

### Synopsis

update downloads a telescreen release archive, verifies its sha256
against the release's checksums.txt, and replaces the current
executable in place. Without --tag it targets the latest release.
It does not touch the enrolled skills or units; run
telescreen install --force afterwards when the release notes say
the shipped skills changed.

```
telescreen update [flags]
```

### Options

```
  -h, --help         help for update
      --tag string   release tag to install (default: latest)
```

### SEE ALSO

* [telescreen](telescreen.md)	 - a screen for monitoring your daily job, in your terminal

