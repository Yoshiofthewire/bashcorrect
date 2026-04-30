BINARY   := bashcorrect
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-X github.com/Yoshiofthewire/bashcorrect/cmd.version=$(VERSION) -s -w"
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

.PHONY: build install test lint clean dist

build:
	go build $(LDFLAGS) -o $(BINARY) .

install:
	go install $(LDFLAGS) .

test:
	go test ./...

lint:
	go vet ./...
	@command -v staticcheck >/dev/null 2>&1 && staticcheck ./... || true

clean:
	rm -f $(BINARY)
	rm -rf dist/

dist:
	@mkdir -p dist
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d/ -f1); \
		GOARCH=$$(echo $$platform | cut -d/ -f2); \
		out=dist/$(BINARY)-$$GOOS-$$GOARCH; \
		[ "$$GOOS" = "windows" ] && out=$$out.exe; \
		echo "Building $$out..."; \
		GOOS=$$GOOS GOARCH=$$GOARCH go build $(LDFLAGS) -o $$out . || exit 1; \
	done
	@echo "Done. Binaries in dist/"
