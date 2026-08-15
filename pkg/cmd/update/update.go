// Package update swaps the running binary for a released one. It
// resolves the release through the GitHub API, verifies the archive
// against the release's checksums.txt, and renames the extracted
// binary over the current executable.
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/maxgio92/telescreen/pkg/cmd/version"
)

// apiBase is the release API root; tests point it at a local server.
var apiBase = "https://api.github.com/repos/maxgio92/telescreen"

// maxBinarySize caps the extracted binary at 200 MiB.
const maxBinarySize = 200 << 20

// New returns the update subcommand.
func New() *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "swap this binary for a released one",
		Long: `update downloads a telescreen release archive, verifies its sha256
against the release's checksums.txt, and replaces the current
executable in place. Without --tag it targets the latest release.
It does not touch the enrolled skills or units; run
telescreen install --force afterwards when the release notes say
the shipped skills changed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			exe, err = filepath.EvalSymlinks(exe)
			if err != nil {
				return err
			}
			return run(cmd.OutOrStdout(), tag, exe, version.Version())
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "release tag to install (default: latest)")
	return cmd
}

// release is the slice of the GitHub release payload update reads.
type release struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// run resolves the release, verifies the archive, and swaps exe.
func run(out io.Writer, tag, exe, current string) error {
	rel, err := resolve(tag)
	if err != nil {
		return err
	}
	relVersion := strings.TrimPrefix(rel.TagName, "v")
	if current != "dev" && relVersion == current {
		_, _ = fmt.Fprintf(out, "telescreen %s is already the release version\n", current)
		return nil
	}

	suffix := fmt.Sprintf("_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	var archiveURL, archiveName, sumsURL string
	for _, a := range rel.Assets {
		switch {
		case strings.HasSuffix(a.Name, suffix):
			archiveURL, archiveName = a.URL, a.Name
		case a.Name == "checksums.txt":
			sumsURL = a.URL
		}
	}
	if archiveURL == "" {
		return fmt.Errorf("release %s has no asset for %s/%s", rel.TagName, runtime.GOOS, runtime.GOARCH)
	}
	if sumsURL == "" {
		return fmt.Errorf("release %s has no checksums.txt", rel.TagName)
	}

	archive, err := fetch(archiveURL)
	if err != nil {
		return err
	}
	sums, err := fetch(sumsURL)
	if err != nil {
		return err
	}
	if err := verify(archive, sums, archiveName); err != nil {
		return err
	}

	binary, err := extract(archive)
	if err != nil {
		return err
	}
	if err := swap(exe, binary); err != nil {
		return permissionHint(exe, err)
	}
	_, _ = fmt.Fprintf(out, "updated telescreen %s -> %s\n", current, relVersion)
	_, _ = fmt.Fprintln(out, "run telescreen install --force to refresh the shipped skills when the release notes say they changed")
	return nil
}

// resolve fetches the release metadata for tag, or the latest release
// when tag is empty.
func resolve(tag string) (*release, error) {
	url := apiBase + "/releases/latest"
	if tag != "" {
		url = apiBase + "/releases/tags/" + tag
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		if tag == "" {
			return nil, errors.New("no release published yet")
		}
		return nil, fmt.Errorf("release %s not found", tag)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("release lookup: %s", resp.Status)
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

// fetch downloads url into memory.
func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBinarySize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxBinarySize {
		return nil, fmt.Errorf("download %s: larger than the %d byte cap", url, maxBinarySize)
	}
	return b, nil
}

// verify checks the archive's sha256 against its checksums.txt line.
func verify(archive, sums []byte, name string) error {
	sum := sha256.Sum256(archive)
	got := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name {
			if fields[0] == got {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s: archive is %s, checksums.txt says %s", name, got, fields[0])
		}
	}
	return fmt.Errorf("checksums.txt has no line for %s", name)
}

// extract returns the telescreen member of the tar.gz archive.
func extract(archive []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || path.Base(hdr.Name) != "telescreen" {
			continue
		}
		if hdr.Size > maxBinarySize {
			return nil, fmt.Errorf("archive member telescreen is %d bytes, cap is %d", hdr.Size, maxBinarySize)
		}
		return io.ReadAll(io.LimitReader(tr, maxBinarySize))
	}
	return nil, errors.New("archive has no telescreen binary")
}

// swap writes binary next to exe and renames it over exe, keeping
// exe's mode. The rename is atomic on the same filesystem.
func swap(exe string, binary []byte) error {
	info, err := os.Stat(exe)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(exe), ".telescreen-update-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(binary); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace %s: %w", exe, err)
	}
	return nil
}

// permissionHint wraps a swap error on a path the user cannot write:
// a root-owned /usr/bin/telescreen came from a package, and the
// package manager owns its upgrades.
func permissionHint(exe string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("%w\n%s is not writable by you; if telescreen came from a package, upgrade it with your package manager", err, exe)
	}
	return err
}
