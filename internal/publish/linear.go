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

// linearGraphQLURL is the Linear GraphQL endpoint. Tests reroute it by
// swapping HTTPClient's transport.
const linearGraphQLURL = "https://api.linear.app/graphql"

// linearIssueKey matches an issue identifier path segment such as
// FUL-123.
var linearIssueKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-\d+$`)

// linearIssue posts drafts as issue comments via the Linear GraphQL
// API, authorized by the LINEAR_API_KEY environment variable. It
// resolves the issue id with the issue query, which accepts a KEY-123
// identifier as its id argument, then runs commentCreate. The permalink
// is the issue URL plus the returned comment id as a fragment.
var linearIssue = Publisher{
	Name: "linear-issue",
	Match: func(rawURL string) bool {
		_, ok := linearIssueIdentifier(rawURL)
		return ok
	},
	Post: func(rawURL, draft string) (string, error) {
		identifier, ok := linearIssueIdentifier(rawURL)
		if !ok {
			return "", fmt.Errorf("not a linear issue URL: %q", rawURL)
		}
		key := os.Getenv("LINEAR_API_KEY")
		if key == "" {
			return "", fmt.Errorf("LINEAR_API_KEY is not set")
		}
		var issue struct {
			Issue struct {
				ID string `json:"id"`
			} `json:"issue"`
		}
		if err := linearQuery(key,
			`query($id: String!) { issue(id: $id) { id } }`,
			map[string]any{"id": identifier}, &issue); err != nil {
			return "", err
		}
		var created struct {
			CommentCreate struct {
				Comment struct {
					ID string `json:"id"`
				} `json:"comment"`
			} `json:"commentCreate"`
		}
		if err := linearQuery(key,
			`mutation($issueId: String!, $body: String!) { commentCreate(input: {issueId: $issueId, body: $body}) { comment { id } } }`,
			map[string]any{"issueId": issue.Issue.ID, "body": draft}, &created); err != nil {
			return "", err
		}
		return rawURL + "#comment-" + created.CommentCreate.Comment.ID, nil
	},
}

// linearIssueIdentifier parses a https://linear.app/<workspace>/issue/<KEY>-<n>
// URL into the KEY-<n> issue identifier.
func linearIssueIdentifier(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() != "linear.app" {
		return "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 3 || parts[1] != "issue" || !linearIssueKey.MatchString(parts[2]) {
		return "", false
	}
	return parts[2], true
}

// linearQuery runs one GraphQL request and decodes its data field into
// out. GraphQL errors come back as an error naming them.
func linearQuery(key, query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, linearGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("linear graphql: %s: %w", resp.Status, err)
	}
	if len(envelope.Errors) > 0 {
		msgs := make([]string, len(envelope.Errors))
		for i, e := range envelope.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("linear graphql: %s", strings.Join(msgs, "; "))
	}
	return json.Unmarshal(envelope.Data, out)
}
