BINARY_NAME=vaultsort
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install uninstall clean

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/vaultsort

install: build
	cp bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Binary installed to /usr/local/bin/$(BINARY_NAME)"
	@echo "Run 'vaultsort install' to set up as a LaunchAgent"

uninstall:
	vaultsort uninstall 2>/dev/null || true
	rm -f /usr/local/bin/$(BINARY_NAME)

clean:
	rm -rf bin/

test:
	go test ./...

testv:
	go test -v ./...

vet:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

all: tidy vet test build
