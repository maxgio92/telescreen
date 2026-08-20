//go:build e2e

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUpdate runs <binary> update against an httptest release: the
// release JSON, the platform archive, and checksums.txt all come from
// the same local server through UPDATE_API_BASE. The command swaps
// os.Executable, so the test runs a scratch copy of the built binary
// and asserts the copy was replaced with the stub's bytes.
func TestUpdate(t *testing.T) {
	s := newScratch(t)

	stub := []byte("#!/bin/sh\necho stub telescreen v9.9.9\n")
	archive, digest := buildReleaseArchive(t, stub)
	name := fmt.Sprintf("telescreen_9.9.9_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)

	mux := http.NewServeMux()
	// Download URLs derive from the request's own host, so the handler
	// needs no reference to the server it runs in and stays race-clean.
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host
		_, _ = fmt.Fprintf(w, `{"tag_name": "v9.9.9", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, name, base+"/dl/"+name, base+"/dl/checksums.txt")
	})
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", digest, name)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// A scratch copy keeps the suite's shared binary intact.
	target := filepath.Join(s.binDir, "telescreen")
	built, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, built, 0o755); err != nil {
		t.Fatal(err)
	}

	s.extraEnv = []string{"UPDATE_API_BASE=" + srv.URL}
	out, code := s.runBin(t, target, "update")
	if code != 0 {
		t.Fatalf("update exited %d:\n%s", code, out)
	}
	// The current version is dev on a dirty tree and a VCS
	// pseudo-version on a clean one; only the target matters.
	if !strings.Contains(out, "updated telescreen ") || !strings.Contains(out, " -> 9.9.9") {
		t.Errorf("update did not report the swap:\n%s", out)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, stub) {
		t.Errorf("binary at %s was not replaced with the stub (%d bytes, want %d)", target, len(got), len(stub))
	}
	// The rename left nothing behind: the dir holds exactly the swapped
	// binary, so the assertion cannot go vacuous on a temp-prefix rename.
	entries, err := os.ReadDir(s.binDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "telescreen" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("bin dir after the swap = %v, want only telescreen", names)
	}
}

// buildReleaseArchive returns a tar.gz holding one telescreen member
// with the given contents, plus its sha256 hex digest.
func buildReleaseArchive(t *testing.T, contents []byte) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "telescreen",
		Typeflag: tar.TypeReg,
		Mode:     0o755,
		Size:     int64(len(contents)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return buf.Bytes(), hex.EncodeToString(sum[:])
}
