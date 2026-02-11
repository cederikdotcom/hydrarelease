package main

import "github.com/cederikdotcom/hydrarelease/internal/cli"

// version is set at build time via -ldflags "-X main.version=v1.0.0"
var version = "dev"

func main() {
	cli.SetVersion(version)
	cli.Execute()
}
