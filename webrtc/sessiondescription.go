// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package webrtc

import (
	"fmt"
	"github.com/pion/sdp/v3"
)

// SessionDescription is used to expose local and remote session descriptions.
type SessionDescription struct {
	Type SDPType `json:"type"`
	SDP  string  `json:"sdp"`

	// This will never be initialized by callers, internal use only
	parsed *sdp.SessionDescription
}

// Unmarshal is a helper to deserialize the sdp.
func (sd *SessionDescription) Unmarshal() (*sdp.SessionDescription, error) {
	fmt.Println("进入session描述符处理阶段")
	sd.parsed = &sdp.SessionDescription{}
	err := sd.parsed.UnmarshalString(sd.SDP)

	return sd.parsed, err
}
