#!/bin/bash
set -euox pipefail

# Retry Loop Test for go-fdo-client
# Tests all retry loop behaviors: RV bypass, TO1, TO2, delays, and exit conditions
#
# IP Address Usage:
# - 127.0.0.1: Local FDO server instances (intentional for testing)
# - 192.0.2.x: IANA TEST-NET-1 reserved IPs (192.0.2.0/24) for unreachable addresses
#
# devskim: ignore DS137138

#==============================================================================
# SETUP
#==============================================================================

# Source common utilities
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

# Test directories
TEST_DIR="/tmp/fdo-retry-loop-test"
LOG_DIR="${TEST_DIR}/logs"
CRED_FILE="${TEST_DIR}/cred.bin"
VOUCHER_FILE="${TEST_DIR}/voucher.pem"

# Test counters
VALIDATIONS_PASSED=0
VALIDATIONS_FAILED=0
SCENARIO_NUM=0

#==============================================================================
# TEST-SPECIFIC FUNCTIONS
#==============================================================================

log_test() {
    SCENARIO_NUM=$((SCENARIO_NUM + 1))
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}SCENARIO ${SCENARIO_NUM}: $1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

log_pass() {
    VALIDATIONS_PASSED=$((VALIDATIONS_PASSED + 1))
    log_info "✓ $1"
}

log_fail() {
    VALIDATIONS_FAILED=$((VALIDATIONS_FAILED + 1))
    log_error "✗ $1"
}

# Wrapper functions that track pass/fail counts
verify_and_count() {
    local func=$1
    shift
    if $func "$@"; then
        VALIDATIONS_PASSED=$((VALIDATIONS_PASSED + 1))
        return 0
    else
        VALIDATIONS_FAILED=$((VALIDATIONS_FAILED + 1))
        return 1
    fi
}

journalctl_args=("--no-pager")
get_logs() {
    # Only show logs from today to reduce noise
    journalctl "${journalctl_args[@]}" --unit go-fdo-server-manufacturer.service --since today
    journalctl "${journalctl_args[@]}" --unit go-fdo-server-rendezvous.service --since today
    journalctl "${journalctl_args[@]}" --unit go-fdo-server-owner.service --since today
}

#==============================================================================
# TEST SCENARIOS
#==============================================================================

# Scenario 1: RV Bypass + TO1 Coexistence + default TO2 Retry Delay (0sec)
test_bypass_to1_coexistence() {
    log_test "RV Bypass + TO1 Coexistence + TO2 Retry Delays"
    local log_file="${LOG_DIR}/scenario1.log"

    cleanup_test_files "${CRED_FILE}" "${VOUCHER_FILE}"

    # Configure RV info with bypass (multiple wrong owners) + normal TO1 (correct owner)
    # First directive: RV bypass with MULTIPLE wrong owner URLs -> TO2 will fail multiple times with default 0s delay
    # Second directive: Normal TO1 with correct RV -> TO2 will succeed
    configure_rv_info '[
        {"rv_bypass": true, "ip": "192.0.2.1", "device_port": "8041", "owner_port": "8043", "protocol": "http", "delay_seconds": 2},
        {"ip": "127.0.0.1", "device_port": "8041", "owner_port": "8041", "protocol": "http"}
    ]' || return 1

    # Configure owner redirect with MULTIPLE unreachable owners for bypass directive
    # This will cause TO2 to fail to first two, then succeed on third
    # Default TO2 retry delay is 0 (no delay between attempts)
    configure_owner_redirect '[
        {"ip": "192.0.2.1", "port": "8043", "protocol": "http"},
        {"ip": "192.0.2.2", "port": "8043", "protocol": "http"},
        {"dns":"owner","port":"8043","protocol":"http","ip":"127.0.0.1"}
    ]' || return 1

    log_info "Adding certificate to rendezvous..."
    crt="/etc/pki/go-fdo-server/device-ca-example.crt"
    add_device_ca_cert http://127.0.0.1:8041 "${crt}"
    log_info "Certificate added to rendezvous"

    device_init "bypass-coexistence-test" "${CRED_FILE}" || return 1
    transfer_voucher "${CRED_FILE}" "${VOUCHER_FILE}" || return 1

    # Run onboarding (should succeed on second directive after bypass fails to all owners)
    go-fdo-client onboard \
        --key ec256 \
        --kex ECDH256 \
        --debug \
        --blob "${CRED_FILE}" 2>&1 | tee "${log_file}"

    local exit_code=$?

    # Validations
    verify_and_count verify_log_contains "${log_file}" "RV bypass enabled" "RV bypass enabled" || return 1
    verify_and_count verify_log_contains "${log_file}" "Using Owner URL from bypass directive" "Bypass Owner URL used" || return 1
    verify_and_count verify_count_range "${log_file}" "TO2 failed" 2 999 "Multiple TO2 failures" || return 1
    verify_and_count verify_log_not_contains "${log_file}" "Applying TO2 retry delay" "No TO2 retry delay" || return 1
    verify_and_count verify_log_contains "${log_file}" "Applying directive delay" "Directive delay applied" || return 1
    verify_and_count verify_log_contains "${log_file}" "Attempting TO1 protocol" "TO1 attempted" || return 1
    verify_and_count verify_log_contains "${log_file}" "TO1 succeeded" "TO1 succeeded" || return 1
    verify_and_count verify_log_contains "${log_file}" "TO2 succeeded" "TO2 succeeded" || return 1
    verify_and_count verify_log_contains "${log_file}" "FIDO Device Onboard Complete" "Onboarding completed" || return 1
    verify_and_count verify_exit_code 0 ${exit_code} "Exit code 0" || return 1

    log_info "Scenario 1 PASSED"
}

