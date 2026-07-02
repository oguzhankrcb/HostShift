---
title: Docker Compose
description: Run the documentation website with Docker Compose.
---

The documentation website is self-contained under `docs-site/`.

Run it with Docker Compose:

```bash
docker compose -f docs-site/compose.yml up --build
```

Open:

```text
http://localhost:4321
```

Stop it with:

```bash
docker compose -f docs-site/compose.yml down
```

The Compose service runs Starlight in development mode and mounts `docs-site/src`, `docs-site/public`, and `docs-site/astro.config.mjs` read-only into the container.
