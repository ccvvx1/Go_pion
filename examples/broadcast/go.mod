module broadcast

go 1.21.6

// go 1.21

require github.com/pion/interceptor v1.2.3

require github.com/pion/webrtc/v4 v4.0.13

require (
	github.com/google/uuid v1.6.0 // indirect
	github.com/pion/datachannel v1.5.10 // indirect
	github.com/pion/dtls/v3 v3.0.4 // indirect
	github.com/pion/ice/v4 v4.0.7 // indirect
	github.com/pion/logging v0.2.3 // indirect
	github.com/pion/mdns/v2 v2.0.7 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.15 // indirect
	github.com/pion/rtp v1.8.12 // indirect
	github.com/pion/sctp v1.8.37 // indirect
	github.com/pion/sdp/v3 v3.0.10 // indirect
	github.com/pion/srtp/v3 v3.0.4 // indirect
	github.com/pion/stun/v3 v3.0.0 // indirect
	github.com/pion/transport/v3 v3.0.7 // indirect
	github.com/pion/turn/v4 v4.0.0 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/crypto v0.33.0 // indirect
	golang.org/x/net v0.35.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
)

replace github.com/pion/webrtc/v4 => /content/GoWebrtc/webrtc // 本地路径
replace github.com/pion/interceptor => /content/GoWebrtc/interceptor
replace github.com/pion/datachannel => /content/GoWebrtc/datachannel // 本地路径
replace github.com/pion/dtls/v3 => /content/GoWebrtc/dtls
replace github.com/pion/ice/v4 => /content/GoWebrtc/ice // 本地路径
replace github.com/pion/logging => /content/GoWebrtc/logging
replace github.com/pion/mdns/v2 => /content/GoWebrtc/mdns // 本地路径
replace github.com/pion/randutil => /content/GoWebrtc/randutil
replace github.com/pion/rtcp => /content/GoWebrtc/rtcp // 本地路径
replace github.com/pion/sctp => /content/GoWebrtc/sctp
replace github.com/pion/rtp => /content/GoWebrtc/rtp // 本地路径
replace github.com/pion/sdp/v3 => /content/GoWebrtc/sdp

replace github.com/pion/srtp/v3 => /content/GoWebrtc/srtp // 本地路径
replace github.com/pion/stun/v3 => /content/GoWebrtc/stun
replace github.com/pion/transport/v3 => /content/GoWebrtc/transport // 本地路径
replace github.com/pion/turn/v4 => /content/GoWebrtc/turn

// 或
//replace github.com/Dreamacro/clash => gitlab.com/your-fork/dependency v1.2.4
