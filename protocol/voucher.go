// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package fdo

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"errors"
	"fmt"
	"hash"
	"slices"

	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
)

// ErrCryptoVerifyFailed indicates that the wrapping error originated from a
// case of cryptographic verification failing rather than a broken invariant.
var ErrCryptoVerifyFailed = errors.New("cryptographic verification failed")

// Voucher is the top level structure.
//
//	OwnershipVoucher = [
//	    OVProtVer:      protver,           ;; protocol version
//	    OVHeaderTag:    bstr .cbor OVHeader,
//	    OVHeaderHMac:   HMac,              ;; hmac[DCHmacSecret, OVHeader]
//	    OVDevCertChain: OVDevCertChainOrNull,
//	    OVEntryArray:   OVEntries
//	]
//
//	;; Device certificate chain
//	;; use null for Intel® EPID.
//	OVDevCertChainOrNull     = X5CHAIN / null  ;; CBOR null for Intel® EPID device key
//
//	;; Ownership voucher entries array
//	OVEntries = [ * OVEntry ]
type Voucher struct {
	Version   uint16
	Header    cbor.Bstr[VoucherHeader]
	Hmac      Hmac
	CertChain *[]*cbor.X509Certificate
	Entries   []cose.Sign1Tag[VoucherEntryPayload, []byte]
}

// VoucherHeader is the Ownership Voucher header, also used in TO1 protocol.
//
//	OVHeader = [
//	    OVHProtVer:        protver,        ;; protocol version
//	    OVGuid:            Guid,           ;; guid
//	    OVRVInfo:          RendezvousInfo, ;; rendezvous instructions
//	    OVDeviceInfo:      tstr,           ;; DeviceInfo
//	    OVPubKey:          PublicKey,      ;; mfg public key
//	    OVDevCertChainHash:OVDevCertChainHashOrNull
//	]
//
//	;; Hash of Device certificate chain
//	;; use null for Intel® EPID
//	OVDevCertChainHashOrNull = Hash / null     ;; CBOR null for Intel® EPID device key
type VoucherHeader struct {
	Version         uint16
	GUID            GUID
	RvInfo          [][]RvInstruction
	DeviceInfo      string
	ManufacturerKey PublicKey
	CertChainHash   *Hash
}

// Equal compares two ownership voucher headers for equality.
//
//nolint:gocyclo
func (ovh *VoucherHeader) Equal(otherOVH *VoucherHeader) bool {
	if ovh.Version != otherOVH.Version {
		return false
	}
	if !bytes.Equal(ovh.GUID[:], otherOVH.GUID[:]) {
		return false
	}
	if !slices.EqualFunc(ovh.RvInfo, otherOVH.RvInfo, func(directivesA, directivesB []RvInstruction) bool {
		return slices.EqualFunc(directivesA, directivesB, func(instA, instB RvInstruction) bool {
			return instA.Variable == instB.Variable && bytes.Equal(instA.Value, instB.Value)
		})
	}) {
		return false
	}
	if ovh.DeviceInfo != otherOVH.DeviceInfo {
		return false
	}
	if ovh.ManufacturerKey.Type != otherOVH.ManufacturerKey.Type {
		return false
	}
	if ovh.ManufacturerKey.Encoding != otherOVH.ManufacturerKey.Encoding {
		return false
	}
	if !bytes.Equal(ovh.ManufacturerKey.Body, otherOVH.ManufacturerKey.Body) {
		return false
	}
	if (ovh.CertChainHash == nil && otherOVH.CertChainHash != nil) || (ovh.CertChainHash != nil && otherOVH.CertChainHash == nil) {
		return false
	}
	if ovh.CertChainHash != nil {
		if ovh.CertChainHash.Algorithm != otherOVH.CertChainHash.Algorithm {
			return false
		}
		if !bytes.Equal(ovh.CertChainHash.Value, otherOVH.CertChainHash.Value) {
			return false
		}
	}
	return true
}

