// SPDX-FileCopyrightText: (C) 2024 Intel Corporation
// SPDX-License-Identifier: Apache 2.0

package blob

import (
	"crypto/hmac"
	"hash"

	"github.com/fido-device-onboard/go-fdo"
)

// Hmac implements fdo.KeyHasher with an in-memory secret.
type Hmac []byte

var _ fdo.KeyedHasher = Hmac(nil)

// NewHmac returns a key-based hash (Hmac) using the given hash function
// some secret.
func (h Hmac) NewHmac(alg fdo.HashAlg) (hash.Hash, error) {
	return hmac.New(alg.HashFunc().New, []byte(h)), nil
}
