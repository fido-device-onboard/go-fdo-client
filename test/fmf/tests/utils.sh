#!/bin/bash

# Common Helper Functions for go-fdo-client Tests
# This library provides reusable functions for FDO client E2E testing
# Source this file in your test scripts: source utils.sh
#
# Aligned with go-fdo-server/test/ci/utils.sh for consistency

set -euo pipefail

#==============================================================================
# LOGGING FUNCTIONS
#==============================================================================

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} ⭐ $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} ❌ $1"
    return 1
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} ✔ $1"
}

log_debug() {
    if [ "${DEBUG:-0}" = "1" ]; then
        echo -e "${BLUE}[DEBUG]${NC} $1"
    fi
}

test_pass() {
    echo -e "${GREEN}[PASS]${NC} ✅ Test PASSED!"
}

test_fail() {
    echo -e "${RED}[FAIL]${NC} ❌ Test FAILED!"
    return 1
}

#==============================================================================
# ENVIRONMENT SETUP
#==============================================================================

set_hostname() {
    local dns=$1
    local ip=$2
    if grep -q " ${dns}" /etc/hosts; then
        tmp_hosts=$(mktemp)
        sed "s/.* ${dns}/${ip} ${dns}/" /etc/hosts >"${tmp_hosts}"
        cp "${tmp_hosts}" /etc/hosts
        rm -f "${tmp_hosts}"
    else
        echo "${ip} ${dns}" | sudo tee -a /etc/hosts > /dev/null
    fi
}


verify_fdo_packages() {
    log_info "Verifying FDO packages..."

    if ! rpm -q go-fdo-client &>/dev/null; then
        log_error "go-fdo-client package not installed"
        return 1
    fi

    if ! rpm -q go-fdo-server-manufacturer go-fdo-server-rendezvous go-fdo-server-owner &>/dev/null; then
        log_error "go-fdo-server packages not installed"
        return 1
    fi

    if ! command -v go-fdo-client &> /dev/null; then
        log_error "go-fdo-client binary not found in PATH"
        return 1
    fi

    log_info "All FDO packages verified"
    return 0
}

start_fdo_services() {
    log_info "Starting FDO services..."

    # Check if services are already running via port check
    local ports_map="manufacturer:8038 rendezvous:8041 owner:8043"
    local all_running=true

    for entry in $ports_map; do
        local service="${entry%:*}"
        local port="${entry#*:}"
        if ! ss -tln | grep -q ":${port} "; then
            all_running=false
            break
        fi
    done

    if $all_running; then
        log_info "FDO services already running (ports 8038, 8041, 8043 in use)"
        # Note: Tests expect fresh databases. If running locally and tests fail with
        # "rvInfo already exists", restart services to clean databases:
        # systemctl stop go-fdo-server-{manufacturer,rendezvous,owner}.service
        # rm -rf /var/lib/go-fdo-server-*/db.sqlite*
        # systemctl start go-fdo-server-{manufacturer,rendezvous,owner}.service
        return 0
    fi

    # Fix certificate permissions
    chown go-fdo-server-manufacturer:go-fdo-server /etc/go-fdo-server/{manufacturer,device-ca}*.{crt,key} 2>/dev/null || true
    chmod g+r /etc/go-fdo-server/device-ca.crt 2>/dev/null || true
    chown go-fdo-server-owner:go-fdo-server /etc/go-fdo-server/owner*.{crt,key} 2>/dev/null || true
    chmod g+r /etc/go-fdo-server/owner.crt 2>/dev/null || true

    for service in manufacturer rendezvous owner; do
        systemctl start go-fdo-server-${service}.service || {
            log_error "Failed to start ${service} server"
            journalctl -u go-fdo-server-${service}.service --no-pager
            return 1
        }
    done

    sleep 5

    # Verify services are running
    for service in manufacturer rendezvous owner; do
        systemctl is-active --quiet go-fdo-server-${service}.service || {
            log_error "go-fdo-server-${service} is not active"
            journalctl -u go-fdo-server-${service}.service --no-pager
            return 1
        }
    done

    log_info "All FDO services running"
    return 0
}

#==============================================================================
# RV INFO CONFIGURATION
#==============================================================================

configure_rv_info() {
    local manufacturer_url=$1
    local rv_json=$2
    local method="POST"

    # Check if rvinfo already exists
    if curl --fail --silent --show-error "${manufacturer_url}/api/v1/rvinfo" 2>/dev/null | grep -qv "No rvInfo found"; then
        method="PUT"  # Update existing
    fi

    curl --fail --silent --show-error \
         --header 'Content-Type: text/plain' \
         --request "${method}" \
         --data-raw "${rv_json}" \
         "${manufacturer_url}/api/v1/rvinfo" || {
        log_error "Failed to configure RV info"
        return 1
    }

    log_debug "RV info configured"
    return 0
}

