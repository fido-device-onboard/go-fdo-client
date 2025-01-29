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
$ make build or go build -o fdo_client ./cmd/fdo_client
$ ./fdo_client

Usage:
  fdo_client [--] [options]

Client options:
  -blob string
        File path of device credential blob (default "cred.bin")
  -cipher suite
        Name of cipher suite to use for encryption (see usage) (default "A128GCM")
  -debug
        Print HTTP contents
  -di URL
        HTTP base URL for DI server
  -di-key string
        Key for device credential [options: ec256, ec384, rsa2048, rsa3072] (default "ec384")
  -di-key-enc string
        Public key encoding to use for manufacturer key [x509,x5chain,cose] (default "x509")
  -download dir
        A dir to download files into (FSIM disabled if empty)
  -echo-commands
        Echo all commands received to stdout (FSIM disabled if false)
  -insecure-tls
        Skip TLS certificate verification
  -kex suite
        Name of cipher suite to use for key exchange (see usage) (default "ECDH384")
  -print
        Print device credential blob and stop
  -rv-only
        Perform TO1 then stop
  -resale
        Perform resale
  -tpm path
        Use a TPM at path for device credential secrets
  -upload files
        List of dirs and files to upload files from, comma-separated and/or flag provided multiple times (FSIM disabled if empty)
  -wget-dir dir
        A dir to wget files into (FSIM disabled if empty)

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
Run the FDO client, specifying the DI URL (on linux systems, root is required to properly gather a device identifier):
```
./fdo_client -di-device-info=gotest -di http://127.0.0.1:8080 -debug
```
### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client -print
```

## Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

## Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode:
```
./fdo_client -rv-only -debug
```
### Run the FDO Client for End-to-End (E2E) Testing
Run the FDO client for E2E testing:
```
./fdo_client -debug
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
The suppoerted key type and key exchange must always be explicit through the -di-key and -kex flag.:
```
./fdo_client -di http://127.0.0.1:8080 -di-device-info=gotest -di-key ec256 -kex ECDH256 -tpm /dev/tpmrm0 -debug
```
>NOTE: fdo_client may require elevated privileges. Please use 'sudo' to execute.
### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client -tpm /dev/tpmrm0  -print
```

## Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

## Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode:
```
./fdo_client -rv-only -di-key ec256 -kex ECDH256 -tpm /dev/tpmrm0  -debug
```
### Run the FDO Client for End-to-End (E2E) Testing
Run the FDO client for E2E testing:
```
./fdo_client -di-key ec256 -kex ECDH256 -tpm /dev/tpmrm0  -debug
```

