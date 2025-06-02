# Variables
BINARY_NAME = fdo_client
CRED_NAME = cred.bin

# Build the Go project
build:
	go build -o $(BINARY_NAME)

# Clean up the binary
clean:
	rm -f $(BINARY_NAME)
	rm -f $(CRED_NAME)

# Default target
all: build
