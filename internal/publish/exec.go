package publish

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// execTimeout bounds a configured command, matching the agentrun
// default.
const execTimeout = 600 * time.Second

// execPublisher builds the configured posting backend for one command
// template: argv exec, no shell, an element that is exactly {url}
// becomes the record URL as one argument, the draft goes on stdin, the
// process environment (the unit's thinkpol.env included) is inherited.
// Exit 0 is success; the first non-empty stdout line becomes the
// permalink when it parses as a URL, else the record URL stands in.
// Non-zero exit fails the post with stderr's tail.
func execPublisher(command string) Publisher {
	return Publisher{
		Name:  "exec",
		Match: func(string) bool { return true },
		Post: func(rawURL, draft string) (string, error) {
			var argv []string
			for _, el := range strings.Fields(command) {
				if el == "{url}" {
					el = rawURL
				}
				argv = append(argv, el)
			}
			ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(draft)
			var out, errb bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errb
			if err := cmd.Run(); err != nil {
				if t := tail(errb.String()); t != "" {
					return "", fmt.Errorf("exec %s: %w: %s", argv[0], err, t)
				}
				return "", fmt.Errorf("exec %s: %w", argv[0], err)
			}
			for _, line := range strings.Split(out.String(), "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if looksLikeURL(line) {
					return line, nil
				}
				break
			}
			return rawURL, nil
		},
	}
}

// looksLikeURL mirrors the verify check: an absolute URL with a scheme
// and a host.
func looksLikeURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

// tail keeps stderr's last lines so the log names the failure without
// carrying the whole stream.
func tail(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 5 {
		lines = lines[len(lines)-5:]
	}
	return strings.Join(lines, "\n")
}
