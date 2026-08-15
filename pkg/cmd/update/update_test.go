package update

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

// buildArchive returns a tar.gz holding one telescreen member with the
// given contents, plus its sha256 hex digest.
func buildArchive(t *testing.T, contents []byte) ([]byte, string) {
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

// serveRelease stands in for the GitHub API and the asset downloads.
// It publishes one release for tag with the platform archive and a
// checksums.txt carrying digest for the archive name.
func serveRelease(t *testing.T, tag string, archive []byte, digest string) *httptest.Server {
	t.Helper()
	name := fmt.Sprintf("telescreen_%s_%s_%s.tar.gz", strings.TrimPrefix(tag, "v"), runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	var srv *httptest.Server
	release := func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `{"tag_name": %q, "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": "checksums.txt", "browser_download_url": %q}
		]}`, tag, name, srv.URL+"/dl/"+name, srv.URL+"/dl/checksums.txt")
	}
	mux.HandleFunc("/releases/latest", release)
	mux.HandleFunc("/releases/tags/"+tag, release)
	mux.HandleFunc("/dl/"+name, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", digest, name)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeExe writes a stand-in executable and returns its path.
func fakeExe(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "telescreen")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestRunNoRelease(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	setAPIBase(t, srv.URL)
	err := run(&bytes.Buffer{}, "", fakeExe(t), "dev")
	if err == nil || err.Error() != "no release published yet" {
		t.Fatalf("err = %v, want no release published yet", err)
	}
}

func TestRunAlreadyUpToDate(t *testing.T) {
	archive, digest := buildArchive(t, []byte("new binary"))
	srv := serveRelease(t, "v0.2.0", archive, digest)
	setAPIBase(t, srv.URL)
	exe := fakeExe(t)
	var out bytes.Buffer
	if err := run(&out, "", exe, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "already the release version") {
		t.Errorf("output = %q, want already-up-to-date message", out.String())
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "old binary" {
		t.Errorf("executable changed on the up-to-date path")
	}
}

func TestRunSwapsBinary(t *testing.T) {
	archive, digest := buildArchive(t, []byte("new binary"))
	srv := serveRelease(t, "v0.2.0", archive, digest)
	setAPIBase(t, srv.URL)
	exe := fakeExe(t)
	var out bytes.Buffer
	if err := run(&out, "", exe, "dev"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("executable = %q, want the released binary", got)
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Errorf("mode = %v, want 0755", info.Mode().Perm())
	}
	if !strings.Contains(out.String(), "updated telescreen dev -> 0.2.0") {
		t.Errorf("output = %q, want the version pair", out.String())
	}
	if !strings.Contains(out.String(), "telescreen install --force") {
		t.Errorf("output = %q, want the install --force note", out.String())
	}
	entries, err := os.ReadDir(filepath.Dir(exe))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries after swap, want 1", len(entries))
	}
}

func TestRunChecksumMismatch(t *testing.T) {
	archive, _ := buildArchive(t, []byte("new binary"))
	srv := serveRelease(t, "v0.2.0", archive, strings.Repeat("0", 64))
	setAPIBase(t, srv.URL)
	exe := fakeExe(t)
	err := run(&bytes.Buffer{}, "", exe, "dev")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v, want checksum mismatch", err)
	}
	got, readErr := os.ReadFile(exe)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old binary" {
		t.Errorf("executable changed after a checksum mismatch")
	}
}

func TestRunTagFlag(t *testing.T) {
	archive, digest := buildArchive(t, []byte("new binary"))
	srv := serveRelease(t, "v0.1.5", archive, digest)
	setAPIBase(t, srv.URL)
	exe := fakeExe(t)
	if err := run(&bytes.Buffer{}, "v0.1.5", exe, "dev"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Errorf("executable = %q, want the released binary", got)
	}
}

// setAPIBase points the release API at a test server and restores the
// real one afterwards, keeping tests order-independent.
func setAPIBase(t *testing.T, url string) {
	t.Helper()
	old := apiBase
	apiBase = url
	t.Cleanup(func() { apiBase = old })
}
