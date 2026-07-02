# Contributing

HostShift accepts focused changes that preserve the read-only-source invariant.

Before submitting a change:

```bash
make test
```

For migration behavior, include either unit tests or an integration fixture. Any change that adds a source command must add a negative safety test proving that mutating commands remain rejected.

Before a release candidate, run the full validation gates documented in `docs/validation.md`. At minimum this means the unit suite, Go suite, Docker dry-run, real Docker matrix, VM dry-run, VM apply workflow, and release snapshot must pass against the current binary.
