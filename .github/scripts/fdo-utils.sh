#! /usr/bin/env bash

set -xeuo pipefail

manufacturer_ip=127.0.0.1
rendezvous_ip=127.0.0.1
owner_ip=127.0.0.1

manufacturer_dns=manufacturer
rendezvous_dns=rendezvous
owner_dns=owner

manufacturer_service="${manufacturer_dns}:8038"
rendezvous_service="${rendezvous_dns}:8041"
owner_service="${owner_dns}:8043"

setup_hostname() {
  local dns
  local ip
  dns=$1
  ip=$2
  if grep -q "${dns}" /etc/hosts ; then
    sudo sed -ie "s/^((25[0-5]|(2[0-4]|1\d|[1-9]|)\d)\.?\b){4} ${dns}$/$ip $dns/" /etc/hosts
  else
    sudo echo "${ip} ${dns}" | sudo tee -a /etc/hosts;
  fi
}

setup_hostnames () {
  setup_hostname ${manufacturer_dns} ${manufacturer_ip}
  setup_hostname ${rendezvous_dns} ${rendezvous_ip}
  setup_hostname ${owner_dns} ${owner_ip}
}

wait_for_service() {
    local status
    local retry=0
    local -r interval=2
    local -r max_retries=1005
    local service=$1
    echo "Waiting for ${service} to be healthy"
    while true; do
        test "$(curl --silent --output /dev/null --write-out '%{http_code}' "http://${service}/health")" = "200" && break
        status=$?
        ((retry+=1))
        if [ $retry -gt $max_retries ]; then
            return $status
        fi
        echo "info: Waiting for a while, then retry ..." 1>&2
        sleep "$interval"
    done
}

wait_for_fdo_servers_ready () {
  # Manufacturer server
  wait_for_service "${manufacturer_service}"
  # Rendezvous server
  wait_for_service "${rendezvous_service}"
  # Owner server
  wait_for_service "${owner_service}"
}

set_rendezvous_info () {
    curl --fail --verbose --silent \
         --header 'Content-Type: text/plain' \
         --data-raw "[[[5,\"${rendezvous_dns}\"],[3,8041],[12,1],[2,\"${rendezvous_ip}\"],[4,8041]]]" \
         "http://${manufacturer_service}/api/v1/rvinfo"
}

run_device_initialization() {
  go-fdo-client --blob creds.bin --debug device-init "http://${manufacturer_service}" --device-info=gotest --key ec256
}

send_voucher_to_owner () {
  local guid
  guid=$(go-fdo-client --blob creds.bin --debug print | grep GUID | awk '{print $2}')
  curl --fail --verbose --silent "http://${manufacturer_service}/api/v1/vouchers?guid=${guid}" -o ownervoucher
  curl -X POST --fail --verbose --silent "http://${owner_service}/api/v1/owner/vouchers" --data-binary @ownervoucher
}

run_to0 () {
  local guid
  guid=$(go-fdo-client --blob creds.bin --debug print | grep GUID | awk '{print $2}')
  curl --fail --verbose --silent "http://${owner_service}/api/v1/to0/${guid}"
}

run_fido_device_onboard () {
  go-fdo-client --blob creds.bin --debug onboard --key ec256 --kex ECDH256 | tee onboarding.log
  grep 'FIDO Device Onboard Complete' onboarding.log
}

get_server_logs() {
  docker logs manufacturer
  docker logs rendezvous
  docker logs owner
}

test_onboarding () {
  setup_hostnames
  wait_for_fdo_servers_ready
  set_rendezvous_info
  run_device_initialization
  send_voucher_to_owner
  run_to0
  run_fido_device_onboard
}
