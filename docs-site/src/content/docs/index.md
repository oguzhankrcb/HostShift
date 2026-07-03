---
title: HostShift Documentation
description: Read-only-source Ubuntu and Debian server migration documentation.
template: splash
hero:
  tagline: Move web workloads between Ubuntu and Debian servers without mutating the source host.
  actions:
    - text: Quick Start
      link: /getting-started/quick-start/
      icon: right-arrow
    - text: Install
      link: /getting-started/install/
      icon: download
---

HostShift is a Go CLI and Codex skill/plugin project for discovering, planning, moving, and verifying Linux web workloads.

It also ships a stdio MCP server for AI clients such as Claude Desktop. The MCP surface exposes planning and dry-run tools only; apply operations stay in the human-operated CLI.

The public documentation site is published at [hostshift.karacabay.com](https://hostshift.karacabay.com).

The core invariant is strict:

> The source server is an observation endpoint. HostShift may read facts and stream read-only exports, but it must not write files, install packages, manage services, alter firewall rules, create snapshots, add keys, or place applications into maintenance mode on the source.

## What It Covers

- Ubuntu and Debian platform detection and target package planning
- Docker Compose applications
- Standalone Docker containers
- Bind mount and file-set transfer
- MySQL/MariaDB and PostgreSQL stream migrations
- Nginx config checks and target reload validation
- Laravel-style database connectivity checks
- SSH keepalive, MySQL bind-address, UFW, and nftables target configuration
- Docker matrix and Lima VM validation gates

Use the reference section for exact CLI flags, profile v2 fields, workload/check types, source discovery facts, platform package mappings, action/state JSON, and test matrix behavior.

## Current Milestone

HostShift is preparing its first public `v0.3.0` release. The Go CLI is the migration implementation; the earlier Node migration runtime has been removed.
