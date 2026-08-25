# plasticity

`plst` is a platform for agent tooling. It manages modules, loads them, and runs
their subcommands. That is all it does.

Everything you would actually want — hooks, harness, config, agents, skills, the
context debugger — is a module, and a module is an executable named
`plst-<name>`.

## Install

macOS or Linux, on Intel or ARM. You need `curl` or `wget`, and `tar`. You do not
need Go.

### 1. Run the installer

```sh
curl -fsSL https://raw.githubusercontent.com/esmarkowski/plasticity/main/install.sh | sh
```

It works out your platform, downloads the matching release, puts `plst` in
`~/.local/bin`, and then installs the `harness` module so there is something to
run. Modules live in `~/.plasticity/modules` — only `plst` itself needs to be on
your PATH.

If you would rather read it before running it — reasonable — it is one file:

```sh
curl -fsSL https://raw.githubusercontent.com/esmarkowski/plasticity/main/install.sh -o install.sh
less install.sh
sh install.sh
```

### 2. Put it on your PATH

The installer tells you if `~/.local/bin` is not already there. If it is not, add
it to your shell's startup file — `~/.zshrc` for zsh, `~/.bashrc` for bash:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Then open a new terminal, or `source` that file.

### 3. Check it worked

```sh
plst
```

You should see `plst`'s own commands, and one module:

```
MODULES
  harness  interchangeable sets of agent configuration
```

If `plst: command not found`, step 2 has not taken effect yet.

### 4. Set up a harness

A harness is your agent's configuration — instructions, rules, agents, skills,
hooks — kept in one directory you can swap.

```sh
plst harness new mine      # scaffold one in ~/.plasticity/harnesses/mine
plst harness use mine      # link it into ~/.claude
plst harness list          # what is installed, and what is applied
```

Anything already in `~/.claude` is **moved aside, not deleted**, and
`plst harness off` puts it back exactly as it was.

Be aware that `new` scaffolds a placeholder `CLAUDE.md`, so applying a fresh
harness does replace the instructions you had — your originals are safe in
`~/.plasticity/harnesses/.parked/`, and `off` restores them, but the agent reads
the placeholder until you fill it in. If you want to keep what you already have,
copy it in first:

```sh
plst harness new mine
cp -R ~/.claude/CLAUDE.md ~/.claude/agents ~/.plasticity/harnesses/mine/
plst harness use mine
```

### 5. Add other modules

```sh
plst install esmarkowski/plasticity-claude-sidecar
plst sidecar start         # see where the context window went
```

`plst install` takes any `owner/repo` that builds a `plst-*` binary. It downloads
a release when the repo has one and clones and builds when it does not — the
second path needs a Go toolchain.

### Options

The installer reads three environment variables, so it can be pointed elsewhere
without being edited:

| | |
|---|---|
| `PLST_BINDIR` | where the binaries go. Default `~/.local/bin` |
| `PLST_MODULES` | modules to install, space separated. Default `esmarkowski/plasticity-modules` |
| `PLST_REPO` | where to fetch `plst` from |

```sh
curl -fsSL .../install.sh | PLST_BINDIR=/usr/local/bin sh
```

### From source

If you have Go and would rather build it:

```sh
go install github.com/esmarkowski/plasticity/cmd/plst@latest
plst install esmarkowski/plasticity-modules
```

### Uninstalling

```sh
plst harness off        # restores whatever the harness displaced
rm ~/.local/bin/plst
rm -rf ~/.plasticity    # harnesses, modules, config, cache
```

Run `plst harness off` first. Without it, `~/.claude` is left holding symlinks
into a directory that is about to stop existing, and the files the harness moved
aside stay in `~/.plasticity/harnesses/.parked/` — which the last line then
deletes.

## Why a binary and not a plugin

The module contract is the one `git`, `kubectl`, `gh`, and `docker` all use, for
the same three reasons.

A module can be written in anything that produces a binary. Go, Rust, a shell
script — `plst` never links against it, so it never constrains it.

