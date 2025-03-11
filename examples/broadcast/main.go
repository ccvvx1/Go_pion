// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

// broadcast demonstrates how to broadcast a video to many peers, while only requiring the broadcaster to upload once.
package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/intervalpli"
	"github.com/pion/webrtc/v4"
)

// nolint:gocognit, cyclop
func main() {
	port := flag.Int("port", 9080, "http server port")
	flag.Parse()

	sdpChan := httpSDPServer(*port)
	// fmt.Println("%+v", sdpChan)
	// fmt.Printf("%+v\n", p) 
	// Everything below is the Pion WebRTC API, thanks for using it ❤️.
	offer := webrtc.SessionDescription{}
	fmt.Printf("offer的结构体是%#v\n", offer) 
	// decode(<-sdpChan, &offer)
	tmpSdp := "eyJ0eXBlIjoib2ZmZXIiLCJzZHAiOiJ2PTBcclxubz0tIDEzMzY4MzcwMjkzOTM5MTI3NjAgMiBJTiBJUDQgMTI3LjAuMC4xXHJcbnM9LVxyXG50PTAgMFxyXG5hPWdyb3VwOkJVTkRMRSAwXHJcbmE9ZXh0bWFwLWFsbG93LW1peGVkXHJcbmE9bXNpZC1zZW1hbnRpYzogV01TIGMyMjdiYmFlLTMzMDgtNGViYi1iZjIzLTFiMzBkN2UxMGI2MFxyXG5tPXZpZGVvIDIyNjIgVURQL1RMUy9SVFAvU0FWUEYgOTYgOTcgMTAzIDEwNCAxMDcgMTA4IDEwOSAxMTQgMTE1IDExNiAxMTcgMTE4IDM5IDQwIDQ1IDQ2IDk4IDk5IDEwMCAxMDEgMTIzIDEyNCAxMjVcclxuYz1JTiBJUDQgMjIxLjE3Ni4zMy45NFxyXG5hPXJ0Y3A6OSBJTiBJUDQgMC4wLjAuMFxyXG5hPWNhbmRpZGF0ZToyNzAwNzY2NzAxIDEgdWRwIDIxMTM5MzcxNTEgODk1ODg3NzktZDYwNi00MTZhLTg0ZDUtNDhkODVmZTQwMjA4LmxvY2FsIDY1MTQ3IHR5cCBob3N0IGdlbmVyYXRpb24gMCBuZXR3b3JrLWNvc3QgOTk5XHJcbmE9Y2FuZGlkYXRlOjI5NTA3MTQ5MjUgMSB1ZHAgMTY3NzcyOTUzNSAyMjEuMTc2LjMzLjk0IDIyNjIgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgMCBnZW5lcmF0aW9uIDAgbmV0d29yay1jb3N0IDk5OVxyXG5hPWljZS11ZnJhZzp3YmZOXHJcbmE9aWNlLXB3ZDpuNEx4TkZ0eXVpdmRoU1kzTUVxMmdTTHhcclxuYT1pY2Utb3B0aW9uczp0cmlja2xlXHJcbmE9ZmluZ2VycHJpbnQ6c2hhLTI1NiA2ODpBRDpFRjpDODo4RTo5Rjo3MzpCMzpDQToyNDpFNzpEQjpGQzo0RTpCNDo4Njo3MjoxRDozMzo2RTo2MjowODo1RDo4OTo2NzpENDpEQzpENjpFNzozMDpFMzpGNFxyXG5hPXNldHVwOmFjdHBhc3NcclxuYT1taWQ6MFxyXG5hPWV4dG1hcDoxIHVybjppZXRmOnBhcmFtczpydHAtaGRyZXh0OnRvZmZzZXRcclxuYT1leHRtYXA6MiBodHRwOi8vd3d3LndlYnJ0Yy5vcmcvZXhwZXJpbWVudHMvcnRwLWhkcmV4dC9hYnMtc2VuZC10aW1lXHJcbmE9ZXh0bWFwOjMgdXJuOjNncHA6dmlkZW8tb3JpZW50YXRpb25cclxuYT1leHRtYXA6NCBodHRwOi8vd3d3LmlldGYub3JnL2lkL2RyYWZ0LWhvbG1lci1ybWNhdC10cmFuc3BvcnQtd2lkZS1jYy1leHRlbnNpb25zLTAxXHJcbmE9ZXh0bWFwOjUgaHR0cDovL3d3dy53ZWJydGMub3JnL2V4cGVyaW1lbnRzL3J0cC1oZHJleHQvcGxheW91dC1kZWxheVxyXG5hPWV4dG1hcDo2IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L3ZpZGVvLWNvbnRlbnQtdHlwZVxyXG5hPWV4dG1hcDo3IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L3ZpZGVvLXRpbWluZ1xyXG5hPWV4dG1hcDo4IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L2NvbG9yLXNwYWNlXHJcbmE9ZXh0bWFwOjkgdXJuOmlldGY6cGFyYW1zOnJ0cC1oZHJleHQ6c2RlczptaWRcclxuYT1leHRtYXA6MTAgdXJuOmlldGY6cGFyYW1zOnJ0cC1oZHJleHQ6c2RlczpydHAtc3RyZWFtLWlkXHJcbmE9ZXh0bWFwOjExIHVybjppZXRmOnBhcmFtczpydHAtaGRyZXh0OnNkZXM6cmVwYWlyZWQtcnRwLXN0cmVhbS1pZFxyXG5hPXNlbmRyZWN2XHJcbmE9bXNpZDpjMjI3YmJhZS0zMzA4LTRlYmItYmYyMy0xYjMwZDdlMTBiNjAgNTA5OTlmYmUtYWM3My00ZmI3LWJjYTUtM2UwMWJhYWNlOWUwXHJcbmE9cnRjcC1tdXhcclxuYT1ydGNwLXJzaXplXHJcbmE9cnRwbWFwOjk2IFZQOC85MDAwMFxyXG5hPXJ0Y3AtZmI6OTYgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjo5NiB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjk2IGNjbSBmaXJcclxuYT1ydGNwLWZiOjk2IG5hY2tcclxuYT1ydGNwLWZiOjk2IG5hY2sgcGxpXHJcbmE9cnRwbWFwOjk3IHJ0eC85MDAwMFxyXG5hPWZtdHA6OTcgYXB0PTk2XHJcbmE9cnRwbWFwOjEwMyBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjoxMDMgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMDMgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMDMgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTAzIG5hY2tcclxuYT1ydGNwLWZiOjEwMyBuYWNrIHBsaVxyXG5hPWZtdHA6MTAzIGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTE7cHJvZmlsZS1sZXZlbC1pZD00MjAwMWZcclxuYT1ydHBtYXA6MTA0IHJ0eC85MDAwMFxyXG5hPWZtdHA6MTA0IGFwdD0xMDNcclxuYT1ydHBtYXA6MTA3IEgyNjQvOTAwMDBcclxuYT1ydGNwLWZiOjEwNyBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjEwNyB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjEwNyBjY20gZmlyXHJcbmE9cnRjcC1mYjoxMDcgbmFja1xyXG5hPXJ0Y3AtZmI6MTA3IG5hY2sgcGxpXHJcbmE9Zm10cDoxMDcgbGV2ZWwtYXN5bW1ldHJ5LWFsbG93ZWQ9MTtwYWNrZXRpemF0aW9uLW1vZGU9MDtwcm9maWxlLWxldmVsLWlkPTQyMDAxZlxyXG5hPXJ0cG1hcDoxMDggcnR4LzkwMDAwXHJcbmE9Zm10cDoxMDggYXB0PTEwN1xyXG5hPXJ0cG1hcDoxMDkgSDI2NC85MDAwMFxyXG5hPXJ0Y3AtZmI6MTA5IGdvb2ctcmVtYlxyXG5hPXJ0Y3AtZmI6MTA5IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6MTA5IGNjbSBmaXJcclxuYT1ydGNwLWZiOjEwOSBuYWNrXHJcbmE9cnRjcC1mYjoxMDkgbmFjayBwbGlcclxuYT1mbXRwOjEwOSBsZXZlbC1hc3ltbWV0cnktYWxsb3dlZD0xO3BhY2tldGl6YXRpb24tbW9kZT0xO3Byb2ZpbGUtbGV2ZWwtaWQ9NDJlMDFmXHJcbmE9cnRwbWFwOjExNCBydHgvOTAwMDBcclxuYT1mbXRwOjExNCBhcHQ9MTA5XHJcbmE9cnRwbWFwOjExNSBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjoxMTUgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMTUgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMTUgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTE1IG5hY2tcclxuYT1ydGNwLWZiOjExNSBuYWNrIHBsaVxyXG5hPWZtdHA6MTE1IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTA7cHJvZmlsZS1sZXZlbC1pZD00MmUwMWZcclxuYT1ydHBtYXA6MTE2IHJ0eC85MDAwMFxyXG5hPWZtdHA6MTE2IGFwdD0xMTVcclxuYT1ydHBtYXA6MTE3IEgyNjQvOTAwMDBcclxuYT1ydGNwLWZiOjExNyBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjExNyB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjExNyBjY20gZmlyXHJcbmE9cnRjcC1mYjoxMTcgbmFja1xyXG5hPXJ0Y3AtZmI6MTE3IG5hY2sgcGxpXHJcbmE9Zm10cDoxMTcgbGV2ZWwtYXN5bW1ldHJ5LWFsbG93ZWQ9MTtwYWNrZXRpemF0aW9uLW1vZGU9MTtwcm9maWxlLWxldmVsLWlkPTRkMDAxZlxyXG5hPXJ0cG1hcDoxMTggcnR4LzkwMDAwXHJcbmE9Zm10cDoxMTggYXB0PTExN1xyXG5hPXJ0cG1hcDozOSBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjozOSBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjM5IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6MzkgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MzkgbmFja1xyXG5hPXJ0Y3AtZmI6MzkgbmFjayBwbGlcclxuYT1mbXRwOjM5IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTA7cHJvZmlsZS1sZXZlbC1pZD00ZDAwMWZcclxuYT1ydHBtYXA6NDAgcnR4LzkwMDAwXHJcbmE9Zm10cDo0MCBhcHQ9MzlcclxuYT1ydHBtYXA6NDUgQVYxLzkwMDAwXHJcbmE9cnRjcC1mYjo0NSBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjQ1IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6NDUgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6NDUgbmFja1xyXG5hPXJ0Y3AtZmI6NDUgbmFjayBwbGlcclxuYT1mbXRwOjQ1IGxldmVsLWlkeD01O3Byb2ZpbGU9MDt0aWVyPTBcclxuYT1ydHBtYXA6NDYgcnR4LzkwMDAwXHJcbmE9Zm10cDo0NiBhcHQ9NDVcclxuYT1ydHBtYXA6OTggVlA5LzkwMDAwXHJcbmE9cnRjcC1mYjo5OCBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjk4IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6OTggY2NtIGZpclxyXG5hPXJ0Y3AtZmI6OTggbmFja1xyXG5hPXJ0Y3AtZmI6OTggbmFjayBwbGlcclxuYT1mbXRwOjk4IHByb2ZpbGUtaWQ9MFxyXG5hPXJ0cG1hcDo5OSBydHgvOTAwMDBcclxuYT1mbXRwOjk5IGFwdD05OFxyXG5hPXJ0cG1hcDoxMDAgVlA5LzkwMDAwXHJcbmE9cnRjcC1mYjoxMDAgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMDAgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMDAgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTAwIG5hY2tcclxuYT1ydGNwLWZiOjEwMCBuYWNrIHBsaVxyXG5hPWZtdHA6MTAwIHByb2ZpbGUtaWQ9MlxyXG5hPXJ0cG1hcDoxMDEgcnR4LzkwMDAwXHJcbmE9Zm10cDoxMDEgYXB0PTEwMFxyXG5hPXJ0cG1hcDoxMjMgcmVkLzkwMDAwXHJcbmE9cnRwbWFwOjEyNCBydHgvOTAwMDBcclxuYT1mbXRwOjEyNCBhcHQ9MTIzXHJcbmE9cnRwbWFwOjEyNSB1bHBmZWMvOTAwMDBcclxuYT1zc3JjLWdyb3VwOkZJRCAxOTk2NDg1MTYwIDU2NTA0ODYxMFxyXG5hPXNzcmM6MTk5NjQ4NTE2MCBjbmFtZTpLVXBhL3dCckw0MU5Qa3VGXHJcbmE9c3NyYzoxOTk2NDg1MTYwIG1zaWQ6YzIyN2JiYWUtMzMwOC00ZWJiLWJmMjMtMWIzMGQ3ZTEwYjYwIDUwOTk5ZmJlLWFjNzMtNGZiNy1iY2E1LTNlMDFiYWFjZTllMFxyXG5hPXNzcmM6NTY1MDQ4NjEwIGNuYW1lOktVcGEvd0JyTDQxTlBrdUZcclxuYT1zc3JjOjU2NTA0ODYxMCBtc2lkOmMyMjdiYmFlLTMzMDgtNGViYi1iZjIzLTFiMzBkN2UxMGI2MCA1MDk5OWZiZS1hYzczLTRmYjctYmNhNS0zZTAxYmFhY2U5ZTBcclxuIn0="
	decode(tmpSdp, &offer)
	fmt.Println("收到外部信息")

	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	mediaEngine := &webrtc.MediaEngine{}
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		panic(err)
	}

	// Create a InterceptorRegistry. This is the user configurable RTP/RTCP Pipeline.
	// This provides NACKs, RTCP Reports and other features. If you use `webrtc.NewPeerConnection`
	// this is enabled by default. If you are manually managing You MUST create a InterceptorRegistry
	// for each PeerConnection.
	interceptorRegistry := &interceptor.Registry{}

	// Use the default set of Interceptors
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		panic(err)
	}

	// Register a intervalpli factory
	// This interceptor sends a PLI every 3 seconds. A PLI causes a video keyframe to be generated by the sender.
	// This makes our video seekable and more error resilent, but at a cost of lower picture quality and higher bitrates
	// A real world application should process incoming RTCP packets from viewers and forward them to senders
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		panic(err)
	}
	interceptorRegistry.Add(intervalPliFactory)

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	).NewPeerConnection(peerConnectionConfig)
	if err != nil {
		panic(err)
	}
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	// Allow us to receive 1 video track
	if _, err = peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
		panic(err)
	}

	localTrackChan := make(chan *webrtc.TrackLocalStaticRTP)
	// Set a handler for when a new remote track starts, this just distributes all our packets
	// to connected peers
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) { //nolint: revive
		// Create a local track, all our SFU clients will be fed via this track
		localTrack, newTrackErr := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, "video", "pion")
		if newTrackErr != nil {
			panic(newTrackErr)
		}
		localTrackChan <- localTrack

		rtpBuf := make([]byte, 1400)
		for {
			i, _, readErr := remoteTrack.Read(rtpBuf)
			if readErr != nil {
				panic(readErr)
			}

			// ErrClosedPipe means we don't have any subscribers, this is ok if no peers have connected yet
			if _, err = localTrack.Write(rtpBuf[:i]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				panic(err)
			}
		}
	})

	// Set the remote SessionDescription
	err = peerConnection.SetRemoteDescription(offer)
	if err != nil {
		panic(err)
	}

	// Create answer
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	// Sets the LocalDescription, and starts our UDP listeners
	err = peerConnection.SetLocalDescription(answer)
	if err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	// Get the LocalDescription and take it to base64 so we can paste in browser
	fmt.Println(encode(peerConnection.LocalDescription()))

	localTrack := <-localTrackChan
	for {
		fmt.Println("")
		fmt.Println("Curl an base64 SDP to start sendonly peer connection")

		recvOnlyOffer := webrtc.SessionDescription{}
		decode(<-sdpChan, &recvOnlyOffer)

		// Create a new PeerConnection
		peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)
		if err != nil {
			panic(err)
		}

		rtpSender, err := peerConnection.AddTrack(localTrack)
		if err != nil {
			panic(err)
		}

		// Read incoming RTCP packets
		// Before these packets are returned they are processed by interceptors. For things
		// like NACK this needs to be called.
		go func() {
			rtcpBuf := make([]byte, 1500)
			for {
				if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
					return
				}
			}
		}()

		// Set the remote SessionDescription
		err = peerConnection.SetRemoteDescription(recvOnlyOffer)
		if err != nil {
			panic(err)
		}

		// Create answer
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			panic(err)
		}

		// Create channel that is blocked until ICE Gathering is complete
		gatherComplete = webrtc.GatheringCompletePromise(peerConnection)

		// Sets the LocalDescription, and starts our UDP listeners
		err = peerConnection.SetLocalDescription(answer)
		if err != nil {
			panic(err)
		}

		// Block until ICE Gathering is complete, disabling trickle ICE
		// we do this because we only can exchange one signaling message
		// in a production application you should exchange ICE Candidates via OnICECandidate
		<-gatherComplete

		// Get the LocalDescription and take it to base64 so we can paste in browser
		fmt.Println(encode(peerConnection.LocalDescription()))
	}
}

