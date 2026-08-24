// Package ui is how plst talks to a terminal.
//
// Kept out of the packages that do the work: install and module have no opinion
// about how anything looks, so they can be tested by what they return rather
// than by what they printed.
package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/esmarkowski/plasticity/internal/module"
)

// Colours are AdaptiveColor throughout so plst is legible on a light or dark
// terminal without being configured for one.
var (
	fg    = lipgloss.AdaptiveColor{Light: "#1F2430", Dark: "#E4E6EB"}
	dim   = lipgloss.AdaptiveColor{Light: "#7A8194", Dark: "#767C8C"}
	faint = lipgloss.AdaptiveColor{Light: "#B4BAC8", Dark: "#454A57"}
	acc   = lipgloss.AdaptiveColor{Light: "#4A6FE0", Dark: "#7E9CFF"}

	title = lipgloss.NewStyle().Foreground(fg).Bold(true)
	name  = lipgloss.NewStyle().Foreground(acc).Bold(true)
	desc  = lipgloss.NewStyle().Foreground(dim)
	note  = lipgloss.NewStyle().Foreground(faint)
	bad   = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#B23A34", Dark: "#F0776E"})
	good  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2F7D4F", Dark: "#63C68A"})
)

// Builtin is a command plst answers itself.
type Builtin struct {
	Name  string
	Args  string
	About string
}

// Usage is what `plst` with no arguments prints: what plst does, then what is
// installed.
//
// The modules come second and are the point. plst's own commands manage modules
// and nothing else, so a long list of built-ins would be plst describing itself
// instead of describing what it can do.
func Usage(w io.Writer, builtins []Builtin, mods []module.Module) {
	fmt.Fprintln(w, title.Render("plst")+desc.Render(" — a platform for agent tooling"))
	fmt.Fprintln(w)

	fmt.Fprintln(w, note.Render("MANAGE"))
	width := 0
	for _, b := range builtins {
		if n := len(b.Name) + len(b.Args) + 1; n > width {
			width = n
		}
	}
	for _, b := range builtins {
		label := b.Name
		if b.Args != "" {
			label += " " + b.Args
		}
		fmt.Fprintf(w, "  %s%s\n",
			pad(name.Render(b.Name)+desc.Render(strings.TrimPrefix(label, b.Name)), width+2),
			desc.Render(b.About))
	}

	fmt.Fprintln(w)
	if len(mods) == 0 {
		fmt.Fprintln(w, note.Render("MODULES"))
		fmt.Fprintln(w, "  "+desc.Render("none installed — try ")+
			name.Render("plst install <owner>/<repo>"))
		return
	}
	fmt.Fprintln(w, note.Render("MODULES"))
	width = 0
	for _, m := range mods {
		if len(m.Name) > width {
			width = len(m.Name)
		}
	}
	for _, m := range mods {
		about := m.Manifest.Description
		if about == "" {
			about = "no description"
		}
		line := "  " + pad(name.Render(m.Name), width+2) + desc.Render(about)
		if m.OnPath {
			line += note.Render("  (on PATH)")
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, note.Render("  plst <module> --help for a module's own commands"))
}

// Modules is the long form: every module, where it came from, and what it offers.
func Modules(w io.Writer, mods []module.Module, source func(string) string) {
	if len(mods) == 0 {
		fmt.Fprintln(w, desc.Render("no modules installed"))
		return
	}
	for i, m := range mods {
		if i > 0 {
			fmt.Fprintln(w)
		}
		head := name.Render(m.Name)
		if v := m.Manifest.Version; v != "" {
			head += note.Render(" " + v)
		}
		fmt.Fprintln(w, head)
		if d := m.Manifest.Description; d != "" {
			fmt.Fprintln(w, "  "+desc.Render(d))
		}
		if s := source(m.Name); s != "" {
			fmt.Fprintln(w, "  "+note.Render(s))
		}
		fmt.Fprintln(w, "  "+note.Render(m.Path))
		if len(m.Manifest.Commands) == 0 {
			continue
		}
		width := 0
		for _, c := range m.Manifest.Commands {
			if len(c.Name) > width {
				width = len(c.Name)
			}
		}
		for _, c := range m.Manifest.Commands {
			fmt.Fprintf(w, "    %s%s\n", pad(desc.Render(c.Name), width+2), note.Render(c.Description))
		}
	}
}

// Say is progress: what a long-running command is doing right now.
func Say(w io.Writer, s string) { fmt.Fprintln(w, note.Render("  "+s)) }

// Done reports a success worth confirming.
func Done(w io.Writer, s string) { fmt.Fprintln(w, good.Render("✓ ")+s) }

// Fail reports a failure. Returned to the caller as well, so the exit code and
// the message never disagree.
func Fail(w io.Writer, err error) { fmt.Fprintln(w, bad.Render("✗ ")+err.Error()) }

// Unknown is the one error worth spending real estate on: a name that is not a
// module is usually a module that is not installed yet.
func Unknown(w io.Writer, cmd string, mods []module.Module) {
	fmt.Fprintln(w, bad.Render("✗ ")+fmt.Sprintf("no built-in command or module named %q", cmd))
	if len(mods) > 0 {
		var names []string
		for _, m := range mods {
			names = append(names, m.Name)
		}
		fmt.Fprintln(w, desc.Render("  installed: ")+strings.Join(names, ", "))
	}
	fmt.Fprintln(w, desc.Render("  install one with ")+name.Render("plst install <owner>/<repo>"))
}

// pad right-pads to a display width, measured with lipgloss so styled content
// lines up.
func pad(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}
