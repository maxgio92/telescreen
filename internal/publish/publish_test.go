package publish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMatch(t *testing.T) {
	cases := []struct {
		rawURL string
		name   string
		ok     bool
	}{
		{"https://github.com/o/r/pull/1", "github-pr", true},
		{"https://github.com/o/r/pull/1/files", "github-pr", true},
		{"https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456", "slack-thread", true},
		{"https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456?thread_ts=1754000000.000100", "slack-thread", true},
		{"https://linear.app/acme/issue/FUL-123", "linear-issue", true},
		{"https://linear.app/acme/issue/FUL-123/some-title", "linear-issue", true},
		{"https://github.com/o/r/issues/7", "", false},
		{"https://acme.slack.com/", "", false},
		{"https://slack.com/archives/C0A86EX00GH/p1755000000123456", "", false},
		{"https://linear.app/acme/document/roadmap-abc", "", false},
		{"https://example.com/thread/1", "", false},
	}
	for _, c := range cases {
		name, ok := Match(c.rawURL)
		if name != c.name || ok != c.ok {
			t.Errorf("Match(%q) = %q, %v; want %q, %v", c.rawURL, name, ok, c.name, c.ok)
		}
	}
}

// rewriteTransport sends every request to the test server, keeping the
// request path so the handler can dispatch on it.
type rewriteTransport struct{ base *url.URL }

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.URL.Scheme = t.base.Scheme
	r.URL.Host = t.base.Host
	return http.DefaultTransport.RoundTrip(r)
}

// serve points HTTPClient at handler for the test's duration.
func serve(t *testing.T, handler http.Handler) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	orig := HTTPClient
	HTTPClient = &http.Client{Transport: rewriteTransport{base: base}}
	t.Cleanup(func() { HTTPClient = orig })
}

func TestSlackPost(t *testing.T) {
	cases := []struct {
		name         string
		rawURL       string
		wantThreadTS string
	}{
		{
			name:         "thread_ts from the query",
			rawURL:       "https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456?thread_ts=1754000000.000100",
			wantThreadTS: "1754000000.000100",
		},
		{
			name:         "thread_ts derived from the p segment",
			rawURL:       "https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456",
			wantThreadTS: "1755000000.123456",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("SLACK_TOKEN", "xoxp-test")
			var gotAuth string
			var gotBody map[string]string
			serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/api/chat.postMessage" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				gotAuth = r.Header.Get("Authorization")
				if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
					t.Error(err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "ts": "1755000099.000200"})
			}))

			name, permalink, err := Post(c.rawURL, "the draft text")
			if err != nil {
				t.Fatal(err)
			}
			if name != "slack-thread" {
				t.Errorf("publisher = %q, want slack-thread", name)
			}
			if gotAuth != "Bearer xoxp-test" {
				t.Errorf("Authorization = %q, want Bearer xoxp-test", gotAuth)
			}
			want := map[string]string{
				"channel":   "C0A86EX00GH",
				"thread_ts": c.wantThreadTS,
				"text":      "the draft text",
			}
			for k, v := range want {
				if gotBody[k] != v {
					t.Errorf("body[%s] = %q, want %q", k, gotBody[k], v)
				}
			}
			wantLink := "https://acme.slack.com/archives/C0A86EX00GH/p1755000099000200"
			if permalink != wantLink {
				t.Errorf("permalink = %q, want %q", permalink, wantLink)
			}
		})
	}
}

func TestSlackPostAPIError(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "xoxp-test")
	serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": "not_in_channel"})
	}))
	_, _, err := Post("https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456", "d")
	if err == nil || !strings.Contains(err.Error(), "not_in_channel") {
		t.Errorf("err = %v, want the slack error string", err)
	}
}

func TestSlackPostMissingToken(t *testing.T) {
	t.Setenv("SLACK_TOKEN", "")
	serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a missing token still hit the API")
	}))
	_, _, err := Post("https://acme.slack.com/archives/C0A86EX00GH/p1755000000123456", "d")
	if err == nil || !strings.Contains(err.Error(), "SLACK_TOKEN") {
		t.Errorf("err = %v, want a SLACK_TOKEN error", err)
	}
}

