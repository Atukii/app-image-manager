# Versions come from Conventional Commits, tagged and released in one workflow

Every push to `main` is tagged with the next semantic version when it contains a release-worthy commit: `fix:` and `perf:` bump the patch, `feat:` the minor, and `!` after the type or a `BREAKING CHANGE:` footer the major. Other types (`docs:`, `test:`, `ci:`, `refactor:`, `build:`, `chore:`, `style:`) don't release. Before v1.0.0 a breaking change bumps the minor, and the first release is v0.1.0. The calculation is a small tested Go package (`internal/release`) run by `tools/nextversion`.

Tagging, building and releasing are jobs of one workflow run (`build.yaml`) because a tag pushed with the workflow's own `GITHUB_TOKEN` deliberately does not start other workflow runs; a separate "on tag" workflow would never fire. Pushing a `v*` tag by hand still builds and releases it.

## Considered Options

- **release-please**: opens a release pull request that has to be merged, rather than tagging each push.
- **semantic-release or a marketplace tagging action**: works, but hands write access to third-party code, against the preference for allowlisted, pinned tooling.
- **A personal access token or GitHub App** so the tag triggers a separate workflow: one more secret to create and rotate, for no gain.

## Consequences

- Commit messages are the release interface: a feature committed as `chore:` is never released.
- Going to v1.0.0 is a deliberate step: push a `v1.0.0` tag by hand.
