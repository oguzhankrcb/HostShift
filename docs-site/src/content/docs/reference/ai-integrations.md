---
title: AI Integrations
description: Codex, Claude, and MCP integration points for HostShift.
---

HostShift is CLI-first. AI integrations call the deterministic Go binary instead of reimplementing migration behavior.

## MCP Server

Run the stdio MCP server:

```bash
hostshift mcp stdio
```

The server exposes safe planning tools:

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

Start from `integrations/claude/claude_desktop_config.example.json`:

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

## Codex

The Codex plugin under `plugins/hostshift` provides the `migrate-server` skill. It is an operator layer around the same CLI and safety model.

## Safety Boundary

AI clients may inspect plans and run source read-only discovery through HostShift. They must not bypass HostShift by running arbitrary SSH commands.

If an AI client suggests an apply command, run the matching dry-run first and review blockers, actions, streams, and rollback metadata manually.
