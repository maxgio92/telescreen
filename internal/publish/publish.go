// Package publish is the single publisher table docs/contracts/thinkpol.md calls for:
// the one place that knows which URL shapes the acting layer can post
// to and how. The TUI gates the p key with Match and thinkpol posts
// with Post, so the view can never approve what the actor would refuse.
package publish

import (
	"fmt"
	"net/http"
)

// Publisher posts approved drafts to one target family, identified by
// its URL shape.
type Publisher struct {
	// Name identifies the publisher in logs.
	Name string
	// Match reports whether rawURL is this publisher's shape.
	Match func(rawURL string) bool
	// Post publishes draft to rawURL and returns a permalink to the
	// posted comment or message.
	Post func(rawURL, draft string) (permalink string, err error)
}

// Table lists every publisher in priority order; the first Match wins.
var Table = []Publisher{githubPR, slackThread, linearIssue}

// HTTPClient carries every HTTP call the publishers make, so tests
// point it at httptest servers.
var HTTPClient = http.DefaultClient

// Match returns the name of the first publisher matching rawURL,
// ok=false when none does.
func Match(rawURL string) (name string, ok bool) {
	for _, p := range Table {
		if p.Match(rawURL) {
			return p.Name, true
		}
	}
	return "", false
}

// Post publishes draft through the first publisher matching rawURL and
// returns its name and the permalink. No matching publisher is an
// error; callers wanting to distinguish refusal from failure check
// Match first.
func Post(rawURL, draft string) (name, permalink string, err error) {
	for _, p := range Table {
		if p.Match(rawURL) {
			permalink, err := p.Post(rawURL, draft)
			return p.Name, permalink, err
		}
	}
	return "", "", fmt.Errorf("no publisher for %q", rawURL)
}