// VoucherEntryPayload is an entry in a voucher's list of recorded transfers.
//
// ;; ...each entry is a COSE Sign1 object with a payload
// OVEntry = CoseSignature
// $COSEProtectedHeaders //= (
//
//	1: OVSignType
//
// )
// $COSEPayloads /= (
//
//	OVEntryPayload
//
// )
// ;; ... each payload contains the hash of the previous entry
// ;; and the signature of the public key to verify the next signature
// ;; (or the Owner, in the last entry).
// OVEntryPayload = [
//
//	OVEHashPrevEntry: Hash,
//	OVEHashHdrInfo:   Hash,  ;; hash[GUID||DeviceInfo] in header
//	OVEExtra:         null / bstr .cbor OVEExtraInfo
//	OVEPubKey:        PublicKey
//
// ]
//
// OVEExtraInfo = { * OVEExtraInfoType: bstr }
// OVEExtraInfoType = int
//
// ;;OVSignType = Supporting COSE signature types
type VoucherEntryPayload struct {
	PreviousHash Hash
	HeaderHash   Hash
	Extra        *cbor.Bstr[ExtraInfo]
	PublicKey    PublicKey
}

// ExtraInfo may be used to pass additional supply-chain information along with
// the Ownership Voucher. The Device implicitly verifies the plaintext of
// OVEExtra along with the verification of the Ownership Voucher. An Owner
// which trusts the Device' verification of the Ownership Voucher may also
// choose to trust OVEExtra.
type ExtraInfo map[int][]byte

func (v *Voucher) shallowClone() *Voucher {
	return &Voucher{
		Version: v.Version,
		Header: *cbor.NewBstr(VoucherHeader{
			Version:         v.Header.Val.Version,
			GUID:            v.Header.Val.GUID,
			RvInfo:          v.Header.Val.RvInfo,
			DeviceInfo:      v.Header.Val.DeviceInfo,
			ManufacturerKey: v.Header.Val.ManufacturerKey,
			CertChainHash:   v.Header.Val.CertChainHash,
		}),
		Hmac:      v.Hmac,
		CertChain: v.CertChain,
		Entries:   v.Entries,
	}
}

// DevicePublicKey extracts the device's public key from from the certificate
// chain. Before calling this method, the voucher must be fully verified. For
// certain key types, such as Intel EPID, the public key will be nil.
func (v *Voucher) DevicePublicKey() (crypto.PublicKey, error) {
	if v.CertChain == nil {
		return nil, nil
	}
	if len(*v.CertChain) == 0 {
		return nil, errors.New("empty cert chain")
	}
	return (*v.CertChain)[0].PublicKey, nil
}

// OwnerPublicKey extracts the voucher owner's public key from either the
// header or the entries list.
func (v *Voucher) OwnerPublicKey() (crypto.PublicKey, error) {
	if len(v.Entries) == 0 {
		return v.Header.Val.ManufacturerKey.Public()
	}
	return v.Entries[len(v.Entries)-1].Payload.Val.PublicKey.Public()
}

// VerifyHeader checks that the OVHeader was not modified by comparing the HMAC
// generated using the secret from the device credentials.
func (v *Voucher) VerifyHeader(deviceCredential KeyedHasher) error {
	return hmacVerify(deviceCredential, v.Hmac, &v.Header.Val)
}

// VerifyDeviceCertChain using trusted roots. If roots is nil then the last
// certificate in the chain will be implicitly trusted.
func (v *Voucher) VerifyDeviceCertChain(roots *x509.CertPool) error {
	if v.CertChain == nil {
		return nil
	}
	if len(*v.CertChain) == 0 {
		return errors.New("empty cert chain")
	}
	chain := make([]*x509.Certificate, len(*v.CertChain))
	for i, cert := range *v.CertChain {
		chain[i] = (*x509.Certificate)(cert)
	}
	return verifyCertChain(chain, roots)
}

// VerifyCertChainHash uses the hash in the voucher header to verify that the
// certificate chain of the voucher has not been tampered with. This method
// should therefore not be called before VerifyHeader.
func (v *Voucher) VerifyCertChainHash() error {
	switch {
	case v.CertChain == nil && v.Header.Val.CertChainHash == nil:
		return nil
	case v.CertChain == nil || v.Header.Val.CertChainHash == nil:
		return errors.New("device cert chain and hash must both be present or both be absent")
	}

	cchash := v.Header.Val.CertChainHash

	var digest hash.Hash
	switch cchash.Algorithm {
	case Sha256Hash:
		digest = sha256.New()
	case Sha384Hash:
		digest = sha512.New384()
	default:
		return fmt.Errorf("unsupported hash algorithm: %s", cchash.Algorithm)
	}

	for _, cert := range *v.CertChain {
		if _, err := digest.Write(cert.Raw); err != nil {
			return fmt.Errorf("error computing hash: %w", err)
		}
	}

	if !hmac.Equal(digest.Sum(nil), cchash.Value) {
		return fmt.Errorf("%w: certificate chain hash did not match", ErrCryptoVerifyFailed)
	}
	return nil
}

