#!/bin/bash
set -eox pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/utils.sh"

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
    sudo cp "${tmp_hosts}" /etc/hosts
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

prepare_env() {
  # Install go-fdo-server and other required packages if run this test script locally.
  # When run it in packit/tmt, these packages will be installed by tmt.
  if [[ ! -v "PACKIT_COPR_RPMS" ]]; then
    sudo dnf install -y golang make openssl curl git podman firewalld libvirt-client libvirt-daemon-kvm libvirt-daemon qemu-img qemu-kvm virt-install lorax jq gobject-introspection rpmbuild go-rpm-macros
    dnf=$(readlink $(command -v dnf)); [ "\${dnf}" = "dnf5" ] || dnf=dnf ; \
        rpm -q --whatprovides ${dnf}'-command(copr)' &> /dev/null || ${dnf} install -y ${dnf}'-command(copr)'; \
        ${dnf} copr enable -y '@fedora-iot/fedora-iot'; \
        ${dnf} install -y go-fdo-server go-fdo-server-manufacturer go-fdo-server-owner go-fdo-server-rendezvous sqlite; \
        ${dnf} copr disable -y @fedora-iot/fedora-iot
    [ "${ID}" != "centos" ] || sudo dnf install -y epel-release
  fi
  sudo systemctl start firewalld

  log_info "Configuring libvirt permissions"
  sudo tee /etc/polkit-1/rules.d/50-libvirt.rules >/dev/null <<'EOF'
polkit.addRule(function(action, subject) {
    if (action.id == "org.libvirt.unix.manage" &&
        subject.isInGroup("adm")) {
            return polkit.Result.YES;
    }
});
EOF

  log_info "Starting libvirt daemon"
  sudo systemctl start libvirtd
  if ! sudo virsh list --all >/dev/null; then
    echo "Failed to connect to libvirt" >&2
    exit 1
  fi

  # Setup libvirt network
  log_info "Setting up libvirt network"
  local network_xml="/tmp/integration.xml"
  sudo tee "${network_xml}" >/dev/null <<'EOF'
<network xmlns:dnsmasq='http://libvirt.org/schemas/network/dnsmasq/1.0'>
<name>integration</name>
<uuid>1c8fe98c-b53a-4ca4-bbdb-deb0f26b3579</uuid>
<forward mode='nat'>
  <nat>
    <port start='1024' end='65535'/>
  </nat>
</forward>
<bridge name='integration' zone='trusted' stp='on' delay='0'/>
<mac address='52:54:00:36:46:ef'/>
<ip address='192.168.100.1' netmask='255.255.255.0'>
  <dhcp>
    <range start='192.168.100.2' end='192.168.100.254'/>
    <host mac='34:49:22:B0:83:30' name='vm-1' ip='192.168.100.50'/>
    <host mac='34:49:22:B0:83:31' name='vm-2' ip='192.168.100.51'/>
    <host mac='34:49:22:B0:83:32' name='vm-3' ip='192.168.100.52'/>
  </dhcp>
</ip>
<dnsmasq:options>
  <dnsmasq:option value='dhcp-vendorclass=set:efi-http,HTTPClient:Arch:00016'/>
  <dnsmasq:option value='dhcp-option-force=tag:efi-http,60,HTTPClient'/>
  <dnsmasq:option value='dhcp-boot=tag:efi-http,&quot;http://192.168.100.1/httpboot/EFI/BOOT/BOOTX64.EFI&quot;'/>
</dnsmasq:options>
</network>
EOF

  # Define network if it doesn't exist
  if ! sudo virsh net-info integration >/dev/null 2>&1; then
    sudo virsh net-define "${network_xml}"
  fi

  # Start network if not active
  if [[ $(sudo virsh net-info integration | awk '/Active/ {print $2}') == "no" ]]; then
    sudo virsh net-start integration
  fi
}

. /etc/os-release
[[ "${ID}" = "centos" && "${VERSION_ID}" = "9" ]] || \
[[ "${ID}" = "fedora" && "${VERSION_ID}" = "41" ]] || \
journalctl_args+=("--invocation=0")

