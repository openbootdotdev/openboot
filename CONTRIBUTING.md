# Contributing

Contributions are welcome. @fullstackjam maintains the project and reviews all PRs — he has final say on what gets merged, but good ideas land fast.

## Quick Start

```bash
git clone https://github.com/YOUR_USERNAME/openboot.git
cd openboot
openboot init        # installs Go, upx, gh, tart — then builds
git checkout -b fix-something

# Test
make test-unit

# Commit
git commit -m "fix: the thing"
git push origin fix-something
```

> Don't have OpenBoot yet? `curl -fsSL openboot.dev/install.sh | bash`

Then open a PR — use the template, it's short.

## Good First Contributions

- Add a package to `internal/config/data/packages.yaml`
- Fix a typo or improve an error message
- Add a missing test

See [issues labeled `good first issue`](https://github.com/openbootdotdev/openboot/issues?q=is%3Aopen+label%3A%22good+first+issue%22) for tracked tasks.

## Running Tests

```bash
make test-unit           # Fast
make test-integration    # Slower
make test-all            # Everything + coverage
```

VM-based E2E tests require [Tart](https://github.com/cirruslabs/tart) (macOS virtualization). Install it only if you need to run `make test-vm-*`:

```bash
brew install cirruslabs/cli/tart
```

## Code Expectations

- Standard Go style (`go vet` must pass)
- Add tests for new features
- Conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`)
- One logical change per PR

## Architecture

See [CLAUDE.md](CLAUDE.md) for how everything fits together.

## Questions

Open a [Discussion](https://github.com/openbootdotdev/openboot/discussions).
