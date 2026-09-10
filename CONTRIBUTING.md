# Contributing to mcrcon

Thanks for helping out. This guide covers the workflow, quality gates, and
conventions used in this repo.

## Setup

Requires Go 1.22+ (see `go.mod` for the exact toolchain).

```sh
git clone https://github.com/mcrcon/mcrcon.git && cd mcrcon
go build -o mcrcon .
./mcrcon --help
```

Or use the Makefile shortcuts: `make build`, `make test`, `make vet`,
`make fmt`.

## Workflow

1. Open an issue first for anything non-trivial, so design can be discussed.
2. Fork, branch off `main` (`feat/...`, `fix/...`, `docs/...`).
3. Keep changes focused; one concern per pull request.
4. Update docs (`README.md`, `--help` output) when behavior changes.
5. Fill in the PR template, including test notes.

## Quality gates (also enforced by CI)

```sh
gofmt -l .                  # must print nothing
go vet ./...
go test -race -count=1 ./...
```

Manual TUI changes should be smoke-tested against a real or fake RCON
server in all affected modes (TUI / one-shot / plain).

## Conventions

- All user-facing strings are English.
- Follow existing code style: small focused functions, table-free
  straightforward tests in `*_test.go` next to the code.
- Commit messages: short imperative summary (`Add X`, `Fix Y`), with a
  body explaining *why* when it isn't obvious.
- Never commit secrets, passwords, tokens, or real server addresses.
  Test fixtures must use dummy values (`secret`, `127.0.0.1`).

## Releases

Maintainers cut releases by pushing a tag; GoReleaser builds and publishes
everything (see `.goreleaser.yml` and `.github/workflows/release.yml`):

```sh
git tag v1.2.0 && git push origin v1.2.0
```
