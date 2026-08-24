// Command plst is a platform for agent tooling.
//
// It manages modules, loads them, and runs their subcommands. That is all it
// does: everything a user actually wants — hooks, harness, config, agents,
// skills, the context debugger — is a module, and a module is an executable
// named plst-<name>. Keeping the host this small is what makes a module free to
// be written in anything, to own the terminal for a full screen UI, and to be
// installed and removed without rebuilding plst.
package main

import (
	"fmt"
	"os"

	"github.com/esmarkowski/plasticity/internal/config"
	"github.com/esmarkowski/plasticity/internal/install"
	"github.com/esmarkowski/plasticity/internal/module"
	"github.com/esmarkowski/plasticity/internal/ui"
)

// version is stamped at build time. Unset in a local build, which is honest:
// there is no release to name.
var version = "dev"

// builtins are the commands plst answers itself. Module management, and nothing
// else — a name here is a name no module can use, so the list stays short on
// purpose.
var builtins = []ui.Builtin{
	{Name: "install", Args: "<owner>/<repo>[@ref]", About: "install a module from a repo"},
	{Name: "update", Args: "<module>", About: "reinstall a module from where it came from"},
	{Name: "remove", Args: "<module>", About: "uninstall a module"},
	{Name: "modules", About: "what is installed, and what it offers"},
	{Name: "where", About: "the paths plst is using"},
	{Name: "version", About: "plst's own version"},
}

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	cfg := config.Load()

	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) > 1 {
			// `plst help <module>` is the module's own help, which only the
			// module can write.
			return dispatch(cfg, args[1], []string{"--help"})
		}
		ui.Usage(os.Stdout, builtins, module.Describe(cfg, module.List(cfg)))
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "install":
		return doInstall(cfg, rest)
	case "update":
		return doUpdate(cfg, rest)
	case "remove", "uninstall":
		return doRemove(cfg, rest)
	case "modules", "list":
		records := install.Records(cfg)
		ui.Modules(os.Stdout, module.Describe(cfg, module.List(cfg)), func(name string) string {
			if r, ok := records[name]; ok {
				return r.Source + " (" + r.From + ")"
			}
			return ""
		})
		return 0
	case "where":
		fmt.Printf("home     %s\nmodules  %s\nconfig   %s\ncache    %s\n",
			cfg.Home(), cfg.Modules(), config.Path(), cfg.Cache())
		return 0
	case "version", "--version":
		fmt.Println("plst " + version)
		return 0
	}
	return dispatch(cfg, cmd, rest)
}

// dispatch hands everything else to a module.
func dispatch(cfg config.Config, name string, args []string) int {
	m, ok := module.Find(cfg, name)
	if !ok {
		ui.Unknown(os.Stderr, name, module.List(cfg))
		return 127
	}
	if err := module.Run(cfg, m, args); err != nil {
		ui.Fail(os.Stderr, err)
		return 1
	}
	return 0
}

func doInstall(cfg config.Config, args []string) int {
	if len(args) != 1 {
		ui.Fail(os.Stderr, fmt.Errorf("usage: plst install <owner>/<repo>[@ref]"))
		return 2
	}
	recs, err := install.Install(cfg, args[0], func(s string) { ui.Say(os.Stderr, s) })
	if err != nil {
		ui.Fail(os.Stderr, err)
		return 1
	}
	report(cfg, recs)
	return 0
}

func doUpdate(cfg config.Config, args []string) int {
	if len(args) != 1 {
		ui.Fail(os.Stderr, fmt.Errorf("usage: plst update <module>"))
		return 2
	}
	recs, err := install.Update(cfg, args[0], func(s string) { ui.Say(os.Stderr, s) })
	if err != nil {
		ui.Fail(os.Stderr, err)
		return 1
	}
	report(cfg, recs)
	return 0
}

func doRemove(cfg config.Config, args []string) int {
	if len(args) != 1 {
		ui.Fail(os.Stderr, fmt.Errorf("usage: plst remove <module>"))
		return 2
	}
	if err := install.Remove(cfg, args[0]); err != nil {
		ui.Fail(os.Stderr, err)
		return 1
	}
	ui.Done(os.Stdout, "removed "+args[0])
	return 0
}

// report names what an install actually produced, including the subcommands, so
// the next thing to type is on screen rather than in a manifest somewhere.
func report(cfg config.Config, recs []install.Record) {
	for _, r := range recs {
		m, ok := module.Find(cfg, r.Name)
		if !ok {
			ui.Done(os.Stdout, "installed "+r.Name)
			continue
		}
		mods := module.Describe(cfg, []module.Module{m})
		line := "installed " + r.Name
		if v := r.Version; v != "" {
			line += " " + v
		}
		ui.Done(os.Stdout, line)
		for _, c := range mods[0].Manifest.Commands {
			fmt.Printf("    plst %s %s\n", r.Name, c.Name)
		}
	}
}
