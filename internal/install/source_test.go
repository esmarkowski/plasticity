package install

import (
	"os"
	"path/filepath"
	"testing"
)

// A Go repo laying its commands out under cmd/plst-<name> needs no spec: that is
// detectable, and requiring a file for the conventional case would mean every
// module carries boilerplate.
func TestInferSpecReadsTheConventionalLayout(t *testing.T) {
	dir := t.TempDir()
	for _, d := range []string{"cmd/plst-sidecar", "cmd/plst-hooks", "cmd/helper", "cmd/plst-x/sub"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	spec, err := readSpec(dir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, m := range spec.Modules {
		found[m.Name] = m.Binary
	}
	if len(found) != 3 {
		t.Fatalf("inferred %d modules, want the three plst-* ones: %v", len(found), found)
	}
	if found["sidecar"] != "plst-sidecar" || found["hooks"] != "plst-hooks" {
		t.Errorf("inferred binaries wrong: %v", found)
	}
	if _, ok := found["helper"]; ok {
		t.Error("cmd/helper was taken for a module")
	}
}

// A repo that says what it builds is believed, which is what lets a module be
// written in something other than Go.
func TestReadSpecPrefersTheRepoOwnAccount(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "plst-inferred"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"modules":[{"name":"rusty","build":"cargo build --release && cp target/release/thing {{out}}"}]}`
	if err := os.WriteFile(filepath.Join(dir, specFile), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	spec, err := readSpec(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Modules) != 1 || spec.Modules[0].Name != "rusty" {
		t.Fatalf("spec was ignored in favour of inference: %+v", spec.Modules)
	}
	// The binary name defaults to the prefix rule, so a spec only has to say it
	// when it differs.
	if got := spec.Modules[0].Binary; got != "plst-rusty" {
		t.Errorf("binary = %q, want plst-rusty", got)
	}
}

// A repo that is not a module repo has to say so clearly rather than install
// nothing and report success.
func TestReadSpecRefusesARepoWithNothingToBuild(t *testing.T) {
	if _, err := readSpec(t.TempDir()); err == nil {
		t.Error("a repo with no cmd/ and no plst.json was accepted")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cmd", "other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := readSpec(dir); err == nil {
		t.Error("a repo with only non-module commands was accepted")
	}
}

// The build command is the module author's, and {{out}} is plst's: the module
// says how, plst says where.
func TestBuildPutsTheBinaryWherePlstAsked(t *testing.T) {
	repo, dest := t.TempDir(), t.TempDir()
	out, err := build(repo, dest, SpecModule{
		Name: "x", Binary: "plst-x", Build: "printf built > {{out}}",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != filepath.Join(dest, "plst-x") {
		t.Errorf("built to %q", out)
	}
	if b, _ := os.ReadFile(out); string(b) != "built" {
		t.Errorf("contents = %q", b)
	}
}

// A build that reports success but produces nothing must not be installed as if
// it had worked.
func TestBuildRequiresTheBinaryToExist(t *testing.T) {
	if _, err := build(t.TempDir(), t.TempDir(), SpecModule{
		Name: "x", Binary: "plst-x", Build: "true",
	}); err == nil {
		t.Error("a build that produced nothing was accepted")
	}
}
