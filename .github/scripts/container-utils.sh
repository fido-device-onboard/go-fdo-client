#! /usr/bin/env bash

set -xeuo pipefail
shopt -s expand_aliases

creds_dir="/tmp/device-credentials"
alias go-fdo-client="docker run --rm --volume "${creds_dir}:${creds_dir}" --network fdo --workdir ${creds_dir} go-fdo-client"

source "$(dirname "${BASH_SOURCE[0]}")/fdo-utils.sh"


# When the device onboarding is running in a container we need
# to setup the RVIPAddress to the actual IP of the rendezvous
# container as 127.0.0.1 won't work.
rendezvous_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "rendezvous" | jq -r '.[]|.IPAddress')"
owner_ip="$(docker inspect --format='{{json .NetworkSettings.Networks}}' "owner" | jq -r '.[]|.IPAddress')"