// JSON encode + base64 a SessionDescription.
func encode(obj *webrtc.SessionDescription) string {
	b, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}

	return base64.StdEncoding.EncodeToString(b)
}

// Decode a base64 and unmarshal JSON into a SessionDescription.
func decode(in string, obj *webrtc.SessionDescription) {
    fmt.Printf("【解码开始】输入字符串长度: %d 字符\n", len(in))
    fmt.Printf("  输入预览: %.40s...\n", in) // 显示前40个字符

    // Base64 解码
    b, err := base64.StdEncoding.DecodeString(in)
    if err != nil {
        fmt.Printf("❌ Base64解码失败! 错误类型: %T\n", err)
        fmt.Printf("   错误详情: %v\n", err)
        fmt.Printf("   输入片段: %.20q...\n", in) // 显示有问题的部分
        panic(err)
    }
    fmt.Printf("✅ Base64解码成功 解码后字节数: %d\n", len(b))
    fmt.Printf("   16进制预览: %x...\n", b[:min(8, len(b))]) // 显示前8字节

    // JSON 反序列化
    fmt.Println("🔄 开始JSON反序列化...")
    if err = json.Unmarshal(b, obj); err != nil {
        fmt.Printf("❌ JSON解析失败! 错误类型: %T\n", err)
        fmt.Printf("   错误详情: %v\n", err)
        fmt.Printf("   JSON片段: %.100s...\n", string(b)) // 显示可能有问题的部分
        panic(err)
    }
    fmt.Printf("✅ JSON解析成功 对象类型: %T\n", obj)
    fmt.Printf("✅ JSON解析成功 对象类型: %+v\n", obj)
}


// httpSDPServer starts a HTTP Server that consumes SDPs.
func httpSDPServer(port int) chan string {
	sdpChan := make(chan string)
	http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		fmt.Fprintf(res, "done") //nolint: errcheck
		sdpChan <- string(body)
	})

	go func() {
		// nolint: gosec
		panic(http.ListenAndServe(":"+strconv.Itoa(port), nil))
	}()

	return sdpChan
}