A module owns the terminal. `plst` hands over the process rather than proxying
stdio, so a full-screen [charm](https://charm.sh) UI needs no cooperation from
the host at all — no signal forwarding, no pty, nothing to get wrong.

A module cannot take `plst` down with it. It is a separate process with its own
exit code.

The alternatives were considered and rejected. Go's `plugin.Open` needs the host
and the plugin built with an identical toolchain and identical dependency
versions, has no Windows support, and rules out every other language. A gRPC
plugin protocol is the right answer when the host needs to call *into* a module,
and this host only needs to hand it arguments.

## Running a module

```
plst <module> <args...>
```

Where the platform allows it, `plst` replaces its own process with the module's
rather than spawning a child. `plst` sits in front of hook targets that fire on
every turn of an agent session, so a process it does not need to keep is a
process it should not pay for.

Measured on an M-series Mac: `plst sidecar emit` costs 7.1ms against 3.3ms for
running the module binary directly. Both are fine for a person; for a hook that
fires twenty times a turn, a module that registers a hook should register its own
binary. `plst` tells each module where it lives so it can.

## Managing modules

```
plst install <owner>/<repo>[@ref]   install a module
plst update <module>                reinstall from where it came from
plst remove <module>                uninstall
plst modules                        what is installed, and what it offers
plst where                          the paths plst is using
```

`install` tries the repo's latest release asset for your os and arch first, since
that needs no toolchain. Asset names are matched on any common spelling of the
platform — `darwin_arm64`, `Darwin-arm64`, `macos-aarch64` — because release
naming is a matter of taste and every build tool has its own.

With no matching asset it clones and builds. A repo laying its commands out under
`cmd/plst-<name>` needs to say nothing; anything else declares itself:

```json
{
  "modules": [
    { "name": "rusty", "build": "cargo build --release && cp target/release/thing {{out}}" }
  ]
}
```

`{{out}}` is where the binary must end up. The module says how, `plst` says
where.

## Writing a module

Two things. Be an executable named `plst-<name>`, and dispatch on `os.Args[1]`.

Optionally answer `--plst-manifest` with JSON so `plst` can describe you without
having to run you:

```json
{
  "name": "sidecar",
  "description": "agent context debugger",
  "version": "1.0.0",
  "commands": [{ "name": "start", "description": "open the dashboard" }]
}
```

A module that answers nothing is still perfectly runnable — `plst` dispatches by
name and does not need to understand a module to hand it its arguments. The
manifest only buys a better `plst` with no arguments. It is cached against the
binary's size and mtime, so a rebuild invalidates it without `plst` being told.

## No absolute paths

Nothing here hardcodes a location. `plst` resolves its own state directory —
`PLST_HOME`, then `XDG_CONFIG_HOME/plasticity`, then `~/.plasticity` — and hands
every module the answer rather than letting it work it out again:

| | |
|---|---|
| `PLST_HOME` | root of plst's state |
| `PLST_MODULE_DIR` | where module binaries live |
| `PLST_CONFIG` | the config file |
| `PLST_TOOLS` | which agent harnesses are in play, comma separated |
| `PLST_BIN` | the `plst` that invoked this module |

A module that derives its own paths is a second implementation of config that
will disagree with the first the moment either changes.

## Modules

| module | repo | ships with plst |
|---|---|---|
| `harness` | [plasticity-modules](https://github.com/esmarkowski/plasticity-modules) | yes, via the installer |
| `sidecar` | [plasticity-claude-sidecar](https://github.com/esmarkowski/plasticity-claude-sidecar) | no |

A harness itself does not ship with anything, and should not: the instructions,
rules, agents and hooks in it are yours, and swapping them is the point.

## Releasing

Each repo releases its own binaries. Tag it and push the tag, and GoReleaser
builds darwin and linux for amd64 and arm64 and publishes the archives.

`plst install` looks for a release asset for the running platform before it
considers building from source, so a module with a release installs as a download
and a module without one still installs.

The host's release deliberately does not build the modules. Doing so would make
this repo's release depend on another repo's main branch, where a broken commit
would break the ability to install `plst` at all. `install.sh` installs the
default modules as a second step instead — the same one command for anyone using
it, and no coupling here.
