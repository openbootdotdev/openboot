# Contributing

PRs welcome. This is a solo project, so I review everything personally.

## Quick Start

Fork, branch, code, test, PR:

```bash
git clone https://github.com/YOUR_USERNAME/openboot.git
cd openboot
git checkout -b fix-something

# Make changes
go build -o openboot ./cmd/openboot
./openboot --dry-run

# Test
make test-unit

# Commit
git commit -m "fix: the thing"
git push origin fix-something
```

Then open a PR.

## Easy First Contributions

Just:
- Add a package to `internal/config/data/packages.yaml`
- Fix a typo
- Improve an error message
- Add a test

## Running Tests

```bash
make test-unit           # Fast
make test-integration    # Slower
make test-all            # Everything + coverage
```

## Code Expectations

- Standard Go style (run `go vet`)
- Add tests if you add features
- Conventional commits (`feat:`, `fix:`, `docs:`)
- One thing per commit

## Architecture

See [AGENTS.md](AGENTS.md) for how everything fits together.

## Questions

Open a [Discussion](https://github.com/openbootdotdev/openboot/discussions). I respond within 24 hours (usually faster).
