# Security

<!-- prettier-ignore -->
> [!WARNING]
> **AI-assisted project**<br>
> This codebase was built with [Claude Code](https://claude.com/claude-code). It
> works for the author's specific setup but has not been independently audited.
> Review the code before running it in any security-sensitive or production
> environment.

lazyincus connects with the `incus` CLI's own remotes and client
certificate, so it can do whatever that certificate is trusted to: start,
stop and delete instances, edit their config, and open a shell inside them.
Mark a remote you only want to look at
[read-only](docs/Config.md#top-level), or run with `--read-only`, and give
the certificate no more than the projects you need.

## Reporting a vulnerability

Report it privately through
[GitHub's private vulnerability reporting](https://github.com/tallica/lazyincus/security/advisories/new),
not in a public issue. Include what you found, how to reproduce it, and the
version (`lazyincus --version`). Expect an answer within a week; there is no
bounty.

## Supported versions

Only the latest release gets fixes while the project is pre-1.0.
