#!/bin/bash
set -eox pipefail

# E2E Test Script for go-fdo-client
# This script tests the FDO onboarding workflow with local services.
# 127.0.0.1 usage is intentional for testing local FDO server instances.
# devskim: ignore DS137138

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

function log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

function log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

set_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  if grep -q " ${dns}" /etc/hosts; then
    echo "${ip} ${dns}"
    tmp_hosts=$(mktemp)
    sed "s/.* ${dns}/$ip $dns/" /etc/hosts >"${tmp_hosts}"
    cp "${tmp_hosts}" /etc/hosts
    rm -f "${tmp_hosts}"
  else
    echo "${ip} ${dns}" | sudo tee -a /etc/hosts
  fi
}

journalctl_args=("--no-pager")
function get_logs () {
  journalctl "${journalctl_args[@]}" --unit go-fdo-server-manufacturer.service
  journalctl "${journalctl_args[@]}" --unit go-fdo-server-rendezvous.service
  journalctl "${journalctl_args[@]}" --unit go-fdo-server-owner.service
}

. /etc/os-release
[[ "${ID}" = "centos" && "${VERSION_ID}" = "9" ]] || \
[[ "${ID}" = "fedora" && "${VERSION_ID}" = "41" ]] || \
journalctl_args+=("--invocation=0")

printenv | sort

trap get_logs EXIT

log_info "Setup hostnames..."
set_hostname manufacturer 127.0.0.1
set_hostname rendezvous 127.0.0.1
set_hostname owner 127.0.0.1

# Verify go-fdo-client is installed
log_info "Verifying go-fdo-client installation..."
if ! rpm -q go-fdo-client &>/dev/null; then
    log_error "go-fdo-client package is not installed"
    exit 1
fi
CLIENT_PKG=$(rpm -q go-fdo-client)
log_info "go-fdo-client package is installed: ${CLIENT_PKG}"

if [[ -n "${PACKIT_COPR_RPMS}" && ! "$CLIENT_PKG" = "${PACKIT_COPR_RPMS}" ]]; then
    log_error "Package version does not appear to be from PR build: '${CLIENT_PKG}'"
    log_error "Expected version was '${PACKIT_COPR_RPMS}', but got: '${CLIENT_PKG}'"
    log_error "This suggests the PR artifact was replaced by a stable version."
    exit 1
fi

log_info "Confirmed testing PR artifact build"

# Verify go-fdo-server subpackages are installed
log_info "Verifying go-fdo-server installation..."
if ! rpm -q go-fdo-server-manufacturer; then
    log_error "go-fdo-server-manufacturer package is not installed"
    exit 1
fi
if ! rpm -q go-fdo-server-rendezvous; then
    log_error "go-fdo-server-rendezvous package is not installed"
    exit 1
fi
if ! rpm -q go-fdo-server-owner; then
    log_error "go-fdo-server-owner package is not installed"
    exit 1
fi
log_info "go-fdo-server packages are installed"

# Check that go-fdo-client binary is available
log_info "Checking go-fdo-client binary..."
if ! command -v go-fdo-client &> /dev/null; then
    log_error "go-fdo-client binary not found in PATH"
    exit 1
fi
log_info "go-fdo-client binary found: $(which go-fdo-client)"

# Verify client help menu is accessible
log_info "Verifying go-fdo-client help menu..."
if ! go-fdo-client --help &>/dev/null; then
    log_error "Failed to display go-fdo-client help menu"
    exit 1
fi
log_info "go-fdo-client help menu is accessible"

# Create test directory
mkdir -p /tmp/fdo-test
cd /tmp/fdo-test

# Start FDO services
log_info "Setting up FDO server environment..."
log_info "Starting FDO Manufacturing Server..."
systemctl start go-fdo-server-manufacturer.service || {
    log_error "Failed to start FDO Manufacturing Server"
    journalctl -u go-fdo-server-manufacturer.service --no-pager
    exit 1
}

#TODO: Fix on server side
log_warn "Fixing certificate permissions"
chown go-fdo-server-manufacturer:go-fdo-server /etc/go-fdo-server/{manufacturer,device-ca}*.{crt,key}
chmod g+r /etc/go-fdo-server/device-ca.crt
chown go-fdo-server-owner:go-fdo-server /etc/go-fdo-server/owner*.{crt,key}
chmod g+r /etc/go-fdo-server/owner.crt


log_info "Starting FDO Rendezvous Server..."
systemctl start go-fdo-server-rendezvous.service || {
    log_error "Failed to start FDO Rendezvous Server"
    journalctl -u go-fdo-server-rendezvous.service --no-pager
    exit 1
}

log_info "Starting FDO Owner Server..."
systemctl start go-fdo-server-owner.service || {
    log_error "Failed to start FDO Owner Server"
    journalctl -u go-fdo-server-owner.service --no-pager
    exit 1
}

# Wait for services to be ready
sleep 5

# Check service status
log_info "Verifying FDO services are running..."
systemctl is-active --quiet go-fdo-server-manufacturer.service || {
    log_error "FDO Manufacturing Server is not active"
    journalctl -u go-fdo-server-manufacturer.service --no-pager
    exit 1
}

