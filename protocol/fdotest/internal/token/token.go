// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

// Package token implements all server state interfaces possible using a
// stateless token.
package token

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"time"

	"github.com/fido-device-onboard/go-fdo"
	"github.com/fido-device-onboard/go-fdo/cbor"
	"github.com/fido-device-onboard/go-fdo/cose"
	"github.com/fido-device-onboard/go-fdo/kex"
)

type diState struct {
	Unique
	OVH   *fdo.VoucherHeader
	Chain []*cbor.X509Certificate
}

type to0State struct {
	Unique
	Nonce fdo.Nonce
}

type to1State struct {
	Unique
	Nonce  fdo.Nonce
	SigAlg cose.SignatureAlgorithm
}

type to2State struct {
	Unique
	GUID        fdo.GUID
	Replacement struct {
		GUID fdo.GUID
		Hmac fdo.Hmac
	}
	KeyExchange keyExchange `cbor:",flat2"`
	ProveDv     fdo.Nonce
	SetupDv     fdo.Nonce
	MTU         uint16
}

type keyExchange struct {
	Suite kex.Suite
	Sess  kex.Session
}

func (x keyExchange) FlatMarshalCBOR(w io.Writer) error {
	if err := cbor.NewEncoder(w).Encode(x.Suite); err != nil {
		return err
	}
	return cbor.NewEncoder(w).Encode(x.Sess)
}

func (x *keyExchange) FlatUnmarshalCBOR(r io.Reader) error {
	if err := cbor.NewDecoder(r).Decode(&x.Suite); err != nil {
		return fmt.Errorf("error decoding key exchange suite: %w", err)
	}

	// If no suite is set, the next value will be nil
	if x.Suite == "" {
		var v struct{}
		return cbor.NewDecoder(r).Decode(&v)
	}

	// Initialize a session with a valid cipher suite (any) to decode
	x.Sess = x.Suite.New(nil, 1)
	if err := cbor.NewDecoder(r).Decode(&x.Sess); err != nil {
		return fmt.Errorf("error decoding key exchange session: %w", err)
	}
	return nil
}

// CA for creating a device certificate chain
type CA struct {
	Key   crypto.Signer
	Chain []*x509.Certificate
}

// Service implements the fdo.TokenService interface and state interfaces
// that do not need to persist beyond a single protocol session.
type Service struct {
	HmacSecret []byte
	CAs        map[fdo.KeyType]CA
}

var _ fdo.TokenService = (*Service)(nil)
var _ fdo.DISessionState = (*Service)(nil)
var _ fdo.TO0SessionState = (*Service)(nil)
var _ fdo.TO1SessionState = (*Service)(nil)
var _ fdo.TO2SessionState = (*Service)(nil)

