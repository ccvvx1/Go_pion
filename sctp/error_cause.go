// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package sctp

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// errorCauseCode is a cause code that appears in either a ERROR or ABORT chunk.
type errorCauseCode uint16

type errorCause interface {
	unmarshal([]byte) error
	marshal() ([]byte, error)
	length() uint16
	String() string

	errorCauseCode() errorCauseCode
}

// Error and abort chunk errors.
var (
	ErrBuildErrorCaseHandle = errors.New("BuildErrorCause does not handle")
)

// buildErrorCause delegates the building of a error cause from raw bytes to the correct structure.
func buildErrorCause(raw []byte) (errorCause, error) {
	var errCause errorCause

	c := errorCauseCode(binary.BigEndian.Uint16(raw[0:]))
	switch c {
	case invalidMandatoryParameter:
		errCause = &errorCauseInvalidMandatoryParameter{}
	case unrecognizedChunkType:
		errCause = &errorCauseUnrecognizedChunkType{}
	case protocolViolation:
		errCause = &errorCauseProtocolViolation{}
	case userInitiatedAbort:
		errCause = &errorCauseUserInitiatedAbort{}
	default:
		return nil, fmt.Errorf("%w: %s", ErrBuildErrorCaseHandle, c.String())
	}

	if err := errCause.unmarshal(raw); err != nil {
		return nil, err
	}

	return errCause, nil
}

const (
	invalidStreamIdentifier                errorCauseCode = 1
	missingMandatoryParameter              errorCauseCode = 2
	staleCookieError                       errorCauseCode = 3
	outOfResource                          errorCauseCode = 4
	unresolvableAddress                    errorCauseCode = 5
	unrecognizedChunkType                  errorCauseCode = 6
	invalidMandatoryParameter              errorCauseCode = 7
	unrecognizedParameters                 errorCauseCode = 8
	noUserData                             errorCauseCode = 9
	cookieReceivedWhileShuttingDown        errorCauseCode = 10
	restartOfAnAssociationWithNewAddresses errorCauseCode = 11
	userInitiatedAbort                     errorCauseCode = 12
	protocolViolation                      errorCauseCode = 13
)

func (e errorCauseCode) String() string { //nolint:cyclop
	switch e {
	case invalidStreamIdentifier:
		return "Invalid Stream Identifier"
	case missingMandatoryParameter:
		return "Missing Mandatory Parameter"
	case staleCookieError:
		return "Stale Cookie Error"
	case outOfResource:
		return "Out Of Resource"
	case unresolvableAddress:
		return "Unresolvable IP"
	case unrecognizedChunkType:
		return "Unrecognized Chunk Type"
	case invalidMandatoryParameter:
		return "Invalid Mandatory Parameter"
	case unrecognizedParameters:
		return "Unrecognized Parameters"
	case noUserData:
		return "No User Data"
	case cookieReceivedWhileShuttingDown:
		return "Cookie Received While Shutting Down"
	case restartOfAnAssociationWithNewAddresses:
		return "Restart Of An Association With New Addresses"
	case userInitiatedAbort:
		return "User Initiated Abort"
	case protocolViolation:
		return "Protocol Violation"
	default:
		return fmt.Sprintf("Unknown CauseCode: %d", e)
	}
}