systemctl is-active --quiet go-fdo-server-rendezvous.service || {
    log_error "FDO Rendezvous Server is not active"
    journalctl -u go-fdo-server-rendezvous.service --no-pager
    exit 1
}

systemctl is-active --quiet go-fdo-server-owner.service || {
    log_error "FDO Owner Server is not active"
    journalctl -u go-fdo-server-owner.service --no-pager
    exit 1
}

log_info "All FDO services are running"

# Configure rendezvous info in manufacturer server (JSON format)
log_info "Configuring rendezvous info in manufacturing server..."
curl --fail --silent --show-error \
     --header 'Content-Type: text/plain' \
     --request POST \
     --data-raw '[{"dns":"rendezvous","device_port":"8041","owner_port":"8041","protocol":"http","ip":"127.0.0.1"}]' \
     "http://127.0.0.1:8038/api/v1/rvinfo" || {
    log_error "Failed to set rendezvous info"
    exit 1
}
log_info "Rendezvous info configured"

# Configure owner redirect in owner server (JSON format)
log_info "Configuring owner redirect in owner server..."
curl --fail --silent --show-error \
     --header 'Content-Type: text/plain' \
     --request POST \
     --data-raw '[{"dns":"owner","port":"8043","protocol":"http","ip":"127.0.0.1"}]' \
     "http://127.0.0.1:8043/api/v1/owner/redirect" || {
    log_error "Failed to set owner redirect"
    exit 1
}
log_info "Owner redirect configured"

# Test 1: Device Initialization (DI)
# Using default ports: manufacturer=8038, rendezvous=8041, owner=8043
# Note: 127.0.0.1 is intentional for e2e testing with local services
log_info "Testing Device Initialization (DI)..."
# devskim: ignore DS137138 - 127.0.0.1 is required for testing local FDO services
go-fdo-client device-init http://127.0.0.1:8038 \
    --device-info e2e-test-device \
    --key ec256 \
    --debug \
    --blob /tmp/cred.bin || {
    log_error "Device initialization failed"
    exit 1
}

# Verify credential blob was created
if [ ! -f /tmp/cred.bin ]; then
    log_error "Credential blob file was not created"
    exit 1
fi
log_info "Device initialization successful - credential blob created"

# Extract device GUID from credential blob
log_info "Extracting device GUID from credential blob..."
GUID=$(go-fdo-client print --blob /tmp/cred.bin | grep -oE '[0-9a-fA-F]{32}' | head -n1)
if [ -z "$GUID" ]; then
    log_error "Failed to extract device GUID"
    go-fdo-client print --blob /tmp/cred.bin
    exit 1
fi
log_info "Device GUID: ${GUID}"

# Download ownership voucher from Manufacturing server
log_info "Downloading ownership voucher from Manufacturing server..."
curl --fail --silent --show-error \
     "http://127.0.0.1:8038/api/v1/vouchers/${GUID}" \
     -o /tmp/fdo-test/voucher.pem || {
    log_error "Failed to download ownership voucher"
    exit 1
}
log_info "Ownership voucher downloaded"

# Upload ownership voucher to Owner server
log_info "Uploading ownership voucher to Owner server..."
curl --fail --silent --show-error \
     --request POST \
     --data-binary @/tmp/fdo-test/voucher.pem \
     "http://127.0.0.1:8043/api/v1/owner/vouchers" || {
    log_error "Failed to upload voucher to Owner"
    exit 1
}
log_info "Ownership voucher uploaded to Owner"

log_info "Waiting for TO0 to finish"
sleep 70

# Run device onboarding (TO1 + TO2)
log_info "Running device onboarding (TO1 + TO2)..."
go-fdo-client onboard \
    --key ec256 \
    --kex ECDH256 \
    --debug \
    --blob /tmp/cred.bin | tee /tmp/fdo-test/onboard.log || {
    log_error "Device onboarding failed"
    cat /tmp/fdo-test/onboard.log
    exit 1
}

# Verify onboarding completed successfully
if ! grep -q 'FIDO Device Onboard Complete' /tmp/fdo-test/onboard.log; then
    log_error "Onboarding did not complete successfully"
    cat /tmp/fdo-test/onboard.log
    exit 1
fi
log_info "Device onboarding completed successfully"

log_info "Retrieving logs"
get_logs

# Success
log_info "======================================="
log_info "Go FDO Client E2E Test PASSED"
log_info "======================================="
log_info "✓ go-fdo-client package installed correctly"
log_info "✓ go-fdo-client binary is functional"
log_info "✓ FDO server services started successfully"
log_info "✓ Rendezvous info configured"
log_info "✓ Owner redirect configured"
log_info "✓ Device initialization (DI) completed"
log_info "✓ Credential blob created and validated"
log_info "✓ Device GUID extracted: ${GUID}"
log_info "✓ Ownership voucher transferred"
log_info "✓ TO0 protocol completed"
log_info "✓ Device onboarding (TO1/TO2) completed"
log_info "✓ Full end-to-end FDO workflow validated"
log_info "======================================="

exit 0
