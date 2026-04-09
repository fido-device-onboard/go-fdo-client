## go-fdo-client device-init

Run device initialization (DI)

### Synopsis


Run device initialization (DI) to register the device with a manufacturer server.
The server URL can be provided as a positional argument, flag or via config file.
At least one of --blob or --tpm is required to store device credentials.

```
go-fdo-client device-init [server-url] [flags]
```

### Examples

```

  # Using CLI arguments:
  go-fdo-client device-init http://127.0.0.1:8038 --key ec256 --blob cred.bin

  # Using config file:
  go-fdo-client device-init --config config.yaml

  # Mix CLI and config (CLI takes precedence):
  go-fdo-client device-init http://127.0.0.1:8038 --config config.yaml --key ec384
```

### Options

```
      --device-info string       Device information for device credentials, if not specified, it'll be gathered from the system
      --device-info-mac string   Mac-address's iface e.g. eth0 for device credentials
  -h, --help                     help for device-init
      --insecure-tls             Skip TLS certificate verification
      --key-enc string           Public key encoding to use for manufacturer key [x509,x5chain,cose] (default "x509")
      --serial-number string     Serial number for device credentials, if not specified, it'll be gathered from the system
```

### Options inherited from parent commands

```
      --blob string     File path of device credential blob
      --config string   Path to configuration file (YAML or TOML)
      --debug           Print HTTP contents
      --key string      Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]
      --tpm string      Use a TPM at path for device credential secrets
```

### SEE ALSO

* [go-fdo-client](go-fdo-client.md)	 - FIDO Device Onboard (FDO) client

