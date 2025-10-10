#!/bin/bash
set -euox pipefail

# E2E Test Script for go-fdo-client
# This script tests the FDO onboarding workflow with local services.
# 127.0.0.1 usage is intentional for testing local FDO server instances.
# devskim: ignore DS137138

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

function error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

function warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# Verify go-fdo-client is installed
info "Verifying go-fdo-client installation..."
if ! rpm -q go-fdo-client &>/dev/null; then
    error "go-fdo-client package is not installed"
    exit 1
fi
CLIENT_PKG=$(rpm -q go-fdo-client)
info "go-fdo-client package is installed: ${CLIENT_PKG}"
# Verify we're testing the PR artifact (should contain .pr or timestamp in version)
if [[ ! "$CLIENT_PKG" =~ (pr[0-9]+|[0-9]{14}) ]]; then
    error "Package version does not appear to be from PR build: ${CLIENT_PKG}"
    error "Expected version with '.pr' or timestamp, but got: ${CLIENT_PKG}"
    error "This suggests the PR artifact was replaced by a stable version."
    exit 1
fi
info "Confirmed testing PR artifact build"

# Verify go-fdo-server subpackages are installed
info "Verifying go-fdo-server installation..."
if ! rpm -q go-fdo-server-manufacturer &>/dev/null; then
    error "go-fdo-server-manufacturer package is not installed"
    exit 1
fi
if ! rpm -q go-fdo-server-rendezvous &>/dev/null; then
    error "go-fdo-server-rendezvous package is not installed"
    exit 1
fi
if ! rpm -q go-fdo-server-owner &>/dev/null; then
    error "go-fdo-server-owner package is not installed"
    exit 1
fi
info "go-fdo-server packages are installed"

# Check that go-fdo-client binary is available
info "Checking go-fdo-client binary..."
if ! command -v go-fdo-client &> /dev/null; then
    error "go-fdo-client binary not found in PATH"
    exit 1
fi
info "go-fdo-client binary found: $(which go-fdo-client)"

# Verify client help menu is accessible
info "Verifying go-fdo-client help menu..."
if ! go-fdo-client --help &>/dev/null; then
    error "Failed to display go-fdo-client help menu"
    exit 1
fi
info "go-fdo-client help menu is accessible"

# Create test directory
mkdir -p /tmp/fdo-test
cd /tmp/fdo-test

# Start FDO services
info "Setting up FDO server environment..."
info "Starting FDO Manufacturing Server..."
systemctl start go-fdo-server-manufacturer.service || {
    error "Failed to start FDO Manufacturing Server"
    journalctl -u go-fdo-server-manufacturer.service --no-pager
    exit 1
}

info "Starting FDO Rendezvous Server..."
systemctl start go-fdo-server-rendezvous.service || {
    error "Failed to start FDO Rendezvous Server"
    journalctl -u go-fdo-server-rendezvous.service --no-pager
    exit 1
}

info "Starting FDO Owner Server..."
systemctl start go-fdo-server-owner.service || {
    error "Failed to start FDO Owner Server"
    journalctl -u go-fdo-server-owner.service --no-pager
    exit 1
}

# Wait for services to be ready
sleep 5

# Check service status
info "Verifying FDO services are running..."
systemctl is-active --quiet go-fdo-server-manufacturer.service || {
    error "FDO Manufacturing Server is not active"
    journalctl -u go-fdo-server-manufacturer.service --no-pager
    exit 1
}

systemctl is-active --quiet go-fdo-server-rendezvous.service || {
    error "FDO Rendezvous Server is not active"
    journalctl -u go-fdo-server-rendezvous.service --no-pager
    exit 1
}

systemctl is-active --quiet go-fdo-server-owner.service || {
    error "FDO Owner Server is not active"
    journalctl -u go-fdo-server-owner.service --no-pager
    exit 1
}

info "All FDO services are running"

# Configure rendezvous info in manufacturer server (JSON format)
info "Configuring rendezvous info in manufacturing server..."
curl --fail --silent --show-error \
     --header 'Content-Type: text/plain' \
     --request POST \
     --data-raw '[{"dns":"rendezvous","device_port":"8041","owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]' \
     "http://127.0.0.1:8038/api/v1/rvinfo" || {
    error "Failed to set rendezvous info"
    exit 1
}
info "Rendezvous info configured"

