# Contributing

Thanks for your interest in improving this project.

## Issues

Please file bugs and feature requests on the
[issue tracker](https://github.com/deploymenttheory/go-winmd/issues), filling out
the template. Small, well-described reports are genuinely useful contributions.

## Pull requests

- PR titles follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat:`, `fix:`, `docs:`, `chore:`, …) — this is CI-enforced.
- Run `gofmt`/`go vet` and keep the build and tests green.
- By contributing you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Tests

This is a hand-written library. The brute-force suites decode every signature
and attribute in the pinned `Windows.Win32.winmd` fixture (fetched on demand);
they must pass with zero failures. Run `go test ./...`.
