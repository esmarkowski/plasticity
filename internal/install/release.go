package install

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/esmarkowski/plasticity/internal/module"
)

// release is the part of a GitHub release plst needs.
type release struct {
	Tag    string `json:"tag_name"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// httpTimeout bounds a fetch. An install that hangs forever is worse than one
// that fails and can be retried.
const httpTimeout = 60 * time.Second

// latestRelease asks GitHub for a repo's newest release.
func latestRelease(owner, repo string) (release, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	// A token if one is around, purely for the rate limit and for private repos.
	// Unauthenticated is the normal case and has to keep working.
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return release{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return release{}, errNoRelease
	}
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("github returned %s", resp.Status)
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return release{}, err
	}
	return r, nil
}

func githubToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

// assetFor picks the asset built for this machine.
//
// Matched loosely on purpose. Release naming is a matter of taste and every
// build tool has its own — darwin_arm64, Darwin-arm64, macos-aarch64 — so the
// os and the arch are looked for by any of their common spellings rather than
// against one expected filename.
func assetFor(r release, goos, goarch string) (name, url string, ok bool) {
	for _, a := range r.Assets {
		low := strings.ToLower(a.Name)
		if hasAny(low, osNames(goos)) && hasAny(low, archNames(goarch)) {
			return a.Name, a.URL, true
		}
	}
	return "", "", false
}

func osNames(goos string) []string {
	switch goos {
	case "darwin":
		return []string{"darwin", "macos", "apple", "osx"}
	case "linux":
		return []string{"linux"}
	case "windows":
		return []string{"windows", "win"}
	}
	return []string{goos}
}

func archNames(goarch string) []string {
	switch goarch {
	case "arm64":
		return []string{"arm64", "aarch64"}
	case "amd64":
		return []string{"amd64", "x86_64", "x64"}
	}
	return []string{goarch}
}

func hasAny(s string, options []string) bool {
	for _, o := range options {
		if strings.Contains(s, o) {
			return true
		}
	}
	return false
}

// fetchAsset downloads an asset into dir and returns the file it wrote.
func fetchAsset(name, url, dir string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if tok := githubToken(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", name, resp.Status)
	}
	// filepath.Base, because the name comes off the network and must not be able
	// to write outside the directory it was given.
	path := filepath.Join(dir, filepath.Base(name))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

// unpack finds the module binaries in a downloaded asset.
//
// An asset is a tarball, a zip, or the bare binary. All three are common enough
// that refusing two of them would just mean modules that cannot be installed.
func unpack(path, dir string) ([]string, error) {
	switch {
	case strings.HasSuffix(path, ".tar.gz"), strings.HasSuffix(path, ".tgz"):
		return untar(path, dir)
	case strings.HasSuffix(path, ".zip"):
		return unzip(path, dir)
	default:
		// A bare binary, whatever it is called: rename it to the module it is.
		return []string{path}, nil
	}
}

func untar(path, dir string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	var out []string
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if h.Typeflag != tar.TypeReg || !isModuleName(h.Name) {
			continue
		}
		dest, err := writeFile(filepath.Join(dir, filepath.Base(h.Name)), tr)
		if err != nil {
			return nil, err
		}
		out = append(out, dest)
	}
	return out, nil
}

func unzip(path, dir string) ([]string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var out []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !isModuleName(f.Name) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		dest, err := writeFile(filepath.Join(dir, filepath.Base(f.Name)), rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out = append(out, dest)
	}
	return out, nil
}

// isModuleName keeps the prefix rule doing the work: an archive may carry a
// licence, a readme, and completions, and only the plst-<name> entries are
// modules.
func isModuleName(name string) bool {
	return strings.HasPrefix(filepath.Base(name), module.Prefix)
}

func writeFile(path string, r io.Reader) (string, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return "", err
	}
	return path, nil
}
