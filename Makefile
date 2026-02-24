BINARY_NAME=goclip

ifneq ($(wildcard /etc/arch-release),)
	INSTALL_DIR=/usr/bin
else
	INSTALL_DIR=/usr/local/bin
endif

GOFLAGS=-ldflags="-s -w" -trimpath
CGO_ENABLED=0

.PHONY: all build install uninstall clean tidy vet

all: tidy vet build

build:
	CGO_ENABLED=$(CGO_ENABLED) go build $(GOFLAGS) -o ./bin/$(BINARY_NAME) ./cmd/main.go

tidy:
	go mod tidy

vet:
	go vet ./...

install: build
	sudo install -Dm755 ./bin/$(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)

uninstall:
	sudo rm -f $(INSTALL_DIR)/$(BINARY_NAME)

clean:
	rm -f ./bin/$(BINARY_NAME)