# Configure owner redirect in owner server (JSON format)
info "Configuring owner redirect in owner server..."
curl --fail --silent --show-error \
     --header 'Content-Type: text/plain' \
     --request POST \
     --data-raw '[{"dns":"owner","port":"8043","protocol":"http","ip":"127.0.0.1"}]' \
     "http://127.0.0.1:8043/api/v1/owner/redirect" || {
    error "Failed to set owner redirect"
    exit 1
}
info "Owner redirect configured"

# Test 1: Device Initialization (DI)
# Using default ports: manufacturer=8038, rendezvous=8041, owner=8043
# Note: 127.0.0.1 is intentional for e2e testing with local services
info "Testing Device Initialization (DI)..."
# devskim: ignore DS137138 - 127.0.0.1 is required for testing local FDO services
go-fdo-client device-init http://127.0.0.1:8038 \
    --device-info e2e-test-device \
    --key ec256 \
    --debug \
    --blob /tmp/cred.bin || {
    error "Device initialization failed"
    exit 1
}

# Verify credential blob was created
if [ ! -f /tmp/cred.bin ]; then
    error "Credential blob file was not created"
    exit 1
fi
info "Device initialization successful - credential blob created"

# Extract device GUID from credential blob
info "Extracting device GUID from credential blob..."
GUID=$(go-fdo-client print --blob /tmp/cred.bin | grep -oE '[0-9a-fA-F]{32}' | head -n1)
if [ -z "$GUID" ]; then
    error "Failed to extract device GUID"
    go-fdo-client print --blob /tmp/cred.bin
    exit 1
fi
info "Device GUID: ${GUID}"

# Download ownership voucher from Manufacturing server
info "Downloading ownership voucher from Manufacturing server..."
curl --fail --silent --show-error \
     "http://127.0.0.1:8038/api/v1/vouchers/${GUID}" \
     -o /tmp/fdo-test/voucher.pem || {
    error "Failed to download ownership voucher"
    exit 1
}
info "Ownership voucher downloaded"

# Upload ownership voucher to Owner server
info "Uploading ownership voucher to Owner server..."
curl --fail --silent --show-error \
     --request POST \
     --data-binary @/tmp/fdo-test/voucher.pem \
     "http://127.0.0.1:8043/api/v1/owner/vouchers" || {
    error "Failed to upload voucher to Owner"
    exit 1
}
info "Ownership voucher uploaded to Owner"

# Trigger TO0 (Owner registers with Rendezvous)
info "Triggering TO0 protocol (Owner registers with Rendezvous)..."
curl --fail --silent --show-error \
     "http://127.0.0.1:8043/api/v1/to0/${GUID}" || {
    error "Failed to trigger TO0"
    exit 1
}
info "TO0 protocol completed successfully"

# Run device onboarding (TO1 + TO2)
info "Running device onboarding (TO1 + TO2)..."
go-fdo-client onboard \
    --key ec256 \
    --kex ECDH256 \
    --debug \
    --blob /tmp/cred.bin | tee /tmp/fdo-test/onboard.log || {
    error "Device onboarding failed"
    cat /tmp/fdo-test/onboard.log
    exit 1
}

# Verify onboarding completed successfully
if ! grep -q 'FIDO Device Onboard Complete' /tmp/fdo-test/onboard.log; then
    error "Onboarding did not complete successfully"
    cat /tmp/fdo-test/onboard.log
    exit 1
fi
info "Device onboarding completed successfully"

# Check service logs for errors
info "Checking service logs for errors..."
journalctl -u go-fdo-server-manufacturer.service --no-pager | tail -20
journalctl -u go-fdo-server-rendezvous.service --no-pager | tail -20
journalctl -u go-fdo-server-owner.service --no-pager | tail -20

# Success
info "======================================="
info "Go FDO Client E2E Test PASSED"
info "======================================="
info "✓ go-fdo-client package installed correctly"
info "✓ go-fdo-client binary is functional"
info "✓ FDO server services started successfully"
info "✓ Rendezvous info configured"
info "✓ Owner redirect configured"
info "✓ Device initialization (DI) completed"
info "✓ Credential blob created and validated"
info "✓ Device GUID extracted: ${GUID}"
info "✓ Ownership voucher transferred"
info "✓ TO0 protocol completed"
info "✓ Device onboarding (TO1/TO2) completed"
info "✓ Full end-to-end FDO workflow validated"
info "======================================="

exit 0
