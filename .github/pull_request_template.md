## Summary

Describe the behavioral change and its migration impact.

## Validation

- [ ] `make test` passes.
- [ ] Relevant Docker or VM fixtures were added or updated.
- [ ] Documentation and examples match the implementation.

## Source Safety

- [ ] No source-side write, package install, service management, snapshot, or maintenance-mode command was added.
- [ ] Every new source command is typed, read-only, and covered by positive and negative safety tests.
- [ ] Logs, fixtures, and test output contain no credentials or customer-identifying data.

## Target And Rollback

- [ ] Target mutations are visible in the plan and require `--apply`.
- [ ] New target actions include preconditions and rollback metadata where applicable.