// VerifyManufacturerKey by using a public key hash (generally stored as part
// of the device credential).
func (v *Voucher) VerifyManufacturerKey(keyHash Hash) error {
	var digest hash.Hash
	switch keyHash.Algorithm {
	case Sha256Hash:
		digest = sha256.New()
	case Sha384Hash:
		digest = sha512.New384()
	default:
		return fmt.Errorf("unsupported hash algorithm for hashing manufacturer public key: %s", keyHash.Algorithm)
	}
	if err := cbor.NewEncoder(digest).Encode(&v.Header.Val.ManufacturerKey); err != nil {
		return fmt.Errorf("error computing hash of manufacturer public key: %w", err)
	}
	if !hmac.Equal(digest.Sum(nil), keyHash.Value) {
		return fmt.Errorf("%w: manufacturer public key hash did not match", ErrCryptoVerifyFailed)
	}
	return nil
}

// VerifyManufacturerCertChain using trusted roots. If roots is nil then the
// last certificate in the chain will be implicitly trusted.
//
// If the manufacturer public key is X509 encoded rather than X5Chain, then
// this method will fail if a non-nil root certificate pool is given.
func (v *Voucher) VerifyManufacturerCertChain(roots *x509.CertPool) error {
	chain, err := v.Header.Val.ManufacturerKey.Chain()
	if err != nil {
		return fmt.Errorf("error parsing manufacturer public key: %w", err)
	}
	if chain == nil {
		if roots == nil {
			return nil
		}
		return fmt.Errorf("manufacturer public key could not be verified against given roots, because it was not an X5Chain")
	}
	return verifyCertChain(chain, roots)
}

// VerifyEntries checks the chain of signatures on each voucher entry payload.
func (v *Voucher) VerifyEntries() error {
	// Parse the public key from the voucher header
	mfgKey, err := v.Header.Val.ManufacturerKey.Public()
	if err != nil {
		return fmt.Errorf("error parsing manufacturer public key: %w", err)
	}

	// Voucher may have never been extended since manufacturing
	if len(v.Entries) == 0 {
		return nil
	}

	// For entry 0, the previous hash is computed on OVHeader||OVHeaderHMac
	var initialHash hash.Hash
	switch alg := v.Entries[0].Payload.Val.PreviousHash.Algorithm; alg {
	case Sha256Hash:
		initialHash = sha256.New()
	case Sha384Hash:
		initialHash = sha512.New384()
	default:
		return fmt.Errorf("unsupported hash algorithm for hashing initial previous hash of entry list: %s", alg)
	}
	if err := cbor.NewEncoder(initialHash).Encode(&v.Header.Val); err != nil {
		return fmt.Errorf("error computing initial entry hash, writing encoded header: %w", err)
	}
	if err := cbor.NewEncoder(initialHash).Encode(v.Hmac); err != nil {
		return fmt.Errorf("error computing initial entry hash, writing encoded header hmac: %w", err)
	}

	// Precompute SHA256/SHA384 checksum for header info
	headerInfo := append(v.Header.Val.GUID[:], []byte(v.Header.Val.DeviceInfo)...)
	headerInfo256Sum := sha256.Sum256(headerInfo)
	headerInfo384Sum := sha512.Sum384(headerInfo)
	headerInfoSums := map[HashAlg][]byte{
		Sha256Hash: headerInfo256Sum[:],
		Sha384Hash: headerInfo384Sum[:],
	}

	// Validate all entries
	return validateNextEntry(mfgKey, initialHash, headerInfoSums, 0, v.Entries)
}

