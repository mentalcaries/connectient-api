# Container deployment

## Goal

Package the Go API as a small, non-root OCI image that GitHub Actions publishes
to GHCR for a single-replica Dokploy deployment.

## Image

- Build the API with Go 1.26 and `CGO_ENABLED=0`.
- Run it as an unprivileged user on Alpine with CA certificates and timezone data.
- Default to port 4000 while allowing Dokploy to override `PORT` at runtime.
- Use `/health` as the container health check.
- Keep configuration and secrets out of the image; Dokploy supplies all
  environment variables at runtime.
- Do not run Goose migrations during container startup. Managed databases must be
  migrated explicitly before deploying the corresponding API version.

## Publishing

GitHub Actions builds pull requests without publishing. Pushes to `main` publish
`latest` and commit-SHA tags to `ghcr.io/mentalcaries/connectient-api`; version
tags also publish their tag name.

## Runtime constraints

The current appointment event hub is in memory, so Dokploy must run exactly one
API replica. The reverse proxy must permit long-lived SSE responses. The service
requires no persistent filesystem volume.
