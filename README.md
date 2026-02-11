# HydraRelease

Self-hosted release file server for experiencenet projects. Serves binary releases over HTTPS with automatic Let's Encrypt certificates.

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
