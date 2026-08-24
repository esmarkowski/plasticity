// Package config is where plst keeps its own settings, and the only place that
// knows a filesystem path.
//
// Nothing in plst hardcodes a location. A module is found through this, a module
// is installed through this, and a module is told where things are through the
// environment rather than having to work it out again.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
)

// Config is the whole of plst's own state. Deliberately small: plst manages
// modules and runs them, so the only thing it needs to know is where they are
// and which agent harnesses the modules should be acting on.
type Config struct {
	// ModuleDir is where installed module binaries live. Relative paths are
	// resolved against Home, so a config file stays portable between machines.
	ModuleDir string `json:"module_dir,omitempty"`
	// Tools names the LLM harnesses in play — claude, pi — for the modules that
	// act on one. Modules read it rather than each inventing its own answer to
	// "which agent am I configuring".
	Tools []string `json:"tools,omitempty"`

	home string
}

// Env names the variables a module is handed. A module should read these rather
// than deriving paths of its own: plst is the one that decides where things go,
// and a module that guesses will be wrong the moment the config changes.
const (
	EnvHome      = "PLST_HOME"
	EnvModuleDir = "PLST_MODULE_DIR"
	EnvConfig    = "PLST_CONFIG"
	EnvTools     = "PLST_TOOLS"
	EnvBin       = "PLST_BIN"
)

// Home is the root of plst's own state.
//
// PLST_HOME wins so a test, or a second install, can be pointed somewhere else
// without touching the real one. Otherwise XDG if the user has opted into it,
// and ~/.plasticity if not — which is where the config file already lives for
// anyone who used the earlier tooling.
func Home() string {
	if h := os.Getenv(EnvHome); h != "" {
		return h
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "plasticity")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Nowhere to put state is survivable for a read; the caller will fail
		// on the write with a better message than a panic here.
		return ".plasticity"
	}
	return filepath.Join(home, ".plasticity")
}

// Path is the config file.
func Path() string {
	if p := os.Getenv(EnvConfig); p != "" {
		return p
	}
	return filepath.Join(Home(), "config.json")
}

// Load reads the config, falling back to defaults.
//
// A missing or unreadable config is not an error: plst has to work on a machine
// it has never run on, and every field has a usable default. A corrupt one is
// also not fatal — the defaults are better than refusing to start.
func Load() Config {
	c := Config{home: Home()}
	if b, err := os.ReadFile(Path()); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	c.home = Home()
	return c
}

// Save writes the config, creating its directory.
func (c Config) Save() error {
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), append(b, '\n'), 0o600)
}

// Modules is where module binaries are installed and looked for.
func (c Config) Modules() string {
	if d := os.Getenv(EnvModuleDir); d != "" {
		return d
	}
	if c.ModuleDir == "" {
		return filepath.Join(c.home, "modules")
	}
	if filepath.IsAbs(c.ModuleDir) {
		return c.ModuleDir
	}
	return filepath.Join(c.home, c.ModuleDir)
}

// Cache is where derived data that can be thrown away lives.
func (c Config) Cache() string { return filepath.Join(c.home, "cache") }

// Home is the root of plst's state.
func (c Config) Home() string { return c.home }

// Platform is the os/arch pair a release asset has to match.
func Platform() (string, string) { return runtime.GOOS, runtime.GOARCH }
