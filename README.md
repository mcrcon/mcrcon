# ⛏ mcrcon

[![CI](https://github.com/mcrcon/mcrcon/actions/workflows/ci.yml/badge.svg)](https://github.com/mcrcon/mcrcon/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mcrcon/mcrcon)](https://github.com/mcrcon/mcrcon/releases)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![TUI](https://img.shields.io/badge/TUI-Bubble_Tea-FF75B7)](https://github.com/charmbracelet/bubbletea)
[![RCON](https://img.shields.io/badge/RCON-Minecraft-5EBB2B)](https://minecraft.wiki/w/Server.properties#RCON)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)

A modern Minecraft RCON client in Go. Run one-off commands from scripts,
pipe server commands through stdin, or open a polished fullscreen console
with history, tab-completion, and smart scrolling.

```sh
mcrcon -H 127.0.0.1 -P 25575 -p secret "list"
mcrcon -H play.example.com -p secret              # open the TUI
echo "list" | mcrcon -H 127.0.0.1 -p secret --no-tui
```

## Contents

- [Highlights](#highlights)
- [Preview](#preview)
- [Requirements](#requirements)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Usage](#usage)
  - [One-shot mode](#1-one-shot-mode)
  - [TUI mode](#2-tui-mode)
  - [Plain / pipe mode](#3-plain--pipe-mode)
- [Configuration](#configuration)
- [Recipes](#recipes)
- [Keybindings](#keybindings)
- [Project structure](#project-structure)
- [Development](#development)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)
- [Security](#security)
- [Contributing](#contributing)
- [License](#license)

## Highlights

- 🖥️ **Fullscreen TUI** (alt-screen, mouse-wheel scrolling) with a connect
  form and a live session console.
- ⌨️ **Command history** (`↑`/`↓`, `ctrl+p`/`ctrl+n`), persisted across
  restarts at `~/.config/mcrcon/history`.
- 🔍 **Tab-completion** for common Vanilla / Paper / Spigot commands with a
  live candidate preview above the input.
- 📌 **Smart scrolling** — output sticks to the bottom only while you are
  already there; scroll up and a `↓ N new line(s) — press end` indicator
  appears instead of yanking your view.
- ⚡ **Status pill** showing `CONNECTED` / `CONNECTING` / `OFFLINE`, plus
  last-command latency and in-flight request count.
- 🎨 **True Minecraft colors** — `§aHello` renders in green, `§l` bold, etc.
  Colors show in the TUI and on TTYs; piped/script output stays plain text
  (override with `--color always|never`, honors `NO_COLOR`).
- 🔌 **Pure-Go RCON client** — thread-safe auth + exec with multi-packet
  reassembly for large responses such as `help`.
- 🧩 **Three modes in one binary** — TUI, one-shot (classic `mcrcon` style),
  and pipe-friendly plain mode for cron jobs and scripts.
- 🩺 **Actionable errors** — wrong password, refused connection, and
  timeouts each explain what to check.

## Preview

A typical TUI session looks like this:

```text
  mcrcon  │  127.0.0.1:25575    ● CONNECTED · 4ms
15:04:05  Welcome to mcrcon — Minecraft RCON console
15:04:07  Connected to 127.0.0.1:25575
15:04:12  > list
15:04:12  There are 2 of a max of 20 players online: Steve, Alex
15:04:12  — 4ms
Tab ▸ list
> li_
enter send • ↑↓ history • tab complete • PgUp/PgDn scroll • F1 help
```

## Requirements

**Minecraft server** — enable RCON in `server.properties`, then restart:

```properties
enable-rcon=true
rcon.port=25575
rcon.password=change-me-to-something-secret
broadcast-rcon-to-ops=true
```

**Build machine** — Go 1.22 or newer (developed with Go 1.26).

## Installation

Build from source:

```sh
git clone https://github.com/mcrcon/mcrcon.git && cd mcrcon
go build -o mcrcon .
./mcrcon --help
```

Or install the latest release straight into `$GOPATH/bin`:

```sh
go install github.com/mcrcon/mcrcon@latest
```

Prebuilt binaries for Linux, macOS, and Windows (amd64/arm64) are
attached to every [GitHub release](https://github.com/mcrcon/mcrcon/releases).

No runtime dependencies — the result is a single static binary that runs on
Linux, macOS, and Windows.

## Quick start

1. Point it at your server and open the console:

   ```sh
   mcrcon -H 127.0.0.1 -p secret
   ```

2. If you started it without credentials, fill in the connect form
   (`Tab` moves between fields, `Enter` continues).
3. Type `list` and press `Enter`. Press `F1` any time for keybindings.

> 💡 Keep the password out of your shell history and `ps` output by
> exporting it instead of passing `-p`:
>
> ```sh
> export MCRCON_PASSWORD=secret
> mcrcon -H 127.0.0.1 "list"
> ```

## Usage

```text
mcrcon -H <host> -P <port> -p <password> [command...]
```

| Flag | Default | Description |
| --- | --- | --- |
| `-H`, `--host` | `127.0.0.1` (in TUI) | RCON host (`MCRCON_HOST`) |
| `-P`, `--port` | `25575` | RCON port (`MCRCON_PORT`) |
| `-p`, `--password` | — | RCON password (`MCRCON_PASSWORD`, `RCON_PASSWORD`) |
| `--timeout` | `8` | Connection & command timeout, in seconds |
| `--color` | `auto` | Minecraft colors: `auto` (TTY only), `always`, `never` |
| `-t`, `--tui` | — | Force TUI mode |
| `--no-tui` | — | Force plain mode (no TUI) |
| `-h`, `--help` | — | Print help |
| `-v`, `--version` | — | Print version |

Flag values take precedence over environment variables.

### 1. One-shot mode

Pass the command as arguments to execute once, print the reply, and exit.
Exit code `0` on success, `1` on failure.

```sh
mcrcon -H 127.0.0.1 -P 25575 -p secret "list"
mcrcon -H 127.0.0.1 -p secret say Hello from RCON
MCRCON_HOST=mc.example.com MCRCON_PASSWORD=secret mcrcon "tps"
```

### 2. TUI mode

With no command arguments and a TTY attached, the fullscreen console opens
automatically:

```sh
mcrcon -H 127.0.0.1 -p secret
mcrcon   # enter host & password in the connect form
```

The header always shows where you are connected and how healthy the link
is. The input border mirrors the state: **green** connected,
**yellow** connecting, **red** offline.

Local commands (typed in the console, start with `/`):

| Command | Effect |
| --- | --- |
| `/help` | Show local commands in the log |
| `/history` | Show the last 20 commands |
| `/clear` | Clear the screen |
| `/reconnect` | Reconnect to the server |
| `/quit` | Quit mcrcon |

Anything else is sent to the server — a single leading `/` is stripped
automatically, so `/list` and `list` are equivalent.

### 3. Plain / pipe mode

Without a TTY, or with `--no-tui`, mcrcon reads commands line-by-line from
stdin and writes replies to stdout — ideal for scripts:

```sh
echo "list" | mcrcon -H 127.0.0.1 -p secret --no-tui
printf 'time set day\nweather clear\n' | mcrcon -H 127.0.0.1 -p secret --no-tui
mcrcon -H 127.0.0.1 -p secret --no-tui   # simple REPL, exit with Ctrl-D
```

## Configuration

| Source | Variables |
| --- | --- |
| Flags | `-H`, `-P`, `-p`, `--timeout`, `--color` (highest precedence) |
| Environment | `MCRCON_HOST` (`RCON_HOST`, `MINECRAFT_HOST` also work), `MCRCON_PORT` (`RCON_PORT`), `MCRCON_PASSWORD` (`RCON_PASSWORD`, `MCRCON_PASS`) |
| Files | Command history: `~/.config/mcrcon/history` (or `~/.mcrcon_history` as fallback) |

## Recipes

Announce a backup window, then save:

```sh
printf 'say Backup in 60 seconds!\nsave-all\n' | \
  mcrcon -H 127.0.0.1 -p "$MCRCON_PASSWORD" --no-tui
```

Nightly player-count check from cron:

```sh
0 6 * * * /usr/local/bin/mcrcon -H 127.0.0.1 -p "$MCRCON_PASSWORD" "list" >> /var/log/mc-players.log
```

Whitelist a player without opening the console:

```sh
mcrcon -H mc.example.com -p "$MCRCON_PASSWORD" "whitelist add Steve"
```

## Keybindings

| Key | Action |
| --- | --- |
| `enter` | Send command |
| `↑` / `↓` (`ctrl+p` / `ctrl+n`) | Command history |
| `tab` | Complete command (candidates preview above input) |
| `PgUp` / `PgDn` | Scroll output (`Home` top, `End` latest) |
| `shift+↑` / `shift+↓` | Scroll one line |
| `esc` | Clear input |
| `ctrl+u` | Clear input |
| `ctrl+l` | Clear screen |
| `ctrl+r` | Reconnect |
| `ctrl+d` | Disconnect → server screen |
| `F1` (`?` on empty input) | Toggle help (`?` stays typeable while composing) |
| `ctrl+c` | Quit |

## Project structure

```text
.
├── main.go               # CLI: flags and one-shot / TUI / plain dispatch
├── internal/
│   ├── rcon/             # RCON wire protocol + thread-safe client
│   └── tui/              # Bubble Tea model (connect + session screens)
├── .github/
│   ├── workflows/        # CI (fmt/vet/test matrix) + GoReleaser releases
│   ├── ISSUE_TEMPLATE/   # bug report + feature request
│   └── PULL_REQUEST_TEMPLATE.md
├── .goreleaser.yml       # cross-platform release builds + archives
├── Makefile              # build / test / vet / fmt shortcuts
├── CONTRIBUTING.md
├── SECURITY.md
├── LICENSE
└── README.md
```

## Development

```sh
make build     # build ./mcrcon (stamps a -dev version)
make test      # go test -race -count=1 ./...
make vet       # go vet ./...
make fmt       # gofmt -w . + list leftovers
```

CI (`.github/workflows/ci.yml`) runs the `gofmt` check, `go vet`, and the
race-enabled test suite on Linux, macOS, and Windows for every push and
pull request. See [CONTRIBUTING.md](CONTRIBUTING.md) for the full workflow.

### Releases

Releases are automated with GoReleaser (`.goreleaser.yml`). To cut one,
push a tag — `.github/workflows/release.yml` builds Linux/macOS/Windows
binaries (amd64/arm64), attaches them to the GitHub release with
checksums, and stamps the version into the binary:

```sh
git tag v1.2.0 && git push origin v1.2.0
```

Local dry run (requires [GoReleaser](https://goreleaser.com/install)):

```sh
make snapshot        # goreleaser release --snapshot --clean
make release-check   # goreleaser check
```

## Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| `Authentication failed` | Wrong password, `rcon.password` unset, or the server wasn't restarted after changing it |
| `Connection refused` | `enable-rcon=false`, wrong port, firewall rule, or server offline |
| `Timed out` | Host unreachable or port filtered — try `--timeout 15` |
| `(ok, … — no output)` | Normal — e.g. `save-all` genuinely returns little output |
| Weird `§a` characters | Rendered as colors on TTY/TUI; use `--color never` for plain text |

## FAQ

**Does it work with Paper / Purpur / Spigot / Forge?**
Yes — anything speaking the standard Source/Minecraft RCON protocol.
Vanilla, CraftBukkit-family, and modded servers exposing RCON all work.

**How is this different from classic mcrcon?**
Classic `mcrcon` is one-shot only. This tool keeps that mode (same
`-H/-P/-p` flags) and adds a fullscreen console plus a pipe-friendly REPL.

**Where is my command history stored?**
`~/.config/mcrcon/history` (last 500 entries loaded, capped in memory).
It is created with `0600` permissions since commands may contain sensitive
text.

**The TUI looks broken in my terminal.**
It needs a TTY with at least ~80×15 cells and true-color support for the
full theme. For constrained environments use `--no-tui`.

## Security

- RCON has **no encryption**. Bind `rcon.port` to localhost or a trusted
  network, firewall it, and use a long random password.
- Prefer `MCRCON_PASSWORD` over `-p` so the secret never appears in
  process listings or shell history.
- `server.properties` and shell history files containing the password
  should not be committed to version control.

## Contributing

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md)
for the workflow and quality gates, and [SECURITY.md](SECURITY.md) for
reporting vulnerabilities.

## License

MIT — see [LICENSE](LICENSE).
