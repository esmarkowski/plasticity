package install

import "testing"

// A module reference is whatever the user has in their clipboard: the short form,
// the URL from the address bar, or the one git prints.
func TestParseAcceptsEveryWayARepoIsWritten(t *testing.T) {
	cases := map[string][3]string{
		"esmarkowski/plasticity-claude-sidecar":                    {"esmarkowski", "plasticity-claude-sidecar", ""},
		"https://github.com/esmarkowski/plasticity-claude-sidecar": {"esmarkowski", "plasticity-claude-sidecar", ""},
		"github.com/owner/repo":                                    {"owner", "repo", ""},
		"git@github.com:owner/repo.git":                            {"owner", "repo", ""},
		"https://github.com/owner/repo.git":                        {"owner", "repo", ""},
		"owner/repo@v1.2.0":                                        {"owner", "repo", "v1.2.0"},
		"https://github.com/owner/repo@main":                       {"owner", "repo", "main"},
	}
	for in, want := range cases {
		owner, repo, ref, err := parse(in)
		if err != nil {
			t.Errorf("parse(%q): %v", in, err)
			continue
		}
		if owner != want[0] || repo != want[1] || ref != want[2] {
			t.Errorf("parse(%q) = %q %q %q, want %q", in, owner, repo, ref, want)
		}
	}
	for _, bad := range []string{"", "repo", "a/b/c", "/", "owner/"} {
		if _, _, _, err := parse(bad); err == nil {
			t.Errorf("parse(%q) was accepted", bad)
		}
	}
}

// Release naming is a matter of taste and every build tool has its own, so the
// os and arch are looked for by any of their common spellings.
func TestAssetForMatchesTheSpellingsInTheWild(t *testing.T) {
	rel := release{Tag: "v1.0.0", Assets: []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}{
		{Name: "plst-sidecar_linux_amd64.tar.gz", URL: "u1"},
		{Name: "plst-sidecar_Darwin_arm64.tar.gz", URL: "u2"},
		{Name: "plst-sidecar-windows-x86_64.zip", URL: "u3"},
	}}

	if name, url, ok := assetFor(rel, "darwin", "arm64"); !ok || url != "u2" {
		t.Errorf("darwin/arm64 matched %q (%q, ok=%v)", name, url, ok)
	}
	if _, url, ok := assetFor(rel, "linux", "amd64"); !ok || url != "u1" {
		t.Errorf("linux/amd64 matched %q", url)
	}
	// macos/aarch64 naming, and x86_64 for amd64.
	alt := release{Assets: []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	}{{Name: "plst-thing-macos-aarch64", URL: "m"}}}
	if _, url, ok := assetFor(alt, "darwin", "arm64"); !ok || url != "m" {
		t.Errorf("macos-aarch64 not matched for darwin/arm64")
	}

	// No asset for this machine is not an error to guess around — it is the
	// signal to build from source.
	if _, _, ok := assetFor(rel, "linux", "arm64"); ok {
		t.Error("matched an asset that was not built for this platform")
	}
}

// Only plst-* entries in an archive are modules. An archive may also carry a
// licence, a readme, and shell completions.
func TestIsModuleName(t *testing.T) {
	for _, yes := range []string{"plst-sidecar", "dist/plst-hooks", "./plst-x"} {
		if !isModuleName(yes) {
			t.Errorf("%q was not recognised as a module", yes)
		}
	}
	for _, no := range []string{"LICENSE", "README.md", "completions/plst.bash", "sidecar"} {
		if isModuleName(no) {
			t.Errorf("%q was taken for a module", no)
		}
	}
}
