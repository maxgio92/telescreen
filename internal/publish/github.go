package publish

import (
	"bytes"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
)

// GHRun executes gh with args, feeding it stdin, and returns its
// stdout. A package-level variable so tests substitute a fake.
var GHRun = func(args []string, stdin string) (string, error) {
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

// githubPR posts drafts as pull request comments via an authenticated
// gh. The permalink is the comment URL gh prints on its last stdout
// line.
var githubPR = Publisher{
	Name: "github-pr",
	// The built-in match is github.com only; a routing rule can point
	// other hosts (GitHub Enterprise) here, and Post handles them.
	Match: func(rawURL string) bool {
		u, err := url.Parse(rawURL)
		if err != nil || u.Hostname() != "github.com" {
			return false
		}
		_, _, ok := githubPRParts(rawURL)
		return ok
	},
	Post: func(rawURL, draft string) (string, error) {
		repo, number, ok := githubPRParts(rawURL)
		if !ok {
			return "", fmt.Errorf("not a github pull request URL: %q", rawURL)
		}
		out, err := GHRun([]string{"pr", "comment", number, "--repo", repo, "--body-file", "-"}, draft)
		if err != nil {
			return "", err
		}
		permalink := strings.TrimSpace(out)
		if i := strings.LastIndexByte(permalink, '\n'); i >= 0 {
			permalink = permalink[i+1:]
		}
		return permalink, nil
	},
}

// githubPRParts parses a pull request URL into a gh --repo value and
// the PR number. gh takes HOST/OWNER/REPO for hosts other than
// github.com, so a routed enterprise URL posts to its own host.
func githubPRParts(rawURL string) (repo, number string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Hostname() == "" {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", "", false
	}
	if _, err := strconv.Atoi(parts[3]); err != nil {
		return "", "", false
	}
	repo = parts[0] + "/" + parts[1]
	if h := u.Hostname(); h != "github.com" {
		repo = h + "/" + repo
	}
	return repo, parts[3], true
}
