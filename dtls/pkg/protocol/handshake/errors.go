// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package handshake

import (
	"errors"

	"github.com/pion/dtls/v3/pkg/protocol"
)

// Typed errors.
var (
	errUnableToMarshalFragmented = &protocol.InternalError{
		Err: errors.New("unable to marshal fragmented handshakes"), //nolint:err113
	}
	errHandshakeMessageUnset = &protocol.InternalError{
		Err: errors.New("handshake message unset, unable to marshal"), //nolint:err113
	}
	errBufferTooSmall = &protocol.TemporaryError{
		Err: errors.New("buffer is too small"), //nolint:err113
	}
	errLengthMismatch = &protocol.InternalError{
		Err: errors.New("data length and declared length do not match"), //nolint:err113
	}
	errInvalidClientKeyExchange = &protocol.FatalError{
		Err: errors.New("unable to determine if ClientKeyExchange is a public key or PSK Identity"), //nolint:err113
	}
	errInvalidHashAlgorithm = &protocol.FatalError{
		Err: errors.New("invalid hash algorithm"), //nolint:err113
	}
	errInvalidSignatureAlgorithm = &protocol.FatalError{
		Err: errors.New("invalid signature algorithm"), //nolint:err113
	}
	errCookieTooLong = &protocol.FatalError{
		Err: errors.New("cookie must not be longer then 255 bytes"), //nolint:err113
	}
	errInvalidEllipticCurveType = &protocol.FatalError{
		Err: errors.New("invalid or unknown elliptic curve type"), //nolint:err113
	}
	errInvalidNamedCurve = &protocol.FatalError{
		Err: errors.New("invalid named curve"), //nolint:err113
	}
	errCipherSuiteUnset = &protocol.FatalError{
		Err: errors.New("server hello can not be created without a cipher suite"), //nolint:err113
	}
	errCompressionMethodUnset = &protocol.FatalError{
		Err: errors.New("server hello can not be created without a compression method"), //nolint:err113
	}
	errInvalidCompressionMethod = &protocol.FatalError{
		Err: errors.New("invalid or unknown compression method"), //nolint:err113
	}
	errNotImplemented = &protocol.InternalError{
		Err: errors.New("feature has not been implemented yet"), //nolint:err113
	}
)