func TestLinearPost(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_api_test")
	type call struct {
		auth      string
		query     string
		variables map[string]any
	}
	var calls []call
	serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		calls = append(calls, call{auth: r.Header.Get("Authorization"), query: body.Query, variables: body.Variables})
		switch len(calls) {
		case 1:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"issue": map[string]any{"id": "uuid-issue-1"}}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"commentCreate": map[string]any{"comment": map[string]any{"id": "uuid-comment-9"}}}})
		}
	}))

	rawURL := "https://linear.app/acme/issue/FUL-123/fix-the-thing"
	name, permalink, err := Post(rawURL, "the draft text")
	if err != nil {
		t.Fatal(err)
	}
	if name != "linear-issue" {
		t.Errorf("publisher = %q, want linear-issue", name)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d GraphQL calls, want 2", len(calls))
	}
	if calls[0].auth != "lin_api_test" || calls[1].auth != "lin_api_test" {
		t.Errorf("Authorization headers = %q, %q; want the api key", calls[0].auth, calls[1].auth)
	}
	if !strings.Contains(calls[0].query, "issue(id: $id)") || calls[0].variables["id"] != "FUL-123" {
		t.Errorf("first call is not the issue lookup: %+v", calls[0])
	}
	if !strings.Contains(calls[1].query, "commentCreate") ||
		calls[1].variables["issueId"] != "uuid-issue-1" ||
		calls[1].variables["body"] != "the draft text" {
		t.Errorf("second call is not the comment create: %+v", calls[1])
	}
	if want := rawURL + "#comment-uuid-comment-9"; permalink != want {
		t.Errorf("permalink = %q, want %q", permalink, want)
	}
}

func TestLinearPostGraphQLError(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "lin_api_test")
	serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"errors": []map[string]any{{"message": "Entity not found"}}})
	}))
	_, _, err := Post("https://linear.app/acme/issue/FUL-123", "d")
	if err == nil || !strings.Contains(err.Error(), "Entity not found") {
		t.Errorf("err = %v, want the graphql error message", err)
	}
}

func TestLinearPostMissingKey(t *testing.T) {
	t.Setenv("LINEAR_API_KEY", "")
	serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a missing key still hit the API")
	}))
	_, _, err := Post("https://linear.app/acme/issue/FUL-123", "d")
	if err == nil || !strings.Contains(err.Error(), "LINEAR_API_KEY") {
		t.Errorf("err = %v, want a LINEAR_API_KEY error", err)
	}
}

func TestGitHubPost(t *testing.T) {
	var gotArgs []string
	var gotStdin string
	orig := GHRun
	GHRun = func(args []string, stdin string) (string, error) {
		gotArgs, gotStdin = args, stdin
		return "noise\nhttps://github.com/o/r/pull/1#issuecomment-9\n", nil
	}
	t.Cleanup(func() { GHRun = orig })

	name, permalink, err := Post("https://github.com/o/r/pull/1", "the draft text")
	if err != nil {
		t.Fatal(err)
	}
	if name != "github-pr" {
		t.Errorf("publisher = %q, want github-pr", name)
	}
	want := []string{"pr", "comment", "1", "--repo", "o/r", "--body-file", "-"}
	if len(gotArgs) != len(want) {
		t.Fatalf("gh args = %v, want %v", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Fatalf("gh args = %v, want %v", gotArgs, want)
		}
	}
	if gotStdin != "the draft text" {
		t.Errorf("gh stdin = %q, want the draft text", gotStdin)
	}
	if want := "https://github.com/o/r/pull/1#issuecomment-9"; permalink != want {
		t.Errorf("permalink = %q, want %q", permalink, want)
	}
}

func TestPostNoPublisher(t *testing.T) {
	_, _, err := Post("https://example.com/thread/1", "d")
	if err == nil || !strings.Contains(err.Error(), "no publisher") {
		t.Errorf("err = %v, want a no-publisher error", err)
	}
}
