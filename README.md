# FIDO Device Onboard - Go Client

`go-fdo-client` is a client implementation of FIDO Device Onboard specification in Go using [FDO GO protocols.](https://github.com/fido-device-onboard/go-fdo)

[fdo]: https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/FIDO-Device-Onboard-PS-v1.1-20220419.html
[cbor]: https://www.rfc-editor.org/rfc/rfc8949.html
[cose]: https://datatracker.ietf.org/doc/html/rfc8152

## Prerequisites

- Go 1.23.0 or later
- A Go module initialized with `go mod init`


The `update-deps.sh` script updates all dependencies in your Go module to their latest versions and cleans up the `go.mod` and `go.sum` files.

To update your dependencies, simply run the script:
```sh
./update-deps.sh
```

## Building the Client Application

The client application can be built with `make build` or `go build` directly,

```console
$ make build or go build -o fdo_client
$ ./fdo_client

FIDO Device Onboard Client

Usage:
  fdo_client [command]

Available Commands:
  device-init Run device initialization (DI)
  help        Help about any command
  onboard     Run FDO TO1 and TO2 onboarding
  print       Print device credential blob and exit

Flags:
      --blob string   File path of device credential blob
      --debug         Print HTTP contents
  -h, --help          help for fdo_client
      --tpm string    Use a TPM at path for device credential secrets

Use "fdo_client [command] --help" for more information about a command.

$ ./fdo_client device-init -h

Run device initialization (DI)

Usage:
  fdo_client device-init [flags]

Flags:
      --di string                   HTTP base URL for DI server (default "http://127.0.0.1:8080")
      --di-device-info string       Device information for device credentials, if not specified, it'll be gathered from the system
      --di-device-info-mac string   Mac-address's iface e.g. eth0 for device credentials
      --di-key string               Key for device credential [options: ec256, ec384, rsa2048, rsa3072]
      --di-key-enc string           Public key encoding to use for manufacturer key [x509,x5chain,cose] (default "x509")
  -h, --help                        help for device-init
      --insecure-tls                Skip TLS certificate verification

Global Flags:
      --blob string   File path of device credential blob
      --debug         Print HTTP contents
      --tpm string    Use a TPM at path for device credential secrets

$ ./fdo_client onboard -h
Run FDO TO1 and TO2 onboarding

Usage:
  fdo_client onboard [flags]

Flags:
      --cipher string     Name of cipher suite to use for encryption (see usage) (default "A128GCM")
      --di-key string     Key for device credential [options: ec256, ec384, rsa2048, rsa3072]
      --download string   A dir to download files into (FSIM disabled if empty)
      --echo-commands     Echo all commands received to stdout (FSIM disabled if false)
  -h, --help              help for onboard
      --insecure-tls      Skip TLS certificate verification
      --kex string        Name of cipher suite to use for key exchange (see usage)
      --resale            Perform resale
      --rv-only           Perform TO1 then stop
      --upload fsVar      List of dirs and files to upload files from, comma-separated and/or flag provided multiple times (FSIM disabled if empty) (default [])
      --wget-dir string   A dir to wget files into (FSIM disabled if empty)

Global Flags:
      --blob string   File path of device credential blob
      --debug         Print HTTP contents
      --tpm string    Use a TPM at path for device credential secrets

Key types:
  - RSA2048RESTR
  - RSAPKCS
  - RSAPSS
  - SECP256R1
  - SECP384R1

Encryption suites:
  - A128GCM
  - A192GCM
  - A256GCM
  - AES-CCM-64-128-128 (not implemented)
  - AES-CCM-64-128-256 (not implemented)
  - COSEAES128CBC
  - COSEAES128CTR
  - COSEAES256CBC
  - COSEAES256CTR

Key exchange suites:
  - DHKEXid14
  - DHKEXid15
  - ASYMKEX2048
  - ASYMKEX3072
  - ECDH256
  - ECDH384
```

## Running the FDO Client
### Remove Credential File
Remove the credential file if it exists:
```
rm cred.bin
```
### Run the FDO Client with DI URL
Run the FDO client, specifying the DI URL, key type and credentials blob file (on linux systems, root is required to properly gather a device identifier):
```
./fdo_client device-init --di-device-info=gotest --di http://127.0.0.1:8038 --di-key ec256 --debug --blob cred.bin
```
### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client print --blob cred.bin
```

## Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

## Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode:
```
./fdo_client onboard --rv-only --di-key ec256 --kex ECDH256 --debug --blob cred.bin
```
### Run the FDO Client for End-to-End (E2E) Testing
Run the FDO client for E2E testing:
```
./fdo_client --debug
```

## Running the FDO Client with TPM
### Clear TPM NV Index to Delete Existing Credential

Ensure `tpm2_tools` is installed on your system.

**Clear TPM NV Index**

   Use the following command to clear the TPM NV index:

   ```sh
   sudo tpm2_nvundefine 0x01D10001
   ```
### Run the FDO Client with DI URL
Run the FDO client, specifying the DI URL with the TPM resource manager path specified.
The supported key type and key exchange must always be explicit through the -di-key and -kex flag.:
```
./fdo_client device-init --di http://127.0.0.1:8080 --di-device-info=gotest --di-key ec256 --kex ECDH256 --tpm /dev/tpmrm0 --debug
```
>NOTE: fdo_client may require elevated privileges. Please use 'sudo' to execute.
### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client print --tpm /dev/tpmrm0
```

## Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

## Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode:
```
./fdo_client onboard --rv-only --di-key ec256 --kex ECDH256 --tpm /dev/tpmrm0  --debug
```
### Run the FDO Client for End-to-End (E2E) Testing
Run the FDO client for E2E testing:
```
./fdo_client --di-key ec256 --kex ECDH256 --tpm /dev/tpmrm0  --debug
```

