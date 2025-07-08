# FIDO Device Onboard - Go Client

`go-fdo-client` is a client implementation of FIDO Device Onboard specification in Go using [FDO GO protocols.](https://github.com/fido-device-onboard/go-fdo)

[fdo]: https://fidoalliance.org/specs/FDO/FIDO-Device-Onboard-PS-v1.1-20220419/FIDO-Device-Onboard-PS-v1.1-20220419.html
[cbor]: https://www.rfc-editor.org/rfc/rfc8949.html
[cose]: https://datatracker.ietf.org/doc/html/rfc8152

## Prerequisites

- Go 1.23.0 or later
- A Go module initialized with `go mod init`

## Building the Client Application

The client application can be built with `go build` directly,

```console
$ go build -o fdo_client
```

## FDO Client Command Syntax

```console
$ ./fdo_client -h

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
  fdo_client device-init <server-url> [flags]

Flags:
      --device-info string       Device information for device credentials, if not specified, it'll be gathered from the system
      --device-info-mac string   Mac-address's iface e.g. eth0 for device credentials
  -h, --help                     help for device-init
      --insecure-tls             Skip TLS certificate verification
      --key string               Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]
      --key-enc string           Public key encoding to use for manufacturer key [x509,x5chain,cose] (default "x509")

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
      --download string   A dir to download files into (FSIM disabled if empty)
      --echo-commands     Echo all commands received to stdout (FSIM disabled if false)
  -h, --help              help for onboard
      --insecure-tls      Skip TLS certificate verification
      --kex string        Name of cipher suite to use for key exchange (see usage)
      --key string        Key type for device credential [options: ec256, ec384, rsa2048, rsa3072]
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

## Running the FDO Client using a Credential File Blob
### Remove Credential File
Remove the credential file if it exists:
```
rm cred.bin
```
### Run the FDO Client with DI server URL
Run the FDO client, specifying the DI URL, key type and credentials blob file (on linux systems, root is required to properly gather a device identifier):
```
./fdo_client device-init http://127.0.0.1:8038 --device-info gotest --key ec256 --debug --blob cred.bin
```

### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client print --blob cred.bin
```

### Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

### Run the FDO Client onboard command
Perform FDO client onboard. The supported key type and key exchange suite must always be explicitly configured through the --key and --kex flags:
```
./fdo_client onboard --key ec256 --kex ECDH256 --debug --blob cred.bin
```

### Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode, which stops after TO1 is performed:
```
./fdo_client onboard --rv-only --key ec256 --kex ECDH256 --debug --blob cred.bin
```

## Running the FDO Client with a TPM device
>NOTE: fdo\_client may require elevated privileges to use the TPM device. Please use 'sudo' to execute the fdo\_client.

### Clear TPM NV Index to Delete Existing Credential

Ensure `tpm2_tools` is installed on your system.

**Clear TPM NV Index**

   Use the following command to clear the TPM NV index:

   ```sh
   sudo tpm2_nvundefine 0x01D10001
   ```
### Run the FDO Client device-init command with DI server URL
Run FDO client device-init, specifying the DI server URL with the TPM resource manager path specified.
The supported key type must always be explicitly configured through the --key flag:
```
./fdo_client device-init http://127.0.0.1:8038 --device-info gotest --key ec256 --tpm /dev/tpmrm0 --debug
```

### Print FDO Client Configuration or Status
Print the FDO client configuration or status:
```
./fdo_client print --tpm /dev/tpmrm0
```

### Execute TO0 from FDO Go Server
TO0 will be completed in the respective Owner and RV.

### Run the FDO Client onboard command
Perform FDO client onboard. The supported key type and key exchange suite must always be explicitly configured through the --key and --kex flags:
```
./fdo_client onboard --key ec256 --kex ECDH256 --tpm /dev/tpmrm0 --debug
```

### Optional: Run the FDO Client in RV-Only Mode
Run the FDO client in RV-only mode, which stops after TO1 is performed:
The supported key type and key exchange suite must always be explicitly configured through the --key and --kex flags:
```
./fdo_client onboard --rv-only --key ec256 --kex ECDH256 --tpm /dev/tpmrm0  --debug
```
