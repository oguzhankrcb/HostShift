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

## Production

Production uses a multi-stage image. Astro generates static files during the image build, then an Nginx container serves them on port `4321`:

```bash
docker compose \
  -p hostshift-docs \
  -f docs-site/compose.production.yml \
  up -d --build --remove-orphans
```

The production Compose file publishes the service only as `127.0.0.1:4321`. It is not intended to be exposed directly to the internet. A host-level reverse proxy should be the public boundary.

For the official documentation host, the reviewed Nginx vhost is stored at:

```text
docs-site/deploy/hostshift.karacabay.com.nginx.conf
```

Install it only after checking that the domain has no existing vhost, then validate before reload:

```bash
install -o root -g root -m 0644 \
  docs-site/deploy/hostshift.karacabay.com.nginx.conf \
  /etc/nginx/sites-available/hostshift.karacabay.com

ln -s \
  /etc/nginx/sites-available/hostshift.karacabay.com \
  /etc/nginx/sites-enabled/hostshift.karacabay.com

nginx -t
systemctl reload nginx
```

The container has `restart: unless-stopped`, so it returns automatically after a Docker daemon or server restart. TLS for `hostshift.karacabay.com` is terminated by Cloudflare; the origin vhost listens on HTTP and proxies only to the loopback-bound container.
