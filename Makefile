.PHONY: build test fmt vet clean install deploy

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"

build:
	@echo "Building hydrarelease $(VERSION)..."
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/hydrarelease ./cmd/hydrarelease

test:
	go test ./... -v

test-cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/ coverage.out

install: build
	sudo cp bin/hydrarelease /usr/local/bin/

deploy: ## Cross-compile and deploy to release server
	@echo "Building hydrarelease $(VERSION) for linux/amd64..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/hydrarelease-linux-amd64 ./cmd/hydrarelease
	scp bin/hydrarelease-linux-amd64 root@releases.experiencenet.com:/usr/local/bin/hydrarelease
	ssh root@releases.experiencenet.com "systemctl restart hydrarelease"
	@echo "Deployed and restarted."
