// Command thinkpol executes recorded publish approvals, the acting layer
// defined in THINKPOL.md. It drains recdep/intents/*.publish oldest
// first, posts each approved draft through its publisher, appends the
// published marker, and renames the entry to upsub/. It never composes
// text and never judges a draft; the human approved, thinkpol acts.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/maxgio92/telescreen/internal/recdep"
)

var version = "dev"

// ghRun executes gh with args, feeding it stdin, and returns its stdout.
// A package-level variable so tests substitute a fake.
var ghRun = func(args []string, stdin string) (string, error) {
	cmd := exec.Command("gh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// A publisher matches an entry URL shape and yields the gh invocation
// that posts a draft there. This dispatch table is where Slack and
// Linear publishers land later.
type publisher struct {
	name string
	// match returns the gh args for raw, or ok=false when raw is not
	// this publisher's shape. The draft text goes on stdin.
	match func(raw string) (args []string, ok bool)
}

var publishers = []publisher{
	{name: "github-pr", match: githubPRArgs},
}

// githubPRArgs matches a github.com pull request URL and returns the
// gh pr comment invocation for it.
func githubPRArgs(raw string) ([]string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() != "github.com" {
		return nil, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return nil, false
	}
	if _, err := strconv.Atoi(parts[3]); err != nil {
		return nil, false
	}
	return []string{"pr", "comment", parts[3], "--repo", parts[0] + "/" + parts[1], "--body-file", "-"}, true
}

// dispatch returns the first matching publisher's name and gh args for
// raw, ok=false when no publisher matches.
func dispatch(raw string) (name string, args []string, ok bool) {
	for _, p := range publishers {
		if args, ok := p.match(raw); ok {
			return p.name, args, true
		}
	}
	return "", nil, false
}

// logLine appends one line to recdep/publish.log (created when missing)
// and mirrors it to stdout, so a disappeared approval is always
// explained somewhere.
func logLine(root, format string, args ...any) {
	line := time.Now().UTC().Format(time.RFC3339) + " " + fmt.Sprintf(format, args...)
	fmt.Println(line)
	f, err := os.OpenFile(filepath.Join(root, "publish.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(line + "\n"); err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
}

// entryPathOf parses the approval's "entry <absolute path>" line.
func entryPathOf(approval string) (string, bool) {
	for _, line := range strings.Split(approval, "\n") {
		if path, ok := strings.CutPrefix(line, "entry "); ok {
			return path, true
		}
	}
	return "", false
}

// resolve locates the entry: the recorded path when it still exists
// inside one of the four drawers under root, else the same filename in
// the drawers. It returns the drawer and the path, or ok=false when the
// entry exists nowhere.
func resolve(root, recorded string) (state, path string, ok bool) {
	if _, err := os.Stat(recorded); err == nil {
		state := filepath.Base(filepath.Dir(recorded))
		if filepath.Dir(filepath.Dir(recorded)) == root && slices.Contains(recdep.States, state) {
			return state, recorded, true
		}
	}
	name := filepath.Base(recorded)
	for _, s := range recdep.States {
		candidate := filepath.Join(root, s, name)
		if _, err := os.Stat(candidate); err == nil {
			return s, candidate, true
		}
	}
	return "", "", false
}

// publish executes one approval file per the THINKPOL.md procedure. The
// approval is always removed: posted, refused, or failed, nothing
// retries silently. Only an unreadable approval or entry returns an
// error and leaves the file for the next run.
func publish(root, approval string) error {
	aname := filepath.Base(approval)
	b, err := os.ReadFile(approval)
	if err != nil {
		return err
	}
	recorded, ok := entryPathOf(string(b))
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "refused %s: malformed approval, no entry line", aname)
		return nil
	}
	state, path, ok := resolve(root, recorded)
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "orphan %s: entry %s missing from every drawer", aname, recorded)
		return nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	e := recdep.ParseEntry(filepath.Base(path), string(body))
	if recdep.DraftDiscarded(e.Body) {
		_ = os.Remove(approval)
		logLine(root, "refused %s: draft discarded", aname)
		return nil
	}
	draft, ok := recdep.LastSection(e.Body, "draft")
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "refused %s: no draft section in %s", aname, path)
		return nil
	}
	pubName, args, ok := dispatch(e.URL)
	if !ok {
		_ = os.Remove(approval)
		logLine(root, "refused %s: no publisher for %q", aname, e.URL)
		return nil
	}
	out, err := ghRun(args, draft)
	if err != nil {
		_ = os.Remove(approval)
		logLine(root, "failed %s: %v", aname, err)
		return nil
	}
	commentURL := strings.TrimSpace(out)
	if i := strings.LastIndexByte(commentURL, '\n'); i >= 0 {
		commentURL = commentURL[i+1:]
	}
	// The post happened; a failure past this point is logged, and the
	// approval still goes so nothing double-posts.
	marker := "--- published " + time.Now().UTC().Format(time.RFC3339) + " " + commentURL + "\n"
	if err := recdep.AppendMarker(path, marker); err != nil {
		logLine(root, "posted %s but the marker append failed: %v", aname, err)
	}
	if state != "upsub" && state != "files" {
		if err := recdep.MoveEntry(root, e.Name, state, "upsub"); err != nil {
			logLine(root, "posted %s but the move to upsub failed: %v", aname, err)
		}
	}
	_ = os.Remove(approval)
	logLine(root, "published %s via %s: %s", aname, pubName, commentURL)
	return nil
}

// drain executes every pending approval. Approval names start with the
// entry's UTC stamp, so the sorted glob walks entries oldest first (the
// posts are independent, so approval age does not matter).
func drain(root string) error {
	approvals, err := filepath.Glob(filepath.Join(root, recdep.IntentsDir, "*.publish"))
	if err != nil {
		return err
	}
	for _, a := range approvals {
		if err := publish(root, a); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("thinkpol " + version)
		return
	}
	root, err := recdep.StateRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := drain(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
