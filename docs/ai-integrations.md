# AI Integrations

HostShift is CLI-first. AI integrations call the deterministic Go binary instead of reimplementing migration behavior.

## MCP Server

Run the stdio MCP server:

```bash
hostshift mcp stdio
```

The MCP transport is newline-delimited JSON-RPC over stdin/stdout. The server exposes only non-apply tools:

- `hostshift_doctor`
- `hostshift_discover`
- `hostshift_plan`
- `hostshift_prepare_dry_run`
- `hostshift_sync_dry_run`
- `hostshift_verify_dry_run`
- `hostshift_cutover_dry_run`
- `hostshift_rollback`

No MCP tool exposes `--apply`. Target mutations still require a human-operated CLI command.

## Claude Desktop

Use [integrations/claude/claude_desktop_config.example.json](../integrations/claude/claude_desktop_config.example.json) as a starting point:

```json
{
  "mcpServers": {
    "hostshift": {
      "command": "/usr/local/bin/hostshift",
      "args": ["mcp", "stdio"]
    }
  }
}
```

Adjust `command` to the installed `hostshift` binary path.

## Safety Boundary

AI clients may inspect plans and ask HostShift to run source read-only discovery. They must not bypass HostShift by running arbitrary SSH commands. Keep the same invariant as the CLI:

> The source server is an immutable observation endpoint.

If an AI client suggests running `prepare --apply`, `sync --apply`, `verify --apply`, or `cutover --apply`, run `hostshift plan` or the relevant dry-run first and review blockers, actions, streams, and rollback metadata manually.