printenv | sort

trap get_logs EXIT

prepare_env

log_info "Setup hostnames..."
set_hostname manufacturer 192.168.100.1
set_hostname rendezvous 192.168.100.1
set_hostname owner 192.168.100.1

log_info "Starting FDO Servers..."
systemctl start go-fdo-server-manufacturer.service
systemctl start go-fdo-server-rendezvous.service
systemctl start go-fdo-server-owner.service 
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
rv_info="[{\"dns\":\"rendezvous\", \"device_port\":\"8041\", \"protocol\":\"http\", \"ip\":\"192.168.100.1\", \"owner_port\":\"8041\"}]"
configure_rv_info http://192.168.100.1:8038 "${rv_info}"
log_info "Rendezvous info configured"

# Configure owner redirect in owner server (JSON format)
log_info "Configuring owner redirect in owner server..."
owner_redirect="[{\"dns\":\"owner\", \"port\":\"8043\", \"protocol\":\"http\", \"ip\":\"192.168.100.1\"}]"
configure_owner_redirect http://192.168.100.1:8043 "${owner_redirect}"
log_info "Owner redirect configured"

# Add CA cert to rendezvous server
log_info "Adding CA cert to rendezvous server"
crt="/etc/pki/go-fdo-server/device-ca-example.crt"
curl --fail --verbose --silent --insecure \
  --request POST \
  --header 'Content-Type: application/x-pem-file' \
  --data-binary @"${crt}" \
  http://192.168.100.1:8041/api/v1/device-ca

# Determine OS and setup variables
source /etc/os-release
log_info "Detected OS: ${ID} ${VERSION_ID}"
ssh_options=(-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5)
ssh_key="id_rsa"
sudo ssh-keygen -f id_rsa -N "" -q -t rsa-sha2-256 -b 2048 <<<y
ssh_key_pub=$(cat "${ssh_key}.pub")

case "${ID}-${VERSION_ID}" in
  "fedora-43" | "fedora-44")
    bib_url="quay.io/centos-bootc/bootc-image-builder:latest"
    os_variant="fedora-rawhide"
    base_image_url="quay.io/fedora/fedora-bootc:${VERSION_ID}"
    boot_args="uefi"
    ;;
  "centos-9" | "centos-10")
    bib_url="quay.io/centos-bootc/bootc-image-builder:latest"
    os_variant="centos-stream${VERSION_ID}"
    base_image_url="quay.io/centos-bootc/centos-bootc:stream${VERSION_ID}"
    boot_args="uefi,firmware.feature0.name=secure-boot,firmware.feature0.enabled=no"
    ;;
  *)
    log_error "Unsupported distro: ${ID}-${VERSION_ID}"
    exit 1
    ;;
esac

