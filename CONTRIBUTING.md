# Contributing to DewDrops

Thanks for your interest in contributing to DewDrops!

## Getting Started

1. Fork and clone the repository
2. Make sure you have [Go 1.25+](https://go.dev/dl/) installed
3. Run the tests to confirm everything works:
   ```bash
   make test
   ```

## Development

The codebase is a single `main.go` file in `package main`. There are no sub-packages.

### Building

```bash
make build
```

### Running Tests

```bash
make test
```

Or directly with Go:

```bash
go test -v -count=1 ./...
```

Tests create temporary Git repositories as fixtures, so `git` must be available on your system.

### Formatting

All code must be `gofmt`-formatted:

```bash
make format
```

CI will reject unformatted code (`make format-check`).

## Guidelines

- **Write tests.** New features should include tests. Test fixtures should use real temp Git repos (see
  `setupFixtureRepo` in `main_test.go` for the pattern).
- **Don't break existing behavior.** The default `dewdrops .` command must continue to produce the same output format.
  Run `TestDefaultBehaviorUnchanged` to verify.

## Submitting Changes

1. Create a feature branch from `master`
2. Make your changes
3. Ensure all tests pass: `make test`
4. Ensure code is formatted: `make format-check`
5. Open a pull request against `master`

## Reporting Issues

Open an issue on GitHub with:

- What you expected to happen
- What actually happened
- Steps to reproduce
- Your Go version and OS

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
