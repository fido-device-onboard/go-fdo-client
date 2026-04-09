## go-fdo-client print

Print device credentials

### Synopsis

Print the contents of the device's credential store.

The device credentials are read from either a file (--blob) or a TPM device (--tpm)
and printed to standard output.

```
go-fdo-client print [flags]
```

### Examples

```
  # Print credentials from a blob file:
  go-fdo-client print --blob cred.bin

  # Print credentials from a TPM:
  go-fdo-client print --tpm /dev/tpmrm0
```

### Options

```
  -h, --help   help for print
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

