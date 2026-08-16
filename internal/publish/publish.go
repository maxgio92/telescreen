// Package publish is the single publisher table docs/contracts/thinkpol.md calls for:
// the one place that knows which URL shapes the actor can post
// to and how. The TUI gates the p key with Match and thinkpol posts
// with Post, so the view can never approve what the actor would refuse.
package publish

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/maxgio92/telescreen/internal/config"
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
// A publisher added here must also join config's publisherNames, which
// validates rule targets and cannot import this package.
var Table = []Publisher{githubPR, slackThread, linearIssue}

// Rules is the configured routing list (thinkpol.publishers), set by
// callers after config.Load. Rules are consulted in order and the
// first match wins: a rule matches when its publisher is enabled and
// its url_prefix (when set) prefixes the URL. When no rule matches,
// the built-in Match functions run in table order, skipping publishers
// a bare enabled: false rule disabled.
var Rules []config.PublisherRule

// HTTPClient carries every HTTP call the publishers make, so tests
// point it at httptest servers.
var HTTPClient = http.DefaultClient

// resolve picks rawURL's publisher per the Rules semantics above.
func resolve(rawURL string) (Publisher, bool) {
	off := map[string]bool{}
	for _, r := range Rules {
		if !r.On() && r.URLPrefix == "" {
			off[r.Publisher] = true
		}
	}
	for _, r := range Rules {
		if !r.On() || off[r.Publisher] {
			continue
		}
		if r.URLPrefix != "" && !strings.HasPrefix(rawURL, r.URLPrefix) {
			continue
		}
		if r.Publisher == "exec" {
			return execPublisher(r.Command), true
		}
		for _, p := range Table {
			if p.Name == r.Publisher {
				return p, true
			}
		}
	}
	for _, p := range Table {
		if !off[p.Name] && p.Match(rawURL) {
			return p, true
		}
	}
	return Publisher{}, false
}

// Match returns the name of the publisher taking rawURL, ok=false when
// none does.
func Match(rawURL string) (name string, ok bool) {
	p, ok := resolve(rawURL)
	return p.Name, ok
}

// Post publishes draft through the publisher taking rawURL and returns
// its name and the permalink. No matching publisher is an error;
// callers wanting to distinguish refusal from failure check Match
// first.
func Post(rawURL, draft string) (name, permalink string, err error) {
	p, ok := resolve(rawURL)
	if !ok {
		return "", "", fmt.Errorf("no publisher for %q", rawURL)
	}
	permalink, err = p.Post(rawURL, draft)
	return p.Name, permalink, err
}