// Validate each entry recursively
func validateNextEntry(prevOwnerKey crypto.PublicKey, prevHash hash.Hash, headerInfo map[HashAlg][]byte, i int, entries []cose.Sign1Tag[VoucherEntryPayload, []byte]) error {
	entry := entries[0].Untag()

	// Check payload has a valid COSE signature from the previous owner key
	if ok, err := entry.Verify(prevOwnerKey, nil, nil); err != nil {
		return fmt.Errorf("COSE signature for entry %d could not be verified: %w", i, err)
	} else if !ok {
		return fmt.Errorf("%w: COSE signature for entry %d did not match previous owner key", ErrCryptoVerifyFailed, i)
	}

	// Check payload's HeaderHash matches voucher header as hash[GUID||DeviceInfo]
	headerHash := entry.Payload.Val.HeaderHash
	if !hmac.Equal(headerHash.Value, headerInfo[headerHash.Algorithm]) {
		return fmt.Errorf("%w: voucher entry payload %d header hash did not match", ErrCryptoVerifyFailed, i-1)
	}

	// Check payload's PreviousHash matches the previous entry
	if !hmac.Equal(prevHash.Sum(nil), entry.Payload.Val.PreviousHash.Value) {
		return fmt.Errorf("%w: voucher entry payload %d previous hash did not match", ErrCryptoVerifyFailed, i-1)
	}

	// Succeed if no more entries
	if len(entries[1:]) == 0 {
		return nil
	}

	// Parse owner key for next iteration
	ownerKey, err := entry.Payload.Val.PublicKey.Public()
	if err != nil {
		return fmt.Errorf("error parsing public key of entry %d: %w", i-1, err)
	}

	// Hash payload for next iteration
	var payloadHash hash.Hash
	switch alg := entries[1].Payload.Val.PreviousHash.Algorithm; alg {
	case Sha256Hash:
		payloadHash = sha256.New()
	case Sha384Hash:
		payloadHash = sha512.New384()
	default:
		return fmt.Errorf("unsupported hash algorithm for hashing voucher entry payload: %s", alg)
	}
	if err := cbor.NewEncoder(payloadHash).Encode(entry.Tag()); err != nil {
		return fmt.Errorf("error computing hash of voucher entry payload: %w", err)
	}

	// Validate the next entry recursively
	return validateNextEntry(ownerKey, payloadHash, headerInfo, i+1, entries[1:])
}

// VerifyOwnerCertChain validates the certificate chain of the owner public key
// using trusted roots. If roots is nil then the last certificate in the chain
// will be implicitly trusted. If the public key is X509 encoded rather than
// X5Chain, then this method will fail if a non-nil root certificate pool is
// given.
func (e *VoucherEntryPayload) VerifyOwnerCertChain(roots *x509.CertPool) error {
	chain, err := e.PublicKey.Chain()
	if err != nil {
		return fmt.Errorf("error parsing voucher entry's owner public key: %w", err)
	}
	if chain == nil {
		if roots == nil {
			return nil
		}
		return fmt.Errorf("voucher entry's owner public key could not be verified against given roots, because it was not an X5Chain")
	}
	return verifyCertChain(chain, roots)
}

func verifyCertChain(chain []*x509.Certificate, roots *x509.CertPool) error {
	// All all intermediates (if any) to a pool
	intermediates := x509.NewCertPool()
	if len(chain) > 2 {
		for _, cert := range chain[1 : len(chain)-1] {
			intermediates.AddCert(cert)
		}
	}

	// Trust last certificate in chain if roots is nil
	if roots == nil {
		roots = x509.NewCertPool()
		roots.AddCert(chain[len(chain)-1])
	}

	// Return the result of (*x509.Certificate).Verify
	if _, err := chain[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	}); err != nil {
		return fmt.Errorf("%w: %w", ErrCryptoVerifyFailed, err)
	}

	return nil
}