// NewService initializes a stateless token service with a random HMAC secret
// and self-signed CAs for the common key types.
func NewService() (*Service, error) {
	// Hmac secret
	var secret [64]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}

	// Private keys
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	ec256Key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	ec384Key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, err
	}

	// CA cert chains
	generateCA := func(key crypto.Signer) ([]*x509.Certificate, error) {
		template := &x509.Certificate{
			SerialNumber:          big.NewInt(1),
			Subject:               pkix.Name{CommonName: "Test CA"},
			NotBefore:             time.Now(),
			NotAfter:              time.Now().Add(30 * 365 * 24 * time.Hour),
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
		der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
		if err != nil {
			return nil, err
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
		return []*x509.Certificate{cert}, nil
	}
	rsaChain, err := generateCA(rsaKey)
	if err != nil {
		return nil, err
	}
	ec256Chain, err := generateCA(ec256Key)
	if err != nil {
		return nil, err
	}
	ec384Chain, err := generateCA(ec384Key)
	if err != nil {
		return nil, err
	}

	return &Service{
		HmacSecret: secret[:],
		CAs: map[fdo.KeyType]CA{
			fdo.Rsa2048RestrKeyType: {
				Key:   rsaKey,
				Chain: rsaChain,
			},
			fdo.RsaPkcsKeyType: {
				Key:   rsaKey,
				Chain: rsaChain,
			},
			fdo.RsaPssKeyType: {
				Key:   rsaKey,
				Chain: rsaChain,
			},
			fdo.Secp256r1KeyType: {
				Key:   ec256Key,
				Chain: ec256Chain,
			},
			fdo.Secp384r1KeyType: {
				Key:   ec384Key,
				Chain: ec384Chain,
			},
		},
	}, nil
}

// NewToken initializes state for a given protocol and return the
// associated token.
func (s Service) NewToken(ctx context.Context, proto fdo.Protocol) (string, error) {
	switch proto {
	case fdo.DIProtocol:
		return newToken[*diState](s.HmacSecret)
	case fdo.TO0Protocol:
		return newToken[*to0State](s.HmacSecret)
	case fdo.TO1Protocol:
		return newToken[*to1State](s.HmacSecret)
	case fdo.TO2Protocol:
		return newToken[*to2State](s.HmacSecret)
	default:
		return "", fmt.Errorf("unsupported protocol %s", proto)
	}
}

type contextKey struct{}

var key contextKey

// TokenContext injects a context with a token value so that it may be used
// for any of the XXXState interfaces.
func (s Service) TokenContext(parent context.Context, token string) context.Context {
	return context.WithValue(parent, key, &token)
}

// TokenFromContext gets the token value from a context. This is useful,
// because some TokenServices may allow token mutation, such as in the case
// of token-encoded state (i.e. JWTs/CWTs).
func (s Service) TokenFromContext(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(key).(*string)
	if !ok || token == nil {
		return "", false
	}
	return *token, true
}

// InvalidateToken destroys the state associated with a given token.
func (s Service) InvalidateToken(ctx context.Context) error {
	if token, ok := ctx.Value(key).(*string); ok && token != nil {
		*token = ""
	}
	return nil
}

// NewDeviceCertChain creates a device certificate chain based on info
// provided in the (non-normative) DI.AppStart message and also stores it
// in session state.
func (s Service) NewDeviceCertChain(ctx context.Context, info fdo.DeviceMfgInfo) ([]*x509.Certificate, error) {
	// Sign CSR
	csr := x509.CertificateRequest(info.CertInfo)
	if err := csr.CheckSignature(); err != nil {
		return nil, fmt.Errorf("invalid CSR: %w", err)
	}
	ca, ok := s.CAs[info.KeyType]
	if !ok {
		return nil, fmt.Errorf("unsupported key type %s", info.KeyType)
	}
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, fmt.Errorf("error generating certificate serial number: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Issuer:       ca.Chain[0].Subject,
		Subject:      csr.Subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(30 * 360 * 24 * time.Hour), // Matches Java impl
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.Chain[0], csr.PublicKey, ca.Key)
	if err != nil {
		return nil, fmt.Errorf("error signing CSR: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("error parsing signed device cert: %w", err)
	}
	chain := append([]*x509.Certificate{cert}, ca.Chain...)

	// Update state with cert chain
	if err := update(ctx, s, func(state *diState) error {
		state.Chain = make([]*cbor.X509Certificate, len(chain))
		for i, cert := range chain {
			state.Chain[i] = (*cbor.X509Certificate)(cert)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return chain, nil
}

// DeviceCertChain gets a device certificate chain from the current
// session.
func (s Service) DeviceCertChain(ctx context.Context) ([]*x509.Certificate, error) {
	return fetch(ctx, s, func(state diState) ([]*x509.Certificate, error) {
		if len(state.Chain) == 0 {
			return nil, fdo.ErrNotFound
		}
		chain := make([]*x509.Certificate, len(state.Chain))
		for i, cert := range state.Chain {
			chain[i] = (*x509.Certificate)(cert)
		}
		return chain, nil
	})
}

// SetIncompleteVoucherHeader stores an incomplete (missing HMAC) voucher
// header tied to a session.
func (s Service) SetIncompleteVoucherHeader(ctx context.Context, ovh *fdo.VoucherHeader) error {
	return update(ctx, s, func(state *diState) error {
		state.OVH = ovh
		return nil
	})
}

// IncompleteVoucherHeader gets an incomplete (missing HMAC) voucher header
// which has not yet been persisted.
func (s Service) IncompleteVoucherHeader(ctx context.Context) (*fdo.VoucherHeader, error) {
	return fetch(ctx, s, func(state diState) (*fdo.VoucherHeader, error) {
		if state.OVH == nil {
			return nil, fdo.ErrNotFound
		}
		return state.OVH, nil
	})
}

// SetTO0SignNonce sets the Nonce expected in TO0.OwnerSign.
func (s Service) SetTO0SignNonce(ctx context.Context, nonce fdo.Nonce) error {
	return update(ctx, s, func(state *to0State) error {
		state.Nonce = nonce
		return nil
	})
}

// TO0SignNonce returns the Nonce expected in TO0.OwnerSign.
func (s Service) TO0SignNonce(ctx context.Context) (fdo.Nonce, error) {
	return fetch(ctx, s, func(state to0State) (fdo.Nonce, error) {
		if state.Nonce == (fdo.Nonce{}) {
			return fdo.Nonce{}, fdo.ErrNotFound
		}
		return state.Nonce, nil
	})
}

// SetTO1ProofNonce sets the Nonce expected in TO1.ProveToRV.
func (s Service) SetTO1ProofNonce(ctx context.Context, nonce fdo.Nonce) error {
	return update(ctx, s, func(state *to1State) error {
		state.Nonce = nonce
		return nil
	})
}

// TO1ProofNonce returns the Nonce expected in TO1.ProveToRV.
func (s Service) TO1ProofNonce(ctx context.Context) (fdo.Nonce, error) {
	return fetch(ctx, s, func(state to1State) (fdo.Nonce, error) {
		if state.Nonce == (fdo.Nonce{}) {
			return fdo.Nonce{}, fdo.ErrNotFound
		}
		return state.Nonce, nil
	})
}

// SetGUID associates a voucher GUID with a TO2 session.
func (s Service) SetGUID(ctx context.Context, guid fdo.GUID) error {
	return update(ctx, s, func(state *to2State) error {
		state.GUID = guid
		return nil
	})
}

// GUID retrieves the GUID of the voucher associated with the session.
func (s Service) GUID(ctx context.Context) (fdo.GUID, error) {
	return fetch(ctx, s, func(state to2State) (fdo.GUID, error) {
		if state.GUID == (fdo.GUID{}) {
			return fdo.GUID{}, fdo.ErrNotFound
		}
		return state.GUID, nil
	})
}

// SetReplacementGUID stores the device GUID to persist at the end of TO2.
func (s Service) SetReplacementGUID(ctx context.Context, guid fdo.GUID) error {
	return update(ctx, s, func(state *to2State) error {
		state.Replacement.GUID = guid
		return nil
	})
}

// ReplacementGUID retrieves the device GUID to persist at the end of TO2.
func (s Service) ReplacementGUID(ctx context.Context) (fdo.GUID, error) {
	return fetch(ctx, s, func(state to2State) (fdo.GUID, error) {
		if state.Replacement.GUID == (fdo.GUID{}) {
			return fdo.GUID{}, fdo.ErrNotFound
		}
		return state.Replacement.GUID, nil
	})
}

// SetReplacementHmac stores the voucher HMAC to persist at the end of TO2.
func (s Service) SetReplacementHmac(ctx context.Context, hmac fdo.Hmac) error {
	return update(ctx, s, func(state *to2State) error {
		state.Replacement.Hmac = hmac
		return nil
	})
}

// ReplacementHmac retrieves the voucher HMAC to persist at the end of TO2.
func (s Service) ReplacementHmac(ctx context.Context) (fdo.Hmac, error) {
	return fetch(ctx, s, func(state to2State) (fdo.Hmac, error) {
		if state.Replacement.Hmac.Algorithm == 0 {
			return fdo.Hmac{}, fdo.ErrNotFound
		}
		return state.Replacement.Hmac, nil
	})
}

// SetSession updates the current key exchange/encryption session based on an
// opaque "authorization" token.
func (s Service) SetSession(ctx context.Context, suite kex.Suite, sess kex.Session) error {
	return update(ctx, s, func(state *to2State) error {
		state.KeyExchange.Suite, state.KeyExchange.Sess = suite, sess
		return nil
	})
}

// Session returns the current key exchange/encryption session based on an
// opaque "authorization" token.
func (s Service) Session(ctx context.Context, token string) (kex.Suite, kex.Session, error) {
	state, err := fromToken[to2State](token, s.HmacSecret)
	if err != nil {
		return "", nil, err
	}
	if state.KeyExchange.Sess == nil {
		return "", nil, fdo.ErrNotFound
	}
	return state.KeyExchange.Suite, state.KeyExchange.Sess, nil
}

// SetProveDeviceNonce stores the Nonce used in TO2.ProveDevice for use in
// TO2.Done.
func (s Service) SetProveDeviceNonce(ctx context.Context, nonce fdo.Nonce) error {
	return update(ctx, s, func(state *to2State) error {
		state.ProveDv = nonce
		return nil
	})
}

// ProveDeviceNonce returns the Nonce used in TO2.ProveDevice and TO2.Done.
func (s Service) ProveDeviceNonce(ctx context.Context) (fdo.Nonce, error) {
	return fetch(ctx, s, func(state to2State) (fdo.Nonce, error) {
		if state.ProveDv == (fdo.Nonce{}) {
			return fdo.Nonce{}, fdo.ErrNotFound
		}
		return state.ProveDv, nil
	})
}

// SetSetupDeviceNonce stores the Nonce used in TO2.SetupDevice for use in
// TO2.Done2.
func (s Service) SetSetupDeviceNonce(ctx context.Context, nonce fdo.Nonce) error {
	return update(ctx, s, func(state *to2State) error {
		state.SetupDv = nonce
		return nil
	})
}

// SetupDeviceNonce returns the Nonce used in TO2.SetupDevice and TO2.Done2.
func (s Service) SetupDeviceNonce(ctx context.Context) (fdo.Nonce, error) {
	return fetch(ctx, s, func(state to2State) (fdo.Nonce, error) {
		if state.SetupDv == (fdo.Nonce{}) {
			return fdo.Nonce{}, fdo.ErrNotFound
		}
		return state.SetupDv, nil
	})
}

// ExtendVoucher adds a new signed voucher entry to the list and returns the
// new extended vouchers. Vouchers should be treated as immutable structures.
func (s Service) ExtendVoucher(ov *fdo.Voucher, nextOwner crypto.PublicKey) (*fdo.Voucher, error) {
	keyType := ov.Header.Val.ManufacturerKey.Type
	ca, ok := s.CAs[keyType]
	if !ok {
		return nil, fmt.Errorf("signed by unsupported key type %s", keyType)
	}
	switch nextOwner := nextOwner.(type) {
	case *ecdsa.PublicKey:
		return fdo.ExtendVoucher(ov, ca.Key, nextOwner, nil)
	case *rsa.PublicKey:
		return fdo.ExtendVoucher(ov, ca.Key, nextOwner, nil)
	case []*x509.Certificate:
		return fdo.ExtendVoucher(ov, ca.Key, nextOwner, nil)
	default:
		return nil, fmt.Errorf("invalid public key type %T", nextOwner)
	}
}

// SetMTU sets the max service info size the device may receive.
func (s Service) SetMTU(ctx context.Context, mtu uint16) error {
	return update(ctx, s, func(state *to2State) error {
		state.MTU = mtu
		return nil
	})
}

// MTU returns the max service info size the device may receive.
func (s Service) MTU(ctx context.Context) (uint16, error) {
	return fetch(ctx, s, func(state to2State) (uint16, error) {
		if state.MTU == 0 {
			return 0, fdo.ErrNotFound
		}
		return state.MTU, nil
	})
}
