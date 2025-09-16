# Build the Go project
build: tidy fmt vet
	go build

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test -v ./...

# Default target
all: build test