// ExtendVoucher adds a new signed voucher entry to the list and returns the
// new extended voucher. Vouchers should be treated as immutable structures.
func ExtendVoucher[T PublicKeyOrChain](v *Voucher, owner crypto.Signer, nextOwner T, extra ExtraInfo) (*Voucher, error) {
	// This performs a shallow clone, which allows arrays, maps, and pointers
	// to have their contents modified and both the original and copied voucher
	// will see the modification. However, this function does not perform a
	// deep copy/clone of the voucher, because vouchers are generally not used
	// as mutable entities. Every reference type in a voucher - keys, device
	// certificate chain, etc. - is protected by some other signature or hash,
	// so it doesn't make sense to modify.
	xv := v.shallowClone()

	// Each key in the Ownership Voucher must copy the public key type from the
	// manufacturer’s key in OVHeader.OVPubKey, hash, and encoding (e.g., all
	// RSA2048RESTR, all RSAPKCS 3072, all ECDSA secp256r1 or all ECDSA
	// secp384r1). This restriction permits a Device with limited crypto
	// capabilities to verify all the signatures.
	ownerPub := owner.Public()
	switch ownerPub := ownerPub.(type) {
	case *ecdsa.PublicKey:
		if mfgKey, err := v.Header.Val.ManufacturerKey.Public(); err != nil {
			return nil, fmt.Errorf("error parsing manufacturer key from header: %w", err)
		} else if mfgPubKey, ok := mfgKey.(*ecdsa.PublicKey); !ok {
			return nil, fmt.Errorf("owner key for voucher extension did not match the type of the manufacturer key")
		} else if mfgPubKey.Curve != ownerPub.Curve {
			return nil, fmt.Errorf("owner key for voucher extension did not match the type and size/curve of the manufacturer key")
		}
	case *rsa.PublicKey:
		if mfgKey, err := v.Header.Val.ManufacturerKey.Public(); err != nil {
			return nil, fmt.Errorf("error parsing manufacturer key from header: %w", err)
		} else if mfgPubKey, ok := mfgKey.(*rsa.PublicKey); !ok {
			return nil, fmt.Errorf("owner key for voucher extension did not match the type of the manufacturer key")
		} else if mfgPubKey.Size() != ownerPub.Size() {
			return nil, fmt.Errorf("owner key for voucher extension did not match the type and size/curve of the manufacturer key")
		}
	default:
		return nil, fmt.Errorf("unsupported key type: %T", ownerPub)
	}

	// Create the next owner PublicKey structure
	nextOwnerPublicKey, err := newPublicKey(v.Header.Val.ManufacturerKey.Type, nextOwner)
	if err != nil {
		return nil, fmt.Errorf("error marshaling next owner public key: %w", err)
	}

	// Calculate the hash of the voucher header info
	headerInfo := sha512.Sum384(append(v.Header.Val.GUID[:], []byte(v.Header.Val.DeviceInfo)...))
	headerHash := Hash{Algorithm: Sha384Hash, Value: headerInfo[:]}

	// Calculate the hash of the previous entry
	digest := sha512.New384()
	if len(v.Entries) == 0 {
		// For entry 0, the previous hash is computed on OVHeader||OVHeaderHMac
		if err := cbor.NewEncoder(digest).Encode(&v.Header.Val); err != nil {
			return nil, fmt.Errorf("error computing initial entry hash, writing encoded header: %w", err)
		}
		if err := cbor.NewEncoder(digest).Encode(v.Hmac); err != nil {
			return nil, fmt.Errorf("error computing initial entry hash, writing encoded header hmac: %w", err)
		}
	} else {
		if err := cbor.NewEncoder(digest).Encode(v.Entries[len(v.Entries)-1].Tag()); err != nil {
			return nil, fmt.Errorf("error computing hash of voucher entry payload: %w", err)
		}
	}
	prevHash := Hash{Algorithm: Sha384Hash, Value: digest.Sum(nil)}

	// Create and sign next entry
	usePSS := v.Header.Val.ManufacturerKey.Type == RsaPssKeyType
	entry, err := newSignedEntry(owner, usePSS, VoucherEntryPayload{
		PreviousHash: prevHash,
		HeaderHash:   headerHash,
		Extra:        cbor.NewBstr(extra),
		PublicKey:    *nextOwnerPublicKey,
	})
	if err != nil {
		return nil, err
	}
	xv.Entries = append(xv.Entries, *entry)
	return xv, nil
}

func newSignedEntry(owner crypto.Signer, usePSS bool, payload VoucherEntryPayload) (*cose.Sign1Tag[VoucherEntryPayload, []byte], error) {
	var entry cose.Sign1Tag[VoucherEntryPayload, []byte]
	entry.Payload = cbor.NewByteWrap(payload)

	signOpts, err := signOptsFor(owner, usePSS)
	if err != nil {
		return nil, err
	}

	if err := entry.Sign(owner, nil, nil, signOpts); err != nil {
		return nil, fmt.Errorf("error signing voucher entry payload: %w", err)
	}

	return &entry, nil
}
