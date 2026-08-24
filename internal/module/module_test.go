package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/esmarkowski/plasticity/internal/config"
)

// scratch points plst's whole state at a temporary directory, so a test can
// install modules without touching the real one.
func scratch(t *testing.T) config.Config {
	t.Helper()
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	t.Setenv(config.EnvConfig, filepath.Join(home, "config.json"))
	t.Setenv(config.EnvModuleDir, "")
	os.Unsetenv(config.EnvModuleDir)
	return config.Load()
}

func writeModule(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// The contract is the whole of the contract: an executable named plst-<name>.
func TestListFindsExecutablesByPrefix(t *testing.T) {
	cfg := scratch(t)
	dir := cfg.Modules()
	writeModule(t, dir, "plst-sidecar", "#!/bin/sh\nexit 0\n")
	writeModule(t, dir, "plst-hooks", "#!/bin/sh\nexit 0\n")

	// Not modules: the wrong prefix, the prefix alone, a directory, and a file
	// nobody can run.
	writeModule(t, dir, "sidecar", "#!/bin/sh\n")
	writeModule(t, dir, "plst-", "#!/bin/sh\n")
	if err := os.MkdirAll(filepath.Join(dir, "plst-adirectory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plst-notexec"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := List(cfg)
	if len(got) != 2 {
		t.Fatalf("found %d modules, want 2: %+v", len(got), got)
	}
	// Sorted, so help output does not reorder itself between runs.
	if got[0].Name != "hooks" || got[1].Name != "sidecar" {
		t.Errorf("names = %q %q", got[0].Name, got[1].Name)
	}
	if _, ok := Find(cfg, "sidecar"); !ok {
		t.Error("Find missed a module List returned")
	}
	if _, ok := Find(cfg, "nope"); ok {
		t.Error("Find invented a module")
	}
}

// A module on PATH is runnable without being installed, which is what makes a
// module you are building right now usable. The installed copy wins, since that
// is the one plst was asked for.
func TestModuleDirectoryWinsOverPath(t *testing.T) {
	cfg := scratch(t)
	elsewhere := t.TempDir()
	writeModule(t, elsewhere, "plst-sidecar", "#!/bin/sh\nexit 0\n")
	writeModule(t, elsewhere, "plst-onlyhere", "#!/bin/sh\nexit 0\n")
	installed := writeModule(t, cfg.Modules(), "plst-sidecar", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", elsewhere)

	byName := map[string]Module{}
	for _, m := range List(cfg) {
		byName[m.Name] = m
	}
	if got := byName["sidecar"].Path; got != installed {
		t.Errorf("sidecar resolved to %q, want the installed copy %q", got, installed)
	}
	if byName["sidecar"].OnPath {
		t.Error("the installed copy was reported as being on PATH")
	}
	if m, ok := byName["onlyhere"]; !ok || !m.OnPath {
		t.Errorf("a PATH-only module was missed or mislabelled: %+v", m)
	}
}

// A module that answers the manifest flag gets described; one that does not is
// still perfectly runnable. Not answering is allowed.
func TestDescribeToleratesSilence(t *testing.T) {
	cfg := scratch(t)
	dir := cfg.Modules()
	writeModule(t, dir, "plst-talker", `#!/bin/sh
[ "$1" = "--plst-manifest" ] && echo '{"name":"talker","description":"says things","commands":[{"name":"go","description":"do it"}]}'
exit 0
`)
	writeModule(t, dir, "plst-mute", "#!/bin/sh\nexit 0\n")
	writeModule(t, dir, "plst-garbage", "#!/bin/sh\necho not json\nexit 0\n")

	byName := map[string]Module{}
	for _, m := range Describe(cfg, List(cfg)) {
		byName[m.Name] = m
	}
	if got := byName["talker"].Manifest.Description; got != "says things" {
		t.Errorf("description = %q", got)
	}
	if cmds := byName["talker"].Manifest.Commands; len(cmds) != 1 || cmds[0].Name != "go" {
		t.Errorf("commands = %+v", cmds)
	}
	if byName["mute"].Manifest.Name != "" {
		t.Error("a silent module was given a manifest")
	}
	if byName["garbage"].Manifest.Name != "" {
		t.Error("unparseable output was accepted as a manifest")
	}
}

// Described once, then remembered: `plst` with no arguments would otherwise exec
// every installed module before printing anything.
func TestDescribeCachesUntilTheBinaryChanges(t *testing.T) {
	cfg := scratch(t)
	dir := cfg.Modules()
	counter := filepath.Join(t.TempDir(), "asked")
	path := writeModule(t, dir, "plst-counted", `#!/bin/sh
echo x >> `+counter+`
echo '{"name":"counted","description":"first"}'
`)

	for range 3 {
		Describe(cfg, List(cfg))
	}
	if n := lines(t, counter); n != 1 {
		t.Errorf("module was asked %d times, want once", n)
	}

	// A rebuild has to invalidate it without plst being told.
	if err := os.WriteFile(path, []byte(`#!/bin/sh
echo x >> `+counter+`
echo '{"name":"counted","description":"second"}'
`), 0o755); err != nil {
		t.Fatal(err)
	}
	got := Describe(cfg, List(cfg))
	if got[0].Manifest.Description != "second" {
		t.Errorf("a rebuilt module kept its old manifest: %q", got[0].Manifest.Description)
	}
	if n := lines(t, counter); n != 2 {
		t.Errorf("module was asked %d times after a rebuild, want twice", n)
	}
}

func lines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}
