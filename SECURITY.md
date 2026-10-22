# Security policy

## Reporting a vulnerability

Please don't open a public issue for a security problem. Report it privately through GitHub instead: on the repository's **Security** tab, choose **Report a vulnerability** ([GitHub's guide](https://docs.github.com/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)).

Please include the version (`beltlined --version`), what you did, what happened and, if you can, a minimal way to reproduce it. You should hear back within a week. Once a fix is on `main`, the advisory is published, and you are credited unless you'd rather not be.

## Supported versions

Beltline installs from `main`, and only `main` gets fixes.

## Scope

Beltline runs coding agents with their approval prompts turned off, as your user and with your `gh` and `acli` sign-ins. That is by design, and [docs/security.md](docs/security.md) explains it and how to limit it. The following are in scope:

- Ways for another website, another user on the machine, or a request that isn't from the web app to reach the local API, a terminal or the data folder.
- Ways for an artifact to escape its sandbox, or for a request to read files outside the attempt folder.
- Tokens or credentials written somewhere they shouldn't be.
- Beltline taking an outward action (a push, comment, label or ticket move) that no agent or policy asked for.

A prompt injection that makes an agent misbehave is a known risk of agents with full permissions. It is in scope only when Beltline itself makes it worse than the agent CLI would on its own.
