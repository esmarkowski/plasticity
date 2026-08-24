// Package install puts modules on disk and keeps a record of where they came
// from.
package install

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/esmarkowski/plasticity/internal/module"
)

// Spec is a repo's own account of what it builds, read from plst.json at the
// root.
//
// Optional. A Go repo laying its commands out under cmd/plst-<name> needs no
// spec at all — that is detectable. The spec exists so a module written in
// something else can say how to build itself, which is the whole point of a
// module being a binary rather than a plugin.
type Spec struct {
	Modules []SpecModule `json:"modules"`
}

// SpecModule is one binary a repo produces.
type SpecModule struct {
	Name   string `json:"name"`
	Binary string `json:"binary,omitempty"`
	// Build is a shell command run in the repo root. {{out}} is replaced with the
	// path the binary must end up at, so plst chooses the destination and the
	// module only has to say how to get there.
	Build string `json:"build"`
}

const specFile = "plst.json"

// readSpec loads a repo's build spec, or infers one.
func readSpec(dir string) (Spec, error) {
	if b, err := os.ReadFile(filepath.Join(dir, specFile)); err == nil {
		var s Spec
		if err := json.Unmarshal(b, &s); err != nil {
			return Spec{}, fmt.Errorf("%s: %w", specFile, err)
		}
		if len(s.Modules) == 0 {
			return Spec{}, fmt.Errorf("%s declares no modules", specFile)
		}
		for i := range s.Modules {
			if s.Modules[i].Binary == "" {
				s.Modules[i].Binary = module.Prefix + s.Modules[i].Name
			}
		}
		return s, nil
	}
	return inferSpec(dir)
}

// inferSpec works out what a Go repo builds from its layout.
//
// Only cmd/plst-<name>, and deliberately only that: guessing more widely would
// mean building things that are not modules and installing them as if they were.
// Anything with a less conventional layout says so in plst.json.
func inferSpec(dir string) (Spec, error) {
	entries, err := os.ReadDir(filepath.Join(dir, "cmd"))
	if err != nil {
		return Spec{}, fmt.Errorf("no %s, and no cmd/ directory to infer from", specFile)
	}
	var s Spec
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), module.Prefix) {
			continue
		}
		s.Modules = append(s.Modules, SpecModule{
			Name:   strings.TrimPrefix(e.Name(), module.Prefix),
			Binary: e.Name(),
			Build:  "go build -o {{out}} ./cmd/" + e.Name(),
		})
	}
	if len(s.Modules) == 0 {
		return Spec{}, fmt.Errorf("no %s, and nothing under cmd/%s*", specFile, module.Prefix)
	}
	return s, nil
}

// build runs a module's build command and returns where the binary landed.
func build(repo, dest string, sm SpecModule) (string, error) {
	out := filepath.Join(dest, sm.Binary)
	line := strings.ReplaceAll(sm.Build, "{{out}}", out)

	// /bin/sh, because a build command is a command line and this is where the
	// module author's own instructions run. Nothing here is derived from a name
	// plst chose, so the only thing being trusted is the repo, which is the same
	// thing being trusted by building it at all.
	cmd := exec.Command("/bin/sh", "-c", line)
	cmd.Dir = repo
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build %s: %w", sm.Name, err)
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("build %s produced no %s", sm.Name, sm.Binary)
	}
	return out, nil
}

// clone fetches a repo shallowly. History is not wanted: this is a build input,
// not a checkout to work in.
func clone(url, dir, ref string) error {
	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dir)
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}
