## go-fdo-client onboard

Run FDO TO1 and TO2 onboarding

### Synopsis


Run FDO TO1 and TO2 onboarding to transfer device ownership to the owner server.
The device must have been initialized (device-init) before running onboard.
At least one of --blob or --tpm is required to access device credentials.

```
go-fdo-client onboard [flags]
```

### Examples

```

  # Using CLI arguments:
  go-fdo-client onboard --key ec256 --kex ECDH256 --blob cred.bin

  # Using config file:
  go-fdo-client onboard --config config.yaml

  # Mix CLI and config (CLI takes precedence):
  go-fdo-client onboard --config config.yaml --cipher A256GCM
```

### Options

```
      --allow-credential-reuse       Allow credential reuse protocol during onboarding
      --cipher string                Name of cipher suite to use for encryption (see usage) (default "A128GCM")
      --default-working-dir string   Default working directory for all FSIMs (fdo.command, fdo.download, fdo.upload, fdo.wget) (default: current working directory)
      --enable-interop-test          Enable FIDO Alliance interop test module (fsim.Interop)
  -h, --help                         help for onboard
      --insecure-tls                 Skip TLS certificate verification
      --kex string                   Name of cipher suite to use for key exchange (see usage)
      --max-serviceinfo-size int     Maximum service info size to receive (default 1300)
      --resale                       Perform resale
      --to2-retry-delay duration     Delay between failed TO2 attempts when trying multiple Owner URLs from same RV directive (0=disabled)
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

