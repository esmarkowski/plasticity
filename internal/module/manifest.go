package module

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/esmarkowski/plasticity/internal/config"
)

// ManifestFlag is how plst asks a module what it offers.
//
// A flag rather than a subcommand, so it cannot collide with anything a module
// wants to call its own commands, and so a module that has not implemented it
// fails cleanly instead of doing something unintended.
const ManifestFlag = "--plst-manifest"

// Manifest is a module's own account of itself, for help output.
//
// Optional throughout. A module that answers nothing is still perfectly
// runnable — plst dispatches by name and does not need to understand a module to
// hand it its arguments. The manifest only buys a better `plst` with no
// arguments.
type Manifest struct {
	Name        string    `json:"name,omitempty"`
	Description string    `json:"description,omitempty"`
	Version     string    `json:"version,omitempty"`
	Commands    []Command `json:"commands,omitempty"`
}

// Command is one subcommand a module offers.
type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// manifestTimeout bounds the ask. A module is asked for its manifest to draw a
// help screen, and a module that hangs must cost the help screen and not the
// session.
const manifestTimeout = 2 * time.Second

// Describe fills in a module's manifest, from cache where possible.
//
// Cached because `plst` with no arguments would otherwise exec every installed
// module before printing anything, and the answer only changes when the binary
// does. Keyed on size and mtime, so a rebuild invalidates it without plst having
// to be told.
func Describe(cfg config.Config, mods []Module) []Module {
	cache := loadCache(cfg)
	dirty := false
	for i := range mods {
		key, stamp, ok := stampOf(mods[i].Path)
		if !ok {
			continue
		}
		if hit, ok := cache[key]; ok && hit.Stamp == stamp {
			mods[i].Manifest = hit.Manifest
			continue
		}
		mods[i].Manifest = ask(mods[i].Path)
		cache[key] = entry{Stamp: stamp, Manifest: mods[i].Manifest}
		dirty = true
	}
	if dirty {
		saveCache(cfg, cache)
	}
	return mods
}

// ask runs the module and reads its manifest. Any failure is an empty manifest,
// never an error: not answering is allowed.
func ask(path string) Manifest {
	cmd := exec.Command(path, ManifestFlag)
	// No stdin and no inherited stderr. A module asked to describe itself has no
	// business prompting, and its diagnostics do not belong on a help screen.
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return Manifest{}
	}
	var m Manifest
	if json.Unmarshal(out, &m) != nil {
		return Manifest{}
	}
	return m
}

type entry struct {
	Stamp    string   `json:"stamp"`
	Manifest Manifest `json:"manifest"`
}

func stampOf(path string) (key, stamp string, ok bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", "", false
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8]),
		fmt.Sprintf("%d-%d", fi.Size(), fi.ModTime().UnixNano()), true
}

func cachePath(cfg config.Config) string {
	return filepath.Join(cfg.Cache(), "manifests.json")
}

func loadCache(cfg config.Config) map[string]entry {
	out := map[string]entry{}
	if b, err := os.ReadFile(cachePath(cfg)); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// saveCache is best effort. A cache that cannot be written costs a few
// milliseconds next time and nothing else, so it must not be reported as a
// failure of the command the user actually ran.
func saveCache(cfg config.Config, c map[string]entry) {
	if os.MkdirAll(cfg.Cache(), 0o700) != nil {
		return
	}
	if b, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(cachePath(cfg), b, 0o600)
	}
}
