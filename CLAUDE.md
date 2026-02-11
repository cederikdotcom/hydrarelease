# HydraRelease

Self-hosted release file server for experiencenet projects (hydraguard, hydracluster, nimsforest).

## Current Infrastructure

| Resource | Value |
|----------|-------|
| **Release server** | `46.225.120.7` (Hetzner cx23, Nuremberg) |
| **Release URL** | `https://releases.experiencenet.com` |
| **Release files** | `/var/www/releases/` on the release server |
| **Deploy user** | `deploy@releases.experiencenet.com` (SSH key auth, rsync target for CI) |
| **Cert cache** | `/var/lib/hydrarelease/certs` |
| **Systemd unit** | `hydrarelease.service` |
| **Domain DNS** | `experiencenet.com` is on Hetzner DNS (zone ID 788422) |
| **Hetzner project** | Same token as hydraguard (`~/.morpheus/config.yaml`) |

## Build and Deploy

```bash
make build          # Build binary to bin/hydrarelease
make deploy         # Cross-compile and deploy to release server
make test           # Run all tests
make vet            # Run go vet
make fmt            # Format code
```

Requires Go 1.22+.

## Project Structure

```
cmd/hydrarelease/main.go     # CLI entrypoint
internal/cli/                 # Cobra commands (root, serve, common)
```

## File layout on server

```
/var/www/releases/
  hydraguard/
    latest.json                # {"version": "1.1.0"}
    v1.1.0/
      hydraguard-linux-amd64
      hydraguard-linux-arm64
      hydraguard-darwin-amd64
      hydraguard-darwin-arm64
      SHA256SUMS
  hydracluster/                # future
  nimsforest/                  # future
```

## Common Agent Tasks

### Deploy updated binary
```bash
make deploy
```

### SSH to the release server
```bash
ssh root@46.225.120.7
```

### Check service status
```bash
ssh root@46.225.120.7 "systemctl status hydrarelease"
```

### Upload a release manually
```bash
rsync -avz dist/ deploy@releases.experiencenet.com:/var/www/releases/<project>/<version>/
```