# Scenario 2: RVDelaysec Configured vs Default + User Interruption (Ctrl+C)
test_rvdelaysec_vs_default() {
    log_test "RVDelaysec Configured vs Default (120s) + User Interruption"
    local log_file="${LOG_DIR}/scenario2.log"

    cleanup_test_files "${CRED_FILE}" "${VOUCHER_FILE}"

    # Testing TO1 failures with configured vs default delay, then user interruption
    # First directive: TO1 fails with configured 5s delay
    # Second directive (LAST): TO1 fails, triggering default 120s delay (no delay_seconds on last directive)
    # Then user interrupts (Ctrl+C) after seeing default delay message
    configure_rv_info '[
        {"dev_only": true, "ip": "192.0.2.1", "device_port": "8041", "protocol": "http", "delay_seconds": 5},
        {"dev_only": true, "ip": "192.0.2.2", "device_port": "8041", "protocol": "http"}
    ]' || return 1

    log_info "Adding certificate to rendezvous..."
    crt="/etc/pki/go-fdo-server/device-ca-example.crt"
    add_device_ca_cert http://127.0.0.1:8041 "${crt}"
    log_info "Certificate added to rendezvous"

    device_init "delay-test" "${CRED_FILE}" || return 1

    go-fdo-client onboard \
        --key ec256 \
        --kex ECDH256 \
        --debug \
        --blob "${CRED_FILE}" > "${log_file}" 2>&1 &

    local onboard_pid=$!
    log_info "Started onboarding with PID: ${onboard_pid}"

    # Ensure we clean up the background process if test is interrupted
    trap "kill ${onboard_pid} 2>/dev/null || true" RETURN EXIT INT TERM

    # Wait for "Applying default delay for last directive" to appear, then send SIGINT
    local max_wait=180  # Maximum 180 seconds
    local elapsed=0
    local sigint_sent=false

    while [ $elapsed -lt $max_wait ]; do
        if grep -q "Applying default delay for last directive" "${log_file}" 2>/dev/null && [ "$sigint_sent" = "false" ]; then
            log_info "Detected default delay message, sending SIGINT (Ctrl+C) to PID ${onboard_pid}"
            sleep 2  # Give it 2 seconds to write more logs
            kill -INT ${onboard_pid} || true
            sigint_sent=true
            # Give process time to handle SIGINT and exit
            sleep 3
            break
        fi

        # Check if process is still running
        if ! ps -p ${onboard_pid} > /dev/null 2>&1; then
            log_info "Process ${onboard_pid} has exited"
            break
        fi

        sleep 1
        elapsed=$((elapsed + 1))
    done

    # Wait for process to finish (if it hasn't already)
    wait ${onboard_pid} 2>/dev/null || local exit_code=$?

    # If we hit max_wait without sending SIGINT, that's a test failure
    if [ "$sigint_sent" = "false" ]; then
        log_error "Timeout: never saw 'Applying default delay for last directive' message"
        return 1
    fi

    # Validations
    verify_and_count verify_log_contains "${log_file}" "Applying directive delay" "Configured delay applied" || return 1
    verify_and_count verify_log_contains "${log_file}" "Applying default delay for last directive" "Default delay applied" || return 1
    verify_and_count verify_log_contains "${log_file}" "All TO1 attempts failed for this directive" "TO1 failures logged" || return 1
    verify_and_count verify_count_range "${log_file}" "Attempting TO1 protocol" 2 999 "Multiple TO1 attempts" || return 1
    verify_and_count verify_log_contains "${log_file}" "Onboarding canceled by user" "SIGINT handled" || return 1

    log_info "Scenario 2 PASSED"
}

