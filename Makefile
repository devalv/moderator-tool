.PHONY: build-linux-astra build-linux-redos build-win-11 fmt build-deb test


fmt:
	go mod tidy
ifneq ($(filter linux windows,$(HOST_GOOS)),)
	go fix -diff ./...
	golangci-lint run --fix ./...
endif

test:
	go clean -testcache
	go test ./... -race -timeout=5m -v -count=10


build-linux-astra:
	docker build --platform linux/amd64 -f devops/docker/Dockerfile.astralinux -t moderator-builder:linux .
	mkdir -p dist
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src moderator-builder:linux \
		go build -mod=readonly -trimpath -o dist/Moderator-Launcher ./cmd/


build-linux-redos:
	docker build --platform linux/amd64 -f devops/docker/Dockerfile.redos -t moderator-builder:redos .
	mkdir -p dist/redos
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src moderator-builder:redos \
		go build -mod=readonly -trimpath -o dist/redos/Moderator-Launcher ./cmd/


build-win-11:
	docker build --platform linux/amd64 -f devops/docker/Dockerfile.windows -t moderator-builder:windows .
	mkdir -p dist/win-11
	docker run --rm --platform linux/amd64 --user "$$(id -u):$$(id -g)" \
		-v "$(CURDIR):/src" -w /src moderator-builder:windows \
		go build -mod=readonly -trimpath -ldflags '-H=windowsgui' -o dist/win-11/Moderator-Launcher.exe ./cmd/


build-deb:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make build-deb VERSION=1.2.3"; \
		exit 1; \
	fi
	PACKAGE_VERSION="$(VERSION)" ./devops/build-deb.sh
