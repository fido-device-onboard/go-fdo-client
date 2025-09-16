# FDO Client RPM Installation Guide

This document covers installing and verifying FDO Client RPM packages.

The FDO Client RPM packaging creates one package:
- `go-fdo-client` - FDO client binary for device onboarding

This package will be installed from Fedora Official repos or COPR.

## 1. Installation

```bash
# To get the latest version enable COPR repository
sudo dnf install -y 'dnf-command(copr)'
sudo dnf copr enable -y @fedora-iot/fedora-iot

# Install package
sudo dnf install go-fdo-client
```

## 2. Verify Installation

```bash
# Check help
go-fdo-client --help

# Check installed location
which go-fdo-client
```

## 3. Basic Usage

### Device Initialization
```bash
go-fdo-client --blob /path/to/creds.bin device-init http://manufacturer:8038 --device-info "device-name" --key ec256
```

### Device Onboarding
```bash
go-fdo-client --blob /path/to/creds.bin onboard --key ec256 --kex ECDH256
```

### Print Device Info
```bash
go-fdo-client --blob /path/to/creds.bin print
```

## 4. Uninstall RPM

```bash
sudo dnf remove go-fdo-client
```
