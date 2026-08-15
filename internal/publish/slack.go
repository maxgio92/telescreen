package publish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
)

// slackAPIBase is the Slack Web API root. Unit tests reroute by
// swapping HTTPClient's transport; e2e tests point the built binary at
// a local server through SLACK_API_BASE.
const slackAPIBase = "https://slack.com/api"

// slackPostMessageURL resolves the chat.postMessage endpoint, honoring
// a SLACK_API_BASE override from the environment.
func slackPostMessageURL() string {
	if v := os.Getenv("SLACK_API_BASE"); v != "" {
		return v + "/chat.postMessage"
	}
	return slackAPIBase + "/chat.postMessage"
}

// slackMessagePath matches the path of a Slack message archives URL:
// /archives/<CHANNEL>/p<digits>.
var slackMessagePath = regexp.MustCompile(`^/archives/([A-Z0-9]+)/p(\d+)$`)

// slackThread posts drafts as thread replies via the Slack Web API
// chat.postMessage, authorized by SLACK_TOKEN. The token choice and the
// thread_ts derivation are documented in docs/contracts/thinkpol.md's publisher table.
var slackThread = Publisher{
	Name: "slack-thread",
	Match: func(rawURL string) bool {
		_, ok := slackTarget(rawURL)
		return ok
	},
	Post: func(rawURL, draft string) (string, error) {
		t, ok := slackTarget(rawURL)
		if !ok {
			return "", fmt.Errorf("not a slack message URL: %q", rawURL)
		}
		token := os.Getenv("SLACK_TOKEN")
		if token == "" {
			return "", fmt.Errorf("SLACK_TOKEN is not set")
		}
		body, err := json.Marshal(map[string]string{
			"channel":   t.channel,
			"thread_ts": t.threadTS,
			"text":      draft,
		})
		if err != nil {
			return "", err
		}
		req, err := http.NewRequest(http.MethodPost, slackPostMessageURL(), bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		resp, err := HTTPClient.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		var out struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
			TS    string `json:"ts"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", fmt.Errorf("chat.postMessage: %s: %w", resp.Status, err)
		}
		if !out.OK {
			return "", fmt.Errorf("chat.postMessage: %s", out.Error)
		}
		return "https://" + t.host + "/archives/" + t.channel + "/p" + strings.ReplaceAll(out.TS, ".", ""), nil
	},
}

// slackMessage is a parsed Slack message archives URL.
type slackMessage struct {
	host     string
	channel  string
	threadTS string
}

// slackTarget parses a https://<workspace>.slack.com/archives/<CHANNEL>/p<digits>
// URL into its posting target.
func slackTarget(rawURL string) (slackMessage, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return slackMessage{}, false
	}
	host := u.Hostname()
	workspace, found := strings.CutSuffix(host, ".slack.com")
	if !found || workspace == "" || strings.Contains(workspace, ".") {
		return slackMessage{}, false
	}
	m := slackMessagePath.FindStringSubmatch(u.Path)
	if m == nil {
		return slackMessage{}, false
	}
	digits := m[2]
	if len(digits) <= 6 {
		// One answer per URL shape: a malformed p segment never
		// matches, with or without a thread_ts query.
		return slackMessage{}, false
	}
	ts := u.Query().Get("thread_ts")
	if ts == "" {
		ts = digits[:len(digits)-6] + "." + digits[len(digits)-6:]
	}
	return slackMessage{host: host, channel: m[1], threadTS: ts}, true
}
