.PHONY: build run test clean docker-build docker-run docker-stop

# Version information
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# Build flags
LDFLAGS := -ldflags "-w -s \
	-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

build:
	go build $(LDFLAGS) -o forwardauth main.go

run: build
	./forwardauth

test:
	go test -v -race ./...

clean:
	rm -f forwardauth
	go clean -cache

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t elliotechnology/ellio_traefik_forward_auth:latest .

docker-run: docker-build
	docker-compose up -d

docker-stop:
	docker-compose down

docker-logs:
	docker-compose logs -f forwardauth

lint:
	golangci-lint run

fmt:
	go fmt ./...
	
mod-tidy:
	go mod tidy

bench:
	go test -bench=. -benchmem ./...

version:
	@echo "Version:    $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Date: $(BUILD_DATE)"

docker-buildx:
	docker buildx build \
		--platform linux/amd64,linux/arm64,linux/arm/v7 \
		--build-arg VERSION=$(VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t elliotechnology/ellio_traefik_forward_auth:$(VERSION) \
		-t elliotechnology/ellio_traefik_forward_auth:latest \
		--push .