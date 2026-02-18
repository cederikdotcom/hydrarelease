module github.com/cederikdotcom/hydrarelease

go 1.25.7

require (
	github.com/cederikdotcom/hydraapi v0.0.0-00010101000000-000000000000
	github.com/cederikdotcom/hydraauth v0.0.0-00010101000000-000000000000
	github.com/cederikdotcom/hydramonitor v0.0.0-00010101000000-000000000000
	github.com/spf13/cobra v1.10.2
	golang.org/x/crypto v0.48.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace (
	github.com/cederikdotcom/hydraapi => ../hydraapi
	github.com/cederikdotcom/hydraauth => ../hydraauth
	github.com/cederikdotcom/hydramonitor => ../hydramonitor
)
