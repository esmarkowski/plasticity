package install

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/esmarkowski/plasticity/internal/config"
	"github.com/esmarkowski/plasticity/internal/module"
)

// errNoRelease says a repo has published no release, which is not a failure —
// it is the ordinary state of a repo being worked on, and the reason installing
// falls back to building.
var errNoRelease = errors.New("no published release")

// Record is what plst remembers about an installed module, so it can be updated
// or removed later without being asked again where it came from.
type Record struct {
	Name      string    `json:"name"`
	Source    string    `json:"source"`
	Ref       string    `json:"ref,omitempty"`
	Version   string    `json:"version,omitempty"`
	From      string    `json:"from"` // "release" or "source"
	Binary    string    `json:"binary"`
	Installed time.Time `json:"installed"`
}

// Progress reports what an install is doing. Passed in rather than printed here:
// this package should not have an opinion about how a terminal looks.
type Progress func(string)

// Install puts a module on disk from a repo reference.
//
// A release asset first, because it needs no toolchain and is what most machines
// should use, then a build from source, because a repo that has not cut a release
// yet is exactly the repo you are most likely to be installing.
func Install(cfg config.Config, ref string, say Progress) ([]Record, error) {
	owner, repo, gitRef, err := parse(ref)
	if err != nil {
		return nil, err
	}
	dest := cfg.Modules()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}

	if recs, err := fromRelease(owner, repo, dest, say); err == nil {
		return record(cfg, recs)
	} else if !errors.Is(err, errNoRelease) && !errors.Is(err, errNoAsset) {
		return nil, err
	}

	goos, goarch := config.Platform()
	say(fmt.Sprintf("no release asset for %s/%s — building from source", goos, goarch))
	recs, err := fromSource(owner, repo, gitRef, dest, say)
	if err != nil {
		return nil, err
	}
	return record(cfg, recs)
}

var errNoAsset = errors.New("no matching release asset")

func fromRelease(owner, repo, dest string, say Progress) ([]Record, error) {
	rel, err := latestRelease(owner, repo)
	if err != nil {
		return nil, err
	}
	goos, goarch := config.Platform()
	name, url, ok := assetFor(rel, goos, goarch)
	if !ok {
		return nil, errNoAsset
	}
	say(fmt.Sprintf("%s %s — %s", repo, rel.Tag, name))

	tmp, err := os.MkdirTemp("", "plst-install-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	asset, err := fetchAsset(name, url, tmp)
	if err != nil {
		return nil, err
	}
	bins, err := unpack(asset, tmp)
	if err != nil {
		return nil, err
	}
	if len(bins) == 0 {
		return nil, fmt.Errorf("%s contains no %s* binary", name, module.Prefix)
	}

	var out []Record
	for _, bin := range bins {
		base := filepath.Base(bin)
		if !strings.HasPrefix(base, module.Prefix) {
			// A bare asset named after the release rather than the module. The
			// repo name is the best available answer.
			base = module.Prefix + repo
		}
		final, err := place(bin, filepath.Join(dest, base))
		if err != nil {
			return nil, err
		}
		out = append(out, Record{
			Name:   strings.TrimPrefix(filepath.Base(final), module.Prefix),
			Source: fmt.Sprintf("%s/%s", owner, repo), Version: rel.Tag,
			From: "release", Binary: final, Installed: time.Now(),
		})
	}
	return out, nil
}

func fromSource(owner, repo, gitRef, dest string, say Progress) ([]Record, error) {
	tmp, err := os.MkdirTemp("", "plst-build-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	src := filepath.Join(tmp, "src")
	url := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)
	say("cloning " + url)
	if err := clone(url, src, gitRef); err != nil {
		return nil, fmt.Errorf("clone %s: %w", url, err)
	}
	spec, err := readSpec(src)
	if err != nil {
		return nil, err
	}

	var out []Record
	for _, sm := range spec.Modules {
		say("building " + sm.Name)
		bin, err := build(src, tmp, sm)
		if err != nil {
			return nil, err
		}
		final, err := place(bin, filepath.Join(dest, sm.Binary))
		if err != nil {
			return nil, err
		}
		out = append(out, Record{
			Name: sm.Name, Source: fmt.Sprintf("%s/%s", owner, repo), Ref: gitRef,
			From: "source", Binary: final, Installed: time.Now(),
		})
	}
	return out, nil
}

// place moves a built or downloaded binary into the module directory.
//
// Written beside the destination and renamed, so a module being replaced is
// never a half-written file — the old one keeps working right up until the new
// one is complete. Rename across filesystems fails, hence the copy fallback.
func place(from, to string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return "", err
	}
	staged := to + ".new"
	if err := copyFile(from, staged); err != nil {
		return "", err
	}
	if err := os.Rename(staged, to); err != nil {
		os.Remove(staged)
		return "", err
	}
	return to, nil
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o755)
}

// parse reads a module reference. owner/repo, a full URL, or either with @ref.
func parse(ref string) (owner, repo, gitRef string, err error) {
	// Host prefixes come off first. An scp-style remote carries an @ of its own,
	// and looking for the ref before removing it reads git@github.com:owner/repo
	// as the repo "git" at ref "github.com:owner/repo".
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "github.com/"} {
		ref = strings.TrimPrefix(ref, prefix)
	}
	if at := strings.LastIndex(ref, "@"); at > 0 {
		ref, gitRef = ref[:at], ref[at+1:]
	}
	ref = strings.TrimSuffix(ref, ".git")
	parts := strings.Split(strings.Trim(ref, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", fmt.Errorf("cannot read %q as owner/repo", ref)
	}
	return parts[0], parts[1], gitRef, nil
}

func recordPath(cfg config.Config) string { return filepath.Join(cfg.Home(), "modules.json") }

// Records is what plst has installed.
func Records(cfg config.Config) map[string]Record {
	out := map[string]Record{}
	if b, err := os.ReadFile(recordPath(cfg)); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func record(cfg config.Config, recs []Record) ([]Record, error) {
	all := Records(cfg)
	for _, r := range recs {
		all[r.Name] = r
	}
	if err := writeRecords(cfg, all); err != nil {
		return nil, err
	}
	return recs, nil
}

func writeRecords(cfg config.Config, all map[string]Record) error {
	if err := os.MkdirAll(cfg.Home(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(recordPath(cfg), append(b, '\n'), 0o600)
}

// Remove uninstalls a module plst installed.
func Remove(cfg config.Config, name string) error {
	m, ok := module.Find(cfg, name)
	if !ok {
		return fmt.Errorf("no module named %q", name)
	}
	if m.OnPath {
		return fmt.Errorf("%q was found on PATH at %s — plst did not install it", name, m.Path)
	}
	if err := os.Remove(m.Path); err != nil {
		return err
	}
	all := Records(cfg)
	delete(all, name)
	return writeRecords(cfg, all)
}

// Update reinstalls a module from the source it came from.
func Update(cfg config.Config, name string, say Progress) ([]Record, error) {
	rec, ok := Records(cfg)[name]
	if !ok {
		return nil, fmt.Errorf("no record of how %q was installed", name)
	}
	ref := rec.Source
	if rec.Ref != "" {
		ref += "@" + rec.Ref
	}
	return Install(cfg, ref, say)
}