# Scenario 3: TO2 Retry Delay Configured (--to2-retry-delay flag)
test_to2_retry_delay_configured() {
    log_test "TO2 Retry Delay Configured (--to2-retry-delay 3s)"
    local log_file="${LOG_DIR}/scenario3.log"

    cleanup_test_files "${CRED_FILE}" "${VOUCHER_FILE}"

    # Configure single RV directive with working TO1
    configure_rv_info '[
        {"ip": "127.0.0.1", "device_port": "8041", "owner_port": "8041", "protocol": "http"}
    ]' || return 1

    # Configure owner redirect with multiple unreachable owners + final working owner
    # This will test TO2 retry delay between owner attempts
    configure_owner_redirect '[
        {"ip": "192.0.2.1", "port": "8043", "protocol": "http"},
        {"ip": "192.0.2.2", "port": "8043", "protocol": "http"},
        {"dns": "owner", "port": "8043", "protocol": "http", "ip": "127.0.0.1"}
    ]' || return 1

    log_info "Adding certificate to rendezvous..."
    crt="/etc/pki/go-fdo-server/device-ca-example.crt"
    add_device_ca_cert http://127.0.0.1:8041 "${crt}"
    log_info "Certificate added to rendezvous"

    device_init "to2-delay-test" "${CRED_FILE}" || return 1
    transfer_voucher "${CRED_FILE}" "${VOUCHER_FILE}" || return 1

    # Run onboarding with --to2-retry-delay flag
    # Should apply 3s delay between each TO2 owner attempt
    go-fdo-client onboard \
        --key ec256 \
        --kex ECDH256 \
        --debug \
        --blob "${CRED_FILE}" \
        --to2-retry-delay 3s 2>&1 | tee "${log_file}"

    local exit_code=$?

    # Validations
    verify_and_count verify_log_contains "${log_file}" "Applying TO2 retry delay" "TO2 retry delay applied" || return 1
    verify_and_count verify_count_range "${log_file}" "Applying TO2 retry delay" 2 999 "Multiple TO2 retry delays" || return 1
    verify_and_count verify_count_range "${log_file}" "TO2 failed" 2 999 "Multiple TO2 failures" || return 1
    verify_and_count verify_log_contains "${log_file}" "FIDO Device Onboard Complete" "Onboarding completed" || return 1
    verify_and_count verify_exit_code 0 ${exit_code} "Exit code 0" || return 1

    log_info "Scenario 3 PASSED"
}


#==============================================================================
# MAIN TEST EXECUTION
#==============================================================================

run_test() {
    log_info "================================================================"
    log_info "        GO-FDO-CLIENT RETRY LOOP TESTS"
    log_info "================================================================"

    # Setup
    . /etc/os-release
    [[ "${ID}" = "centos" && "${VERSION_ID}" = "9" ]] || \
    [[ "${ID}" = "fedora" && "${VERSION_ID}" = "41" ]] || \
    journalctl_args+=("--invocation=0")

    trap get_logs EXIT

    mkdir -p "${TEST_DIR}" "${LOG_DIR}"

    log_info "Setting up test hostnames..."
    set_hostname manufacturer 127.0.0.1
    set_hostname rendezvous 127.0.0.1
    set_hostname owner 127.0.0.1

    verify_fdo_packages || exit 1
    start_fdo_services || exit 1

    # Run all test scenarios (continue even if one fails to see all results)
    test_bypass_to1_coexistence || {
        log_fail "Scenario 1: RV Bypass + TO1 Coexistence - FAILED"
        VALIDATIONS_FAILED=$((VALIDATIONS_FAILED + 1))
    }

    test_rvdelaysec_vs_default || {
        log_fail "Scenario 2: RVDelaysec vs Default - FAILED"
        VALIDATIONS_FAILED=$((VALIDATIONS_FAILED + 1))
    }

    test_to2_retry_delay_configured || {
        log_fail "Scenario 3: TO2 Retry Delay - FAILED"
        VALIDATIONS_FAILED=$((VALIDATIONS_FAILED + 1))
    }

    # Final summary
    log_info "================================================================"
    log_info "                    TEST SUMMARY"
    log_info "================================================================"
    log_success "Validations Passed: ${VALIDATIONS_PASSED}"

    # Report result based on failures
    if [ ${VALIDATIONS_FAILED} -gt 0 ]; then
        # Has failures - show in red and fail test
        log_error "Validations Failed: ${VALIDATIONS_FAILED}" || true
        log_info "================================================================"
        test_fail
        exit 1
    else
        # No failures - show in neutral color and pass test
        log_info "Validations Failed: ${VALIDATIONS_FAILED}"
        log_info "================================================================"
        test_pass
        exit 0
    fi
}

# Allow running directly or sourcing for testing
[[ "${BASH_SOURCE[0]}" != "$0" ]] || run_test