# Building go-fdo-client rpm packages
log_info "Building go-fdo-client rpm packages"
pushd ../../.. && \
make rpm
sudo cp rpmbuild/rpms/"$(uname -m)"/*.rpm test/fmf/tests && popd

# Building bootc container with go-fdo-client installed
log_info "Building bootc container with go-fdo-client installed"
tee Containerfile >/dev/null <<EOF
FROM ${base_image_url}
COPY go-fdo-client-*.rpm /tmp/
RUN dnf install -y /tmp/go-fdo-client-*.rpm
EOF
podman build --retry=5 --retry-delay=10s -t "fdo-client-bootc:latest" -f Containerfile .

# Using bootc image builder to generate anaconda-iso
log_info "Using bootc image builder to generate anaconda-iso"
rm -fr output
mkdir -pv output
sudo podman run \
  --rm \
  -it \
  --privileged \
  --pull=newer \
  --security-opt label=type:unconfined_t \
  -v "$(pwd)/output:/output" \
  -v "/var/lib/containers/storage:/var/lib/containers/storage" \
  "${bib_url}" \
  --type anaconda-iso \
  --rootfs xfs \
  --use-librepo=true \
  "localhost/fdo-client-bootc:latest"

# Generating kickstart file and mkksiso
log_info "Add fdo client initialization step in iso kickstart file"
rm -fr /var/lib/libvirt/images/install.iso
isomount=$(mktemp -d)
sudo mount -v -o "ro" "output/bootiso/install.iso" "$isomount"
new_ks_file="bib.ks"
cat >"${new_ks_file}" <<EOFKS
text
$(cat "${isomount}/osbuild-base.ks")
$(cat "${isomount}/osbuild.ks")
EOFKS
sed -i '/%include/d' "${new_ks_file}"
sed -i '/%post --erroronfail/i\
user --name=admin --groups=wheel --homedir=/home/admin --iscrypted --password=\$6\$GRmb7S0p8vsYmXzH\$o0E020S.9JQGaHkszoog4ha4AQVs3sk8q0DvLjSMxoxHBKnB2FBXGQ/OkwZQfW/76ktHd0NX5nls2LPxPuUdl.' "${new_ks_file}"
sed -i "/%post --erroronfail/i\
sshkey --username admin \"${ssh_key_pub}\"" "${new_ks_file}"
sed -i "/bootc switch/a\
go-fdo-client --blob /boot/device_credential --debug device-init http://192.168.100.1:8038 --device-info=iot-device --key ec256" "${new_ks_file}"
sed -i '/bootc switch/a\
echo "admin ALL=(ALL) NOPASSWD:ALL" >> /etc/sudoers.d/admin' "${new_ks_file}"
log_info "==== New kickstart file ===="
cat "${new_ks_file}"
log_info "============================"
log_info "Writing new ISO"
sudo mkksiso -c "console=ttyS0,115200" --ks "$new_ks_file" "output/bootiso/install.iso" "/var/lib/libvirt/images/install.iso"
sudo umount -v "$isomount"
rm -rf "$isomount"

# Provision vm with bootc anaconda-iso
log_info "Provision and start virtual machine"
guest_ip="192.168.100.50"
sudo qemu-img create -f qcow2 "/var/lib/libvirt/images/disk.qcow2" 10G
sudo restorecon -Rv /var/lib/libvirt/images/
sudo virt-install --name="fdo-client-bootc" \
    --disk "path=/var/lib/libvirt/images/disk.qcow2,format=qcow2" \
    --ram 3072 \
    --vcpus 2 \
    --network "network=integration,mac=34:49:22:B0:83:30" \
    --os-type linux \
    --os-variant "${os_variant}" \
    --cdrom "/var/lib/libvirt/images/install.iso" \
    --boot "${boot_args}" \
    --nographics \
    --noautoconsole \
    --wait=-1 \
    --noreboot

# Wait for vm ssh connection
log_info "Starting VM..."
sudo virsh start "fdo-client-bootc"
if ! wait_for_ssh $guest_ip; then
    exit 1
fi

# Transfter voucher from manufacture server to owner server
log_info "Get device initialization voucher guid"
guid=$(curl --fail --silent "http://192.168.100.1:8038/api/v1/vouchers" | jq -r '.[0].guid')
log_info "Device initialized with GUID: ${guid}"

log_info "Download voucher from manufacture server and send it to owner server"
ov_file="${guid}.ov"
curl --fail --silent --show-error "http://192.168.100.1:8038/api/v1/vouchers/${guid}" -o "${ov_file}"
curl --fail --silent --show-error --request POST --data-binary "@${ov_file}" "http://192.168.100.1:8043/api/v1/owner/vouchers"
sleep 60

# Perform fdo onboarding
log_info "Running FIDO Device Onboard"
sudo ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${guest_ip}" \
    'set -o pipefail; sudo go-fdo-client --blob /boot/device_credential onboard --key ec256 --kex ECDH256 --debug | tee /tmp/onboarding.log'
if sudo ssh "${ssh_options[@]}" -i "${ssh_key}" "admin@${guest_ip}" 'grep -q "FIDO Device Onboard Complete" /tmp/onboarding.log'; then
    log_info "Onboarding verification successful"
else
    log_error "Onboarding failed - success message not found in log"
    exit 1
fi
exit 0