configure_owner_redirect() {
    local owner_url=$1
    local redirect_json=$2
    local method="POST"

    # Check if owner redirect already exists
    if curl --fail --silent --show-error "${owner_url}/api/v1/owner/redirect" 2>/dev/null | grep -qv "No owner redirect found\|Not Found"; then
        method="PUT"  # Update existing
    fi

    curl --fail --silent --show-error \
         --header 'Content-Type: text/plain' \
         --request "${method}" \
         --data-raw "${redirect_json}" \
         "${owner_url}/api/v1/owner/redirect" || {
        log_error "Failed to configure owner redirect"
        return 1
    }

    log_debug "Owner redirect configured"
    return 0
}

#==============================================================================
# DEVICE OPERATIONS
#==============================================================================

device_init() {
    local device_info=$1
    local cred_file=$2

    go-fdo-client device-init http://127.0.0.1:8038 \
        --device-info "${device_info}" \
        --key ec256 \
        --debug \
        --blob "${cred_file}" || {
        log_error "Device initialization failed"
        return 1
    }

    if [ ! -f "${cred_file}" ]; then
        log_error "Credential blob not created"
        return 1
    fi

    log_debug "Device initialized: ${device_info}"
    return 0
}

get_guid() {
    local cred_file=$1

    local guid=$(go-fdo-client print --blob "${cred_file}" | grep -oE '[0-9a-fA-F]{32}' | head -n1)

    if [ -z "${guid}" ]; then
        log_error "Failed to extract GUID"
        return 1
    fi

    echo "${guid}"
    return 0
}

transfer_voucher() {
    local cred_file=$1
    local voucher_file=$2

    local guid=$(get_guid "${cred_file}") || return 1

    log_debug "GUID: ${guid}"

    # Download voucher from Manufacturing
    curl --fail --silent --show-error \
         "http://127.0.0.1:8038/api/v1/vouchers/${guid}" \
         -o "${voucher_file}" || {
        log_error "Failed to download voucher"
        return 1
    }

    # Upload voucher to Owner
    curl --fail --silent --show-error \
         --request POST \
         --data-binary "@${voucher_file}" \
         "http://127.0.0.1:8043/api/v1/owner/vouchers" || {
        log_error "Failed to upload voucher"
        return 1
    }

    log_debug "Voucher transferred for GUID: ${guid}"
    return 0
}

#==============================================================================
# LOG VALIDATION FUNCTIONS
#==============================================================================

verify_log_contains() {
    local log_file=$1
    local pattern=$2
    local description=$3

    if grep -q "${pattern}" "${log_file}"; then
        log_debug "✓ ${description}"
        return 0
    else
        log_error "✗ ${description}"
        log_error "Pattern not found: ${pattern}"
        return 1
    fi
}

verify_log_not_contains() {
    local log_file=$1
    local pattern=$2
    local description=$3

    if ! grep -q "${pattern}" "${log_file}"; then
        log_debug "✓ ${description}"
        return 0
    else
        log_error "✗ ${description}"
        log_error "Pattern found but should not be: ${pattern}"
        return 1
    fi
}

verify_log_count() {
    local log_file=$1
    local pattern=$2

    grep -c "${pattern}" "${log_file}" 2>/dev/null || echo "0"
}

verify_count_range() {
    local log_file=$1
    local pattern=$2
    local min=$3
    local max=$4
    local description=$5

    local count=$(verify_log_count "${log_file}" "${pattern}")

    if [ "${count}" -ge "${min}" ] && [ "${count}" -le "${max}" ]; then
        log_debug "✓ ${description} (count: ${count})"
        return 0
    else
        log_error "✗ ${description} (expected: ${min}-${max}, got: ${count})"
        return 1
    fi
}

verify_exit_code() {
    local expected=$1
    local actual=$2
    local description=$3

    if [ "${actual}" -eq "${expected}" ]; then
        log_debug "✓ ${description}"
        return 0
    else
        log_error "✗ ${description} (expected: ${expected}, got: ${actual})"
        return 1
    fi
}

#==============================================================================
# TEST UTILITIES
#==============================================================================

cleanup_test_files() {
    local cred_file=$1
    local voucher_file=$2

    rm -f "${cred_file}" "${voucher_file}"
    log_debug "Cleaned up test files"
}

wait_for_ssh() {
  local ip_address=$1
  local max_attempts=30
  local attempt=0

  log_info "Waiting for SSH on ${ip_address}..."
  while [[ ${attempt} -lt ${max_attempts} ]]; do
    if ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${ip_address}" 'echo -n "READY"' 2>/dev/null | grep -q "READY"; then
      log_success "SSH is ready"
      return 0
    fi
    attempt=$((attempt + 1))
    sleep 10
  done

  log_error "SSH connection timed out after $((max_attempts * 10)) seconds"
  return 1
}

