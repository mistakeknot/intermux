# Contributing to intermux

Bug reports and pull requests are welcome.

## Before you open a PR

- `go build ./... && go vet ./... && go test -race ./...` pass locally.
- One change per PR. Describe what a user would notice, not what you typed.
- New behavior comes with a test in the same package.

## Reporting a security issue

See [SECURITY.md](SECURITY.md). Do not open a public issue for a vulnerability.

## License

By contributing you agree your contribution is licensed under the MIT license in [LICENSE](LICENSE).
