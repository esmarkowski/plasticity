package module

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/esmarkowski/plasticity/internal/config"
)

// Run hands a module its arguments and gets out of the way.
//
// Where the platform allows it this replaces the plst process rather than
// spawning a child. Two reasons, and the first is the one that matters: plst sits
// in front of hook targets that fire on every turn of an agent session, and a
// process it does not need to keep is a process it should not pay for. The second
// is that a module with a full screen UI then owns the terminal outright — no
// signal forwarding, no proxied stdio, no exit code to translate.
func Run(cfg config.Config, m Module, args []string) error {
	argv := append([]string{m.Path}, args...)
	env := environ(cfg)

	if runtime.GOOS != "windows" {
		// Returns only on failure: on success this process is gone.
		if err := syscall.Exec(m.Path, argv, env); err != nil {
			return fmt.Errorf("run %s: %w", m.Name, err)
		}
	}

	cmd := exec.Command(m.Path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		var ex *exec.ExitError
		if errors.As(err, &ex) {
			os.Exit(ex.ExitCode())
		}
		return fmt.Errorf("run %s: %w", m.Name, err)
	}
	return nil
}

// environ is the module's environment: everything plst was given, plus where
// plst keeps things.
//
// Passed rather than left to be rediscovered. A module that works out its own
// paths is a second implementation of config that will disagree with the first
// the moment either changes.
func environ(cfg config.Config) []string {
	out := make([]string, 0, len(os.Environ())+5)
	for _, kv := range os.Environ() {
		if !reserved(kv) {
			out = append(out, kv)
		}
	}
	self, _ := os.Executable()
	out = append(out,
		config.EnvHome+"="+cfg.Home(),
		config.EnvModuleDir+"="+cfg.Modules(),
		config.EnvConfig+"="+config.Path(),
		config.EnvTools+"="+joinTools(cfg.Tools),
		config.EnvBin+"="+self,
	)
	return out
}

func reserved(kv string) bool {
	for _, k := range []string{config.EnvHome, config.EnvModuleDir, config.EnvConfig,
		config.EnvTools, config.EnvBin} {
		if len(kv) > len(k) && kv[:len(k)+1] == k+"=" {
			return true
		}
	}
	return false
}

func joinTools(tools []string) string {
	out := ""
	for i, t := range tools {
		if i > 0 {
			out += ","
		}
		out += t
	}
	return out
}
