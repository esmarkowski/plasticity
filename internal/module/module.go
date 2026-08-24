// Package module finds installed modules and runs them.
//
// A module is an executable named plst-<name>. That is the whole contract, and
// it is the contract git, kubectl, and gh all use, for the same reasons: a module
// is any language that can produce a binary, it owns its own terminal so a full
// screen UI needs no cooperation from the host, and a module that crashes takes
// nothing else with it.
package module

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/esmarkowski/plasticity/internal/config"
)

// Prefix is what marks an executable as a module.
const Prefix = "plst-"

// Module is one installed module.
type Module struct {
	Name string
	Path string
	// OnPath is true for a module found on PATH rather than in the module
	// directory. Worth distinguishing because plst did not install it and cannot
	// update or remove it.
	OnPath   bool
	Manifest Manifest
}

// Find locates one module by name.
func Find(cfg config.Config, name string) (Module, bool) {
	for _, m := range List(cfg) {
		if m.Name == name {
			return m, true
		}
	}
	return Module{}, false
}

// List is every module available, the module directory first.
//
// PATH is searched as well as the module directory, which is what makes a module
// you are building right now runnable without installing it: put it on PATH and
// plst finds it. The module directory wins on a tie, since that is the copy plst
// was asked to install.
func List(cfg config.Config) []Module {
	seen := map[string]bool{}
	var out []Module

	for _, m := range scan(cfg.Modules(), false) {
		if !seen[m.Name] {
			seen[m.Name] = true
			out = append(out, m)
		}
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" || dir == cfg.Modules() {
			continue
		}
		for _, m := range scan(dir, true) {
			if !seen[m.Name] {
				seen[m.Name] = true
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func scan(dir string, onPath bool) []Module {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Module
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, Prefix) || len(name) == len(Prefix) {
			continue
		}
		path := filepath.Join(dir, name)
		if !executable(path) {
			continue
		}
		out = append(out, Module{Name: strings.TrimPrefix(name, Prefix), Path: path, OnPath: onPath})
	}
	return out
}

// executable reports whether a path is a file anyone can run. A directory named
// plst-something is not a module, and neither is a source file that happens to
// share the prefix.
func executable(path string) bool {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return false
	}
	return fi.Mode().Perm()&0o111 != 0
}
