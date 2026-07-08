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

Validate the MCP tool surface and Claude Desktop config example:

```bash
hostshift mcp doctor --json
```

The server exposes safe planning tools:

- `hostshift_doctor`
- `hostshift_discover`
- `hostshift_plan`
- `hostshift_explain`
- `hostshift_review`
- `hostshift_prepare_dry_run`
- `hostshift_sync_dry_run`
- `hostshift_verify_dry_run`
- `hostshift_cutover_dry_run`
- `hostshift_profile_migrate`
- `hostshift_policy_source`
- `hostshift_capabilities`
- `hostshift_rollback`

No MCP tool exposes `--apply`. Target mutations still require a human-operated CLI command.

The server also exposes one MCP prompt:

- `hostshift_migration_operator`: loads the HostShift source-safety rules, preferred dry-run workflow, and operator approval boundary into the client.

Use `hostshift_explain` when an AI client needs a concise migration brief. Use `hostshift_review` when it needs structured findings, workload-aware recommendations, suggested YAML snippets, an operator checklist, and an AI safety brief. Use `hostshift_capabilities` when it needs the supported platform, workload, check, source fact, and package capability catalog before proposing a migration plan. Use `hostshift_profile_migrate` for local v1-to-v2 profile conversion and `hostshift_policy_source` when the AI client needs the source immutability contract as structured data. These commands run without remote mutation.

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

Check the example before copying it:

```bash
hostshift mcp doctor --claude-config integrations/claude/claude_desktop_config.example.json --json
```

## Codex

The Codex plugin under `plugins/hostshift` provides the `migrate-server` skill. It is an operator layer around the same CLI and safety model.

## Safety Boundary

AI clients may inspect plans and run source read-only discovery through HostShift. They must not bypass HostShift by running arbitrary SSH commands.

If an AI client suggests an apply command, run `hostshift_review`, `hostshift_explain`, `hostshift plan`, or the matching dry-run first and review blockers, actions, streams, and rollback metadata manually.
