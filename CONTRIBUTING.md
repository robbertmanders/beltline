# Contributing to Beltline

Thanks for helping. This guide covers how to build Beltline, which checks to run and how changes get in. [AGENTS.md](AGENTS.md) is the full map of the code and its rules. Read its first section before a larger change, since it says which code goes in the daemon and which in the web app.

Bug reports and ideas go in [issues](https://github.com/robbertmanders/beltline/issues), and questions in [discussions](https://github.com/robbertmanders/beltline/discussions). Security problems are reported privately, see [SECURITY.md](SECURITY.md). Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## Set up

You need macOS 15 or later or Linux, Go (the version in `daemon/go.mod`), Node.js LTS, `git` and `tmux`. The integration tests' stand-in agent is a zsh script, so on Linux install `zsh` too. You don't need `gh`, `acli` or an agent CLI for the tests or the demo.

```sh
git clone https://github.com/robbertmanders/beltline.git
cd beltline
(cd Web && npm ci)
make demo        # build beltlined with the web app and open a demo on fixture data
```

`make demo` touches neither GitHub nor your data folder. For the web app with hot reload, keep a daemon running and start `cd Web && npm run dev` in another terminal. Vite proxies to port 7331, or to `BELTLINE_DAEMON_PORT`, and [Web/README.md](Web/README.md) explains how to sign in there. [docs/development.md](docs/development.md) explains the build flavours and every `make` target.

## Which checks to run

- **While you work**, run the tests of what you changed:
  - `cd daemon && go test ./internal/<package>`, with `-run <Name>` for one test
  - `cd Web && npx vitest run <file>`
  - `cd Web && npm run typecheck` after TypeScript changes
- **Wire changes.** A change to the API's types also needs `cd daemon && go test ./internal/server/...`, which checks `Web/src/api/wire.gen.ts` against the Go types. Rewrite that file with `BELTLINE_UPDATE_GOLDENS=1 go test ./internal/server/api -run TestTypeScript`. Rewrite the contract goldens in `daemon/testdata/golden/` the same way, and only for an intended change.
- **Before you open a pull request**, run `make quick` (vet, the Go tests without tmux, and the web typecheck and tests; about a minute). For Go changes, also run `make lint`, which needs golangci-lint 2.14.0; AGENTS.md says how to install it.
- **Changes to the tmux, PTY or runner code, `scripts/` or the build** need `make test` (with the integration tests) or `make check` (build, every test and the script checks; about five minutes).
- **Layout changes.** Check them with `make snapshots`, narrowed with `BELTLINE_SNAPSHOT_ONLY=<case prefix>`. Install its browser once with `cd Web && npx playwright install chromium`.

The Go tests run in the `Europe/Amsterdam` time zone, which the contract goldens' clock labels assume; `make` sets `TZ` for you, so set it yourself when you run `go test` directly in another zone.

CI runs the Go tests on macOS and on Linux. A change that touches the operating system (paths, processes, signals, the service, desktop actions) needs both; `make lint` lints the daemon for both.

Write the commands you ran and their results in the pull request.

## Code style

- Go is `gofmt -s` formatted and passes `make lint`. Fix lint findings instead of suppressing them. When a fix would be wrong, put `//nolint:<linter> // <reason>` on that one line.
- New flow behaviour in the engine starts with an engine test. The engine (`core.EngineAdvance`) stays pure.
- Match the code around you: its comment density, its naming and the file naming in AGENTS.md. Keep files roughly 200–450 lines.
- Don't add a dependency without a stated reason. AGENTS.md lists the current ones and why each is there.
- UI changes use the tokens in `Web/src/theme/tokens.css`. Every colour is the one ink at some strength over the one paper, so a new colour is a new `color-mix` strength of the pair rather than a new literal; Dark follows by swapping the pair.

## Pull requests

- Keep each pull request to one change, and link the issue it closes.
- Write the title as a [Conventional Commit](https://www.conventionalcommits.org/): `feat: …`, `fix: …`, `docs: …`, `refactor: …`, `test: …`, `chore: …`, with `!` for a breaking change. The changelog is built from these titles.
- Fill in the template: the goal, the approach and how you validated it.
- CI runs the Go tests, the integration tests, lint and the web checks on every pull request. A maintainer reviews it and squash-merges it.

By contributing, you agree that your contributions are licensed under the [Apache License 2.0](LICENSE).
