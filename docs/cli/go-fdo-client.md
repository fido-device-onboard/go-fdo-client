## go-fdo-client

FIDO Device Onboard (FDO) client

### Synopsis

Run an FDO client to initialize or onboard a device.

Use one of the subcommands to perform device initialization (DI) with a
manufacturer server, onboard a device via TO1/TO2, or print the stored
device credentials.

### Examples

```
  # Initialize a device with a manufacturer server:
  go-fdo-client device-init http://127.0.0.1:8038 --key ec256 --blob cred.bin

  # Onboard a previously initialized device:
  go-fdo-client onboard --key ec256 --kex ECDH256 --blob cred.bin
```

### Options

```
      --blob string     File path of device credential blob
      --config string   Path to configuration file (YAML or TOML)
      --debug           Print HTTP contents
  -h, --help            help for go-fdo-client
      --key string      Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]
      --tpm string      Use a TPM at path for device credential secrets
```

### SEE ALSO

* [go-fdo-client device-init](go-fdo-client_device-init.md)	 - Run device initialization (DI)
* [go-fdo-client onboard](go-fdo-client_onboard.md)	 - Run FDO TO1 and TO2 onboarding
* [go-fdo-client print](go-fdo-client_print.md)	 - Print device credentials

