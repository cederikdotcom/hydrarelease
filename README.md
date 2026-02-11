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
  hydraguard/v1.5.0/hydraguard-linux-amd64
  hydracluster/latest.json
  hydracluster/v0.8.0/hydracluster-linux-amd64
  hydrarelease/latest.json
  hydrarelease/v1.4.0/hydrarelease-linux-amd64
```

## Shared Updater Package

HydraRelease provides a shared Go package (`pkg/updater`) that all three projects import for self-updating:

```go
import "github.com/cederikdotcom/hydrarelease/pkg/updater"

u := updater.NewUpdater("myproject", version)
u.SetServiceName("myproject")          // Restart this systemd service after update
u.StartAutoCheck(6*time.Hour, true)    // Check every 6h, auto-apply
```

Features:
- Checks `releases.experiencenet.com/<project>/latest.json` for new versions
- Downloads, verifies, and atomically replaces the binary
- Restarts the configured systemd service after a successful update
- `StartAutoCheck` runs in a background goroutine for hands-free updates

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

## CLI Commands

```bash
hydrarelease serve                     # Start HTTPS file server (production)
hydrarelease serve --dev               # Start in dev mode (plain HTTP)
hydrarelease check-update              # Check for new version
hydrarelease update                    # Download and install latest version
hydrarelease version                   # Print version
```

The server checks for updates automatically every 6 hours and applies them without manual intervention, restarting the `hydrarelease` systemd service after each update.

## Releasing

Pushing a version tag triggers CI to build, publish, and deploy:

```bash
git tag v1.5.0
git push origin v1.5.0
```

The workflow:
1. Runs tests
2. Builds binaries (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64)
3. Creates a GitHub Release with the binaries
4. Uploads binaries + `latest.json` to the release server
5. Deploys the new binary directly to the release server and restarts the service

For manual deployment:

```bash
make deploy    # Cross-compile, scp to server, restart service
```

## License

MIT
