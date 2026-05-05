# Installing and configuring the FIDO Device Onboard (FDO) client

## Overview

FDO enables zero-touch device onboarding and secure device identity management for edge deployments. The `go-fdo-client` is an application installed on the device that implements the device side of the FDO specification. It provides the following commands:

- **`device-init`** — register the device with a Manufacturer server and create its device credential
- **`onboard`** — transfer device ownership from the Manufacturer to the Owner server
- **`print`** — display the device credential
- **`help`** — display help for any command

## Prerequisites

- Linux system
- FDO servers deployed and accessible (Manufacturer, Rendezvous, Owner). See [Server Installation](https://github.com/fido-device-onboard/go-fdo-server/blob/main/docs/user-guide/README.md#server-installation) in the "FIDO Device Onboarding User Guide" for server deployment.
- Network connectivity between the device and FDO servers

## Installing the FDO client

Install the `go-fdo-client` package using your distribution's package manager:

```console
# dnf install -y go-fdo-client
```

> **Note:** If you plan to use a Trusted Platform Module (TPM) for credential storage, you must also install `tpm2-tools`. For more information, see [Using a TPM](#using-a-tpm).

## FDO workflow

> **Note:** The following commands use example values. Replace them with appropriate values for your environment. For detailed information on all available options, see [Configuration](#configuration).

The FDO workflow consists of two phases:

### Phase 1: Device initialization

Perform device initialization during device provisioning (for example, as part of image installation or initial device setup).

Run the `go-fdo-client` `device-init` command with the Manufacturer server URL. For example:

```console
# go-fdo-client device-init http://manufacturer.example.com:8038 \
    --blob /boot/device_credential \
    --key ec256
```

Where:

- `--blob` — path to the file where the device credential will be stored
- `http://manufacturer.example.com:8038` — URL of the Manufacturer server
- `--key` — cryptographic key type used to authenticate the device during the FDO protocol

This command registers the device with the Manufacturer server and stores the device credential at `/boot/device_credential`.

To verify that device initialization completed successfully, see [Verifying device credential](#verifying-device-credential).

> **Note:** Before onboarding can succeed:
> - The Ownership Voucher must be transferred from the Manufacturer server to the Owner server.
> - The Owner server must register with the Rendezvous server (via the TO0 protocol).
>
> These are server-side operations. For more information, see [Basic Onboarding Flow (Device DI → voucher → TO0 → TO2)](https://github.com/fido-device-onboard/go-fdo-server/blob/main/docs/user-guide/quick-start.md#basic-onboarding-flow-device-di--voucher--to0--to2), step 3 in the "Quickstart: Install, Configure and Run FDO".

### Phase 2: Onboarding at first boot

After device initialization and reboot, the device must run the onboarding command. In a production deployment, configure a one-shot systemd service to run onboarding automatically at first boot. For testing and development, you can run the command manually:

```console
# go-fdo-client onboard \
    --blob /boot/device_credential \
    --key ec256 \
    --kex ECDH256
```

Where:

- `--blob` — path to the device credential created during initialization
- `--key` — cryptographic key type used to authenticate the device during the FDO protocol. When using a TPM, this must match the type used during initialization.
- `--kex` — key exchange suite for secure communication between the device and the Owner server during onboarding

On success, the output contains:

```
FIDO Device Onboard Complete
```

After successful onboarding, the Owner server can invoke service modules on the device, such as running commands or downloading files. The specific actions are configured on the Owner server. For more information, see [Service modules](#service-modules).

> **Note:** The `onboard` command retries indefinitely until it succeeds or you press `Ctrl+C`.

## Configuration

You can configure the FDO client by using CLI flags, a configuration file, or a combination of both.

To see all available options for a specific command, run `go-fdo-client <command> --help`.

### Using a configuration file

Specify a YAML or TOML configuration file with the `--config` flag:

```console
$ go-fdo-client <command> \
    --config /etc/fdo/config.yaml
```

Both YAML (`.yaml`, `.yml`) and TOML (`.toml`) formats are supported. The file format is detected automatically from the file extension.

All file paths in the configuration file can be absolute paths or paths relative to the current working directory.

### Configuration precedence

Values are resolved in the following order (highest to lowest):

1. Positional arguments (for example, server URL for `device-init`)
2. CLI flags (for example, `--key`, `--kex`)
3. Configuration file values
4. Default values

### Configuration options

The following tables list all available options.

#### Global options

These options apply to all commands:

| Option | Type | Required | Description | Default |
|--------|------|----------|-------------|---------|
| `debug` | boolean | No | Enable debug logging (print HTTP contents) | `false` |
| `blob` | string | Yes (if `tpm` is not set) | File path of device credential blob | — |
| `tpm` | string | Yes (if `blob` is not set) | TPM device path for device credential secrets | — |
| `key` | string | Yes (for `device-init` and `onboard`) | Cryptographic key type for device credential: `ec256`, `ec384`, `rsa2048`, `rsa3072` | — |

#### Device initialization options

These options can be set as CLI flags or under the `device-init` section in the configuration file:

| Option | Type | Required | Description | Default |
|--------|------|----------|-------------|---------|
| `server-url` | string | Yes | Manufacturer server URL | — |
| `key-enc` | string | No | Public key encoding: `x509`, `x5chain`, `cose` | `x509` |
| `device-info` | string | No | Custom device information for the device credential | — |
| `device-info-mac` | string | No | Network interface name for MAC-based device ID (for example, `eth0`) | — |
| `insecure-tls` | boolean | No | Skip TLS certificate verification | `false` |
| `serial-number` | string | No | Device serial number. Auto-detected from the system if not specified. | — |

`device-info` and `device-info-mac` are mutually exclusive. If neither is specified, the serial number is used as device info.

#### Onboarding options

These options can be set as CLI flags or under the `onboard` section in the configuration file:

| Option | Type | Required | Description | Default |
|--------|------|----------|-------------|---------|
| `kex` | string | Yes | Key exchange suite (see below) | — |
| `cipher` | string | No | Encryption cipher suite (see below) | `A128GCM` |
| `default-working-dir` | string | No | Working directory for service modules | current directory |
| `insecure-tls` | boolean | No | Skip TLS certificate verification | `false` |
| `max-serviceinfo-size` | integer | No | Maximum service info size (0–65535 bytes) | `1300` |
| `allow-credential-reuse` | boolean | No | Allow credential reuse protocol during onboarding | `false` |
| `resale` | boolean | No | Perform resale/re-onboarding | `false` |
| `to2-retry-delay` | duration | No | Delay between onboarding retries (for example, `5s`, `1m`) | `0` (disabled) |

If specified, the `default-working-dir` option must be set to an absolute path to a writable directory.

##### Supported key exchange suites

- `ECDH256` — Elliptic Curve Diffie-Hellman with P-256
- `ECDH384` — Elliptic Curve Diffie-Hellman with P-384
- `ASYMKEX2048` — Asymmetric key exchange with 2048-bit RSA
- `ASYMKEX3072` — Asymmetric key exchange with 3072-bit RSA
- `DHKEXid14` — Diffie-Hellman group 14
- `DHKEXid15` — Diffie-Hellman group 15

##### Supported encryption cipher suites

- `A128GCM` — AES-128 in GCM mode (default)
- `A192GCM` — AES-192 in GCM mode
- `A256GCM` — AES-256 in GCM mode
- `COSEAES128CBC` — AES-128 in CBC mode (COSE)
- `COSEAES128CTR` — AES-128 in CTR mode (COSE)
- `COSEAES256CBC` — AES-256 in CBC mode (COSE)
- `COSEAES256CTR` — AES-256 in CTR mode (COSE)

### Configuration file examples

Example minimal configuration file in YAML format:

```yaml
blob: "/boot/device_credential"
key: "ec256"

device-init:
  server-url: "http://manufacturer.example.com:8038"

onboard:
  kex: "ECDH256"
```

Example configuration file with additional options in YAML format:

```yaml
blob: "/boot/device_credential"
key: "ec256"

device-init:
  server-url: "http://manufacturer.example.com:8038"
  key-enc: "x5chain"
  device-info: "edge-device-001"

onboard:
  kex: "ECDH256"
  cipher: "A256GCM"
  default-working-dir: "/var/fdo/working"
```

The same configuration in TOML format:

```toml
blob = "/boot/device_credential"
key = "ec256"

[device-init]
server-url = "http://manufacturer.example.com:8038"
key-enc = "x5chain"
device-info = "edge-device-001"

[onboard]
kex = "ECDH256"
cipher = "A256GCM"
default-working-dir = "/var/fdo/working"
```

## Service modules

During onboarding, the FDO Owner server can invoke the following service modules on the device:

| Module | Description |
|--------|-------------|
| `fdo.command` | Execute shell commands on the device. Commands run from `default-working-dir`. |
| `fdo.download` | Download files from the Owner server to the device. Relative paths resolve from `default-working-dir`. Temporary files are created in `default-working-dir`. |
| `fdo.upload` | Upload files from the device to the Owner server. Relative paths resolve from `default-working-dir`. |
| `fdo.wget` | Download files from an HTTP server to the device. Relative paths resolve from `default-working-dir`. Temporary files are created in `default-working-dir`. |

All service modules are enabled by default. The `default-working-dir` is configured as an onboarding option (see [Onboarding options](#onboarding-options)). The Owner server configuration determines which modules are invoked during onboarding. See [Service Info Configuration (FSIM Operations)](https://github.com/fido-device-onboard/go-fdo-server/blob/main/docs/user-guide/server-config.md#service-info-configuration-fsim-operations) in the "Configuration File Reference" for server-side configuration.

## Using a TPM

The FDO client can store the device credential in a TPM instead of a file. This provides hardware-backed protection for credential secrets.

Requirements:
- TPM 2.0 device (for example, `/dev/tpmrm0`)
- `tpm2-tools` package installed
- Root privileges

### Device initialization with TPM

```console
# go-fdo-client device-init http://manufacturer.example.com:8038 \
    --key ec256 \
    --tpm /dev/tpmrm0
```

To verify that the TPM credential was stored successfully, see [Verifying device credential](#verifying-device-credential).

### Onboarding with TPM

```console
# go-fdo-client onboard \
    --key ec256 \
    --kex ECDH256 \
    --tpm /dev/tpmrm0
```

## Verifying device credential

After successful device initialization, the device credential is stored on the device. Use the `print` command to inspect the device credential stored in a file:

```console
# go-fdo-client print \
    --blob /boot/device_credential
```

or with TPM:

```console
# go-fdo-client print \
    --tpm /dev/tpmrm0
```

## Clearing device credential

To remove a file-based device credential:

```console
# rm /boot/device_credential
```

To remove the credential stored in the TPM NV index:

```console
# tpm2_nvundefine 0x01D10001
```

## Troubleshooting

The following are common issues that can occur during device initialization or onboarding, and their solutions.

### Onboarding fails with "TO1 failed"

The Rendezvous server does not have a record for this device. This means the Owner server has not yet registered the device with the Rendezvous server. Verify that:

1. The Ownership Voucher was transferred from the Manufacturer server to the Owner server.
2. The Owner server has had sufficient time to register the device with the Rendezvous server. This does not happen immediately after voucher upload.
3. The Owner server is configured with the correct Rendezvous server address.
4. The correct Ownership Voucher was transferred. The GUID in the voucher must match the GUID in the device credential. Use `go-fdo-client print --blob /boot/device_credential` (or `--tpm /dev/tpmrm0` when using a TPM) to check the device GUID.

These are server-side operations. For more information, see [Basic Onboarding Flow (Device DI → voucher → TO0 → TO2)](https://github.com/fido-device-onboard/go-fdo-server/blob/main/docs/user-guide/quick-start.md#basic-onboarding-flow-device-di--voucher--to0--to2), step 3 in the "Quickstart: Install, Configure and Run FDO". The `onboard` command retries automatically. If it continues to fail, check the Owner and Rendezvous server logs.

### Onboarding fails with "no rendezvous information found that's usable for the device"

The device credential does not contain usable rendezvous information. This can happen if the Manufacturer server was not configured with rendezvous information before device initialization.

Verify the rendezvous information stored in the credential with `go-fdo-client print --blob /boot/device_credential` (or `--tpm /dev/tpmrm0` when using a TPM). Re-initialize the device after configuring the Manufacturer server with the correct rendezvous information.

### Onboarding fails with TLS errors

If the FDO servers use TLS with self-signed certificates, use the `--insecure-tls` flag. This flag is available for both `device-init` and `onboard` commands:

```console
# go-fdo-client onboard \
    --blob /boot/device_credential \
    --key ec256 \
    --kex ECDH256 \
    --insecure-tls
```

> **Warning:** The `--insecure-tls` flag disables TLS certificate verification. Use only in test environments.

### TPM errors

Common TPM issues:

- **"failed to read credential from TPM"** — The TPM NV index does not contain a valid credential. Run `device-init` with the `--tpm` flag first.
- **Permission denied on `/dev/tpmrm0`** — The FDO client requires root privileges for TPM operations. Run the command with `sudo` privileges.
- **TPM NV index corrupted or inaccessible** — If the TPM NV index is in an inconsistent state, clear it manually with `tpm2_nvundefine 0x01D10001` and re-run `device-init`.

### Enabling debug logging

To diagnose issues, enable debug logging to see HTTP request and response content:

```console
# go-fdo-client onboard \
    --blob /boot/device_credential \
    --key ec256 \
    --kex ECDH256 \
    --debug
```

## CLI reference

The `go-fdo-client` command-line interface is built with subcommands for each phase of the FDO workflow. Each subcommand has its own set of flags and options. For complete usage details, including all available flags and examples, see the full CLI reference documentation:

- [go-fdo-client](cli/go-fdo-client.md) — top-level command and global options
- [go-fdo-client device-init](cli/go-fdo-client_device-init.md) — device initialization (DI)
- [go-fdo-client onboard](cli/go-fdo-client_onboard.md) — TO1/TO2 onboarding
- [go-fdo-client print](cli/go-fdo-client_print.md) — display device credentials
