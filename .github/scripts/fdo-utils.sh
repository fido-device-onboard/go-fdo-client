#! /usr/bin/env bash

set -xeuo pipefail

creds_dir=/tmp/device-credentials
device_credentials=${creds_dir}/creds.bin

certs_dir=/tmp/certs

manufacturer_dns=manufacturer
manufacturer_ip=127.0.0.1
manufacturer_port=8038
manufacturer_key="${certs_dir}/manufacturer.key"
manufacturer_crt="${manufacturer_key/\.key/.crt}"
manufacturer_pub="${manufacturer_key/\.key/.pub}"
device_ca_key="${certs_dir}/device-ca.key"
device_ca_crt="${device_ca_key/\.key/.crt}"
device_ca_pub="${device_ca_key/\.key/.pub}"

rendezvous_dns=rendezvous
rendezvous_ip=127.0.0.1
rendezvous_port=8041

owner_dns=owner
owner_ip=127.0.0.1
owner_port=8043
owner_onboard_log="/tmp/onboarding-${owner_dns}.log"
owner_ov="/tmp/owner.ov"
owner_key="${certs_dir}/owner.key"
owner_crt="${owner_key/\.key/.crt}"
owner_pub="${owner_key/\.key/.pub}"

manufacturer_service="${manufacturer_dns}:${manufacturer_port}"
rendezvous_service="${rendezvous_dns}:${rendezvous_port}"
owner_service="${owner_dns}:${owner_port}"

generate_cert() {
  local key=$1
  local crt=$2
  local pub=$3
  local subj=$4
  openssl ecparam -name prime256v1 -genkey -outform der -out "${key}"
  openssl req -x509 -key "${key}" -keyform der -subj "${subj}" -days 365 -out "${crt}"
  openssl x509 -in "${crt}" -pubkey -noout -out "${pub}"
}

generate_certs() {
  mkdir -p "${certs_dir}"
  generate_cert "${manufacturer_key}" "${manufacturer_crt}" "${manufacturer_pub}" "/C=US/O=FDO/CN=Manufacturer"
  generate_cert "${device_ca_key}" "${device_ca_crt}" "${device_ca_pub}" "/C=US/O=FDO/CN=Device CA"
  generate_cert "${owner_key}" "${owner_crt}" "${owner_pub}" "/C=US/O=FDO/CN=Owner"
  ls -l "${certs_dir}"
  chmod a+r "${certs_dir}"/*
}

get_server_logs() {
  docker logs manufacturer
  docker logs rendezvous
  docker logs owner
}

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

wait_for_service() {
    local status
    local retry=0
    local -r interval=2
    local -r max_retries=5
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

setup_hostnames () {
  setup_hostname ${manufacturer_dns} ${manufacturer_ip}
  setup_hostname ${rendezvous_dns} ${rendezvous_ip}
  setup_hostname ${owner_dns} ${owner_ip}
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
  local manufacturer_service=$1
  local rendezvous_dns=$2
  local rendezvous_ip=$3
  local rendezvous_port=$4
  curl --fail --verbose --silent \
       --header 'Content-Type: text/plain' \
       --request POST \
       --data-raw "[{\"dns\":\"${rendezvous_dns}\",\"device_port\":\"${rendezvous_port}\",\"owner_port\":\"${rendezvous_port}\",\"protocol\":\"http\",\"ip\":\"${rendezvous_ip}\"}]" \
       "http://${manufacturer_service}/api/v1/rvinfo"
}

run_device_initialization() {
  rm -rf "${creds_dir}"
  mkdir -p "${creds_dir}"
  cd ${creds_dir}
  go-fdo-client --blob "${device_credentials}" --debug device-init "http://${manufacturer_service}" --device-info=gotest --key ec256
  cd -
}

get_device_guid () {
  go-fdo-client --blob "${device_credentials}" --debug print | grep GUID | awk '{print $2}'
}

get_ov_from_manufacturer () {
  local manufacturer_service=$1
  local guid=$2
  local output=$3
  curl --fail --verbose --silent "http://${manufacturer_service}/api/v1/vouchers/${guid}" -o "${output}"
}

set_owner_redirect_info () {
  local service=$1
  local ip=$2
  local port=$3
  curl --fail --verbose --silent "http://${service}/api/v1/owner/redirect" \
       --header 'Content-Type: text/plain' \
       --data-raw "[{\"dns\":\"${ip}\",\"port\":\"${port}\",\"protocol\":\"http\",\"ip\":\"${ip}\"}]"
}

send_ov_to_owner () {
  local owner_service=$1
  local output=$2
  curl --fail --verbose --silent "http://${owner_service}/api/v1/owner/vouchers" --data-binary "@${output}"
}

add_device_ca_cert() {
    local url="$1"
    local crt="$2"
    curl --fail --verbose --silent --insecure \
        --request POST \
        --header 'Content-Type: application/x-pem-file' \
        --data-binary @"${crt}" \
        "${url}/api/v1/device-ca"
}

run_to0 () {
  local owner_service=$1
  local guid=$2
  curl --fail --verbose --silent "http://${owner_service}/api/v1/to0/${guid}"
}

run_fido_device_onboard () {
  local log=$1
  go-fdo-client --blob "${device_credentials}" --debug onboard --key ec256 --kex ECDH256 | tee "${log}"
  grep 'FIDO Device Onboard Complete' "${log}"
}

test_onboarding () {
  setup_hostnames
  wait_for_fdo_servers_ready
  set_rendezvous_info ${manufacturer_service} ${rendezvous_dns} ${rendezvous_ip} ${rendezvous_port}
  add_device_ca_cert ${rendezvous_service} ${device_ca_crt}
  run_device_initialization
  guid=$(get_device_guid ${device_credentials})
  get_ov_from_manufacturer ${manufacturer_service} "${guid}" ${owner_ov}
  set_owner_redirect_info ${owner_service} ${owner_ip} ${owner_port}
  send_ov_to_owner ${owner_service} ${owner_ov}
  sleep 20
  run_fido_device_onboard ${owner_onboard_log}
}
