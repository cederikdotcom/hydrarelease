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

## Build and Test

```bash
make build          # Build binary to bin/hydrarelease
make test           # Run all tests
make vet            # Run go vet
make fmt            # Format code
```

Requires Go 1.22+.

## Releasing

Releases go through GitHub Actions CI/CD. **Do not use `make deploy` for production.**

1. Tag a new version: `git tag v<X.Y.Z>`
2. Push the tag: `git push origin v<X.Y.Z>`
3. GitHub Actions builds, creates a GitHub Release, and uploads binaries to the release server. The service auto-updates from `latest.json` on its next poll cycle.
4. Verify: `hydrarelease verify --project hydrarelease`

See [docs/runbooks/runbook-release-pipeline.md](docs/runbooks/runbook-release-pipeline.md) for the full release pipeline check procedure.

## Project Structure

```
cmd/hydrarelease/main.go     # CLI entrypoint
internal/cli/                 # Cobra commands (root, serve, common)
```

## File layout on server

```
/var/www/releases/
  hydraguard/
    production/
      latest.json              # {"version": "1.1.0"}
      v1.1.0/
        hydraguard-linux-amd64
        hydraguard-linux-arm64
        hydraguard-darwin-amd64
        hydraguard-darwin-arm64
        SHA256SUMS
    staging/
      latest.json
      v1.2.0/
        ...
  hydracluster/                # future
  nimsforest/                  # future
```

## Common Agent Tasks

### SSH to the release server
```bash
ssh root@46.225.120.7
```

### Check service status
```bash
ssh root@46.225.120.7 "systemctl status hydrarelease"
```

## Runbooks

See [docs/runbooks/](docs/runbooks/) for operational runbooks (release pipeline check, etc.).
