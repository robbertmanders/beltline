# Changelog

Notable changes to Beltline, newest first. Versions follow [SemVer](https://semver.org); before 1.0 a minor version may change the data folder format (always with an automatic, backed-up upgrade) or the API.

## Unreleased

- **Settings → Tools** marks the other agent CLIs as optional once one is found, instead of asking for Copilot CLI and Pi whichever agent you use.
- Beltline installs from source on `main` (`make install`); the Homebrew tap, the release builds and Dependabot are gone.
- Linux support on arm64 and x86-64: a systemd user service for `beltlined install`, `uninstall` and `restart`, the data folder under `$XDG_DATA_HOME`, and `xdg-open`, `code`, `ghostty` and `notify-send` for the desktop actions and notifications. Windows runs the Linux build in WSL2.
- `GET /api/v1/version` reports the `platform` the daemon runs on, and the reveal buttons read "Show in folder" off macOS.
- Review triggers skip pull requests from forks, whose code a review run would check out and run as you.
- New belts start with the **Comment when held** policy off; belts saved with it on keep it.
- The daemon builds with Go 1.27.3 (`toolchain` in `go.mod`), which fixes GO-2026-6809 in `crypto/x509`.

## 0.1.0

The first public version, published on `main` on 2026-10-22. Beltline has no packaged releases; the version names what `main` held then.

- Belts with GitHub issue, pull request review, Jira, scheduled and manual triggers, and agent, approval and watch stages joined by named outcomes, forks and routes.
- Runners for Claude Code, Codex CLI, Copilot CLI, Cursor CLI and Pi, with allowed models, thinking levels and model comparisons per stage.
- The web app: Overview with its Floor, Work, Insights with Evolution, run pages with live agent sessions, the belt and agent editors, Library, AFK and Settings.
- Shadow versions of a belt to A/B-test changes, AFK mode, library repositories for backups and shared belts, and a configuration assistant.
- `beltlined install`, `restart`, `uninstall`, `open`, `--demo`, `--check-library` and `--version`.
