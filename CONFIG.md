# Configuration File Reference

This document describes configuration options available for the FDO client. Configuration files can use TOML or YAML format.

Command line arguments take precedence over configuration file values. If no `--config` option is specified, the client uses defaults where available.

## Configuration File Usage

The configuration file must be specified via the `--config` command line parameter:

```bash
# Using YAML configuration file:
go-fdo-client device-init --config /path/to/config.yaml

# Using TOML configuration file:
go-fdo-client onboard --config /etc/fdo/config.toml

# CLI flags override config file values:
go-fdo-client device-init --config config.yaml --key ec256 https://example.com:8080
```

## Configuration Structure

The configuration file uses a hierarchical structure:

- Global options (`debug`, `blob`, `tpm`) - apply to all commands
- `device-init` - Device initialization specific configuration
- `onboard` - Onboarding (TO1/TO2) specific configuration

## Global Configuration

| Key | Type | Description | Default |
|-----|------|-------------|---------|
| `debug` | boolean | Enable debug logging (print HTTP contents) | false |
| `blob` | string | File path of device credential blob | - |
| `tpm` | string | TPM device path for device credential secrets | - |

**Note**: Either `blob` or `tpm` must be specified (via config file or CLI flag).

## Device Initialization Configuration

The device initialization configuration is under the `device-init` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `server-url` | string | DI server URL (e.g., `https://manufacturing.example.com:8080`) | Yes |
| `key` | string | Key type for device credential. Options: `ec256`, `ec384`, `rsa2048`, `rsa3072` | Yes |
| `key-enc` | string | Public key encoding. Options: `x509`, `x5chain`, `cose` | No (default: `x509`) |
| `device-info` | string | Custom device information for credentials | No |
| `device-info-mac` | string | MAC address interface name (e.g., `eth0`) for device info | No |
| `insecure-tls` | boolean | Skip TLS certificate verification | No (default: false) |

**Note**: `device-info` and `device-info-mac` are mutually exclusive. If neither is specified, device info is gathered automatically from the system.

## Onboard Configuration

The onboarding configuration is under the `onboard` section:

| Key | Type | Description | Required |
|-----|------|-------------|----------|
| `key` | string | Key type for device credential. Options: `ec256`, `ec384`, `rsa2048`, `rsa3072` | Yes |
| `kex` | string | Key exchange suite. Options: `DHKEXid14`, `DHKEXid15`, `ASYMKEX2048`, `ASYMKEX3072`, `ECDH256`, `ECDH384` | Yes |
| `cipher` | string | Cipher suite for encryption. Options: `A128GCM`, `A192GCM`, `A256GCM`, `AES-CCM-64-128-128`, `AES-CCM-64-128-256`, `COSEAES128CBC`, `COSEAES128CTR`, `COSEAES256CBC`, `COSEAES256CTR` | No (default: `A128GCM`) |
| `download` | string | Directory to download files into (FSIM disabled if empty) | No |
| `echo-commands` | boolean | Echo all commands received to stdout (FSIM disabled if false) | No (default: false) |
| `insecure-tls` | boolean | Skip TLS certificate verification | No (default: false) |
| `max-serviceinfo-size` | integer | Maximum service info size to receive (0-65535) | No (default: 1300) |
| `allow-credential-reuse` | boolean | Allow credential reuse protocol during onboarding | No (default: false) |
| `resale` | boolean | Perform resale/re-onboarding | No (default: false) |
| `to2-retry-delay` | duration | Delay between failed TO2 attempts (e.g., `5s`, `1m`) | No (default: 0, disabled) |
| `upload` | list of strings | Directories and files to upload from | No |
| `wget-dir` | string | Directory for wget file operations (FSIM disabled if empty) | No |

## Configuration File Examples

### YAML Configuration

```yaml
debug: true
blob: "cred.bin"

device-init:
  server-url: "https://manufacturing.example.com:8080"
  key: "ec384"
  key-enc: "x509"
  device-info: "device-001"
  insecure-tls: false

onboard:
  key: "ec384"
  kex: "ECDH384"
  cipher: "A256GCM"
  download: "/tmp/downloads"
  echo-commands: false
  insecure-tls: false
  max-serviceinfo-size: 1300
  allow-credential-reuse: false
  resale: false
  to2-retry-delay: "5s"
  upload:
    - "/path/to/file1"
    - "/path/to/dir1"
  wget-dir: "/tmp/wget"
```

### TOML Configuration

```toml
debug = true
blob = "cred.bin"

[device-init]
server-url = "https://manufacturing.example.com:8080"
key = "ec384"
key-enc = "x509"
device-info = "device-001"
insecure-tls = false

[onboard]
key = "ec384"
kex = "ECDH384"
cipher = "A256GCM"
download = "/tmp/downloads"
echo-commands = false
insecure-tls = false
max-serviceinfo-size = 1300
allow-credential-reuse = false
resale = false
to2-retry-delay = "5s"
upload = ["/path/to/file1", "/path/to/dir1"]
wget-dir = "/tmp/wget"
```

## Precedence Order

Configuration values are resolved in the following order (highest to lowest precedence):

1. **Positional arguments** (e.g., server URL for device-init)
2. **CLI flags** (e.g., `--key`, `--kex`)
3. **Configuration file values**
4. **Default values**

### Example

```bash
# Config file has server-url: "https://config.example.com:8080"
# Positional argument overrides config file:
go-fdo-client device-init --config config.yaml https://cli.example.com:9090

# Result: server-url = "https://cli.example.com:9090"
```

## Notes

- All file paths in the configuration should be absolute paths or paths relative to the current working directory
- Boolean values can be specified as `true`/`false` in both YAML and TOML
- Duration values use Go duration format (e.g., `5s`, `1m`, `2h30m`)
- The configuration file format is automatically detected based on file extension (`.yaml`, `.yml`, `.toml`)
