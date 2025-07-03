# Variables
BINARY_NAME = go-fdo-client
INSTALL_DIR = /usr/local/bin


$(BINARY_NAME):
	go build -o $(BINARY_NAME)

# Build the Go project
build: tidy fmt vet $(BINARY_NAME)

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet -v ./...

test:
	go test -v ./...

# Clean up the binary
clean:
	rm -f $(BINARY_NAME)

install: build
	install -D -m 755 $(BINARY_NAME) $(INSTALL_DIR)/$(BINARY_NAME)
# Default target
all: build test
