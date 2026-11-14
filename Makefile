.PHONY: build-linux-astra build-linux-redos build-win-11 fmt build-deb test


fmt:
	go mod tidy
	go fix -diff ./...
	golangci-lint run --fix ./...


test:
	go clean -testcache
	go test ./... -race -timeout=5m -v -count=10


build-linux-astra:
	docker build --platform linux/amd64 -f Dockerfile.astralinux -t glint-moderator-builder:linux .
	mkdir -p dist
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src glint-moderator-builder:linux \
		go build -mod=readonly -trimpath -o dist/GLINT-Moderator-Launcher .


build-linux-redos:
	docker build --platform linux/amd64 -f Dockerfile.redos -t glint-moderator-builder:redos .
	mkdir -p dist/redos
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src glint-moderator-builder:redos \
		go build -mod=readonly -trimpath -o dist/redos/GLINT-Moderator-Launcher .


build-win-11:
	docker build --platform linux/amd64 -f Dockerfile.windows -t glint-moderator-builder:windows .
	mkdir -p dist/win-11
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src glint-moderator-builder:windows \
		go build -mod=readonly -trimpath -ldflags '-H=windowsgui' -o dist/win-11/GLINT-Moderator-Launcher.exe .


build-deb:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make build-deb VERSION=1.2.3"; \
		exit 1; \
	fi
	PACKAGE_VERSION="$(VERSION)" ./devops/build-deb.sh
