# HydraRelease

Self-hosted release file server for experiencenet projects. Serves binary releases over HTTPS with automatic Let's Encrypt certificates at `releases.experiencenet.com`.

## Overview

HydraRelease is the central release server for all experiencenet projects:

- **hydraguard** — WireGuard mesh management
- **hydracluster** — Render node fleet management
- **hydrarelease** — This project (self-hosting)

Each project's GitHub Actions workflow builds binaries on tag push and uploads them via rsync. Clients download updates directly over HTTPS — no authentication required.

```
https://releases.experiencenet.com/
  hydraguard/latest.json
  hydraguard/v1.2.0/hydraguard-linux-amd64
  hydracluster/latest.json
  hydracluster/v0.5.0/hydracluster-linux-amd64
  hydrarelease/latest.json
  hydrarelease/v1.0.0/hydrarelease-linux-amd64
```

## Quick Start

```bash
make build
sudo bin/hydrarelease serve
```

## Development

```bash
make build
bin/hydrarelease serve --dev --dir ./testdata
```

## Deployment

Releases are automated via GitHub Actions. Pushing a version tag triggers the workflow:

```bash
git tag v1.0.1
git push origin v1.0.1
```

The workflow builds all platform binaries, uploads them to the release server, and restarts the service with the new version.

For manual deployment:

```bash
make deploy    # Cross-compile, scp to server, restart service
```
