// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

// play-from-disk-renegotiation demonstrates Pion WebRTC's renegotiation abilities.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pion/randutil"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/ivfreader"
)

var peerConnection *webrtc.PeerConnection //nolint

// doSignaling exchanges all state of the local PeerConnection and is called
// every time a video is added or removed.
func doSignaling(res http.ResponseWriter, req *http.Request) {
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(req.Body).Decode(&offer); err != nil {
		panic(err)
	}

	if err := peerConnection.SetRemoteDescription(offer); err != nil {
		panic(err)
	}

	// Create channel that is blocked until ICE Gathering is complete
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)

	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		panic(err)
	} else if err = peerConnection.SetLocalDescription(answer); err != nil {
		panic(err)
	}

	// Block until ICE Gathering is complete, disabling trickle ICE
	// we do this because we only can exchange one signaling message
	// in a production application you should exchange ICE Candidates via OnICECandidate
	<-gatherComplete

	response, err := json.Marshal(*peerConnection.LocalDescription())
	if err != nil {
		panic(err)
	}

	res.Header().Set("Content-Type", "application/json")
	if _, err := res.Write(response); err != nil {
		panic(err)
	}
}

// Add a single video track.
func createPeerConnection(res http.ResponseWriter, req *http.Request) {
	if peerConnection.ConnectionState() != webrtc.PeerConnectionStateNew {
		panic(fmt.Sprintf("createPeerConnection called in non-new state (%s)", peerConnection.ConnectionState()))
	}

	doSignaling(res, req)
	fmt.Println("PeerConnection has been created")
}

// Add a single video track.
func addVideo(res http.ResponseWriter, req *http.Request) { //nolint:cyclop
	// Open a IVF file and start reading using our IVFReader
	file, err := os.Open("output.ivf")
	if err != nil {
		panic(err)
	}

	ivf, header, err := ivfreader.NewWith(file)
	if err != nil {
		panic(err)
	}

	var mimeType string
	switch header.FourCC {
	case "VP80":
		mimeType = webrtc.MimeTypeVP8
	case "VP90":
		mimeType = webrtc.MimeTypeVP9
	case "AV01":
		mimeType = webrtc.MimeTypeAV1
	default:
		panic(fmt.Sprintf("unsupported codec: %s", header.FourCC))
	}

	videoTrack, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: mimeType},
		fmt.Sprintf("video-%d", randutil.NewMathRandomGenerator().Uint32()),
		fmt.Sprintf("video-%d", randutil.NewMathRandomGenerator().Uint32()),
	)
	if err != nil {
		panic(err)
	}
	rtpSender, err := peerConnection.AddTrack(videoTrack)
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

	doSignaling(res, req)
	fmt.Println("Video track has been added")
	go writeVideoToTrack(ivf, header, videoTrack)
}

// Remove a single sender.
func removeVideo(res http.ResponseWriter, req *http.Request) {
	if senders := peerConnection.GetSenders(); len(senders) != 0 {
		if err := peerConnection.RemoveTrack(senders[0]); err != nil {
			panic(err)
		}
	}

	doSignaling(res, req)
	fmt.Println("Video track has been removed")
}

func main() {
	var err error
	if peerConnection, err = webrtc.NewPeerConnection(webrtc.Configuration{}); err != nil {
		panic(err)
	}
	defer func() {
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("cannot close peerConnection: %v\n", cErr)
		}
	}()

	// Set the handler for Peer connection state
	// This will notify you when the peer has connected/disconnected
	peerConnection.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		fmt.Printf("Peer Connection State has changed: %s\n", state.String())

		if state == webrtc.PeerConnectionStateFailed {
			// Wait until PeerConnection has had no network activity for 30 seconds or another failure.
			// It may be reconnected using an ICE Restart.
			// Use webrtc.PeerConnectionStateDisconnected if you are interested in detecting faster timeout.
			// Note that the PeerConnection may come back from PeerConnectionStateDisconnected.
			fmt.Println("Peer Connection has gone to failed exiting")
			os.Exit(0)
		}

		if state == webrtc.PeerConnectionStateClosed {
			// PeerConnection was explicitly closed. This usually happens from a DTLS CloseNotify
			fmt.Println("Peer Connection has gone to closed exiting")
			os.Exit(0)
		}
	})

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/createPeerConnection", createPeerConnection)
	http.HandleFunc("/addVideo", addVideo)
	http.HandleFunc("/removeVideo", removeVideo)

	go func() {
		fmt.Println("Open http://localhost:8080 to access this demo")
		// nolint: gosec
		panic(http.ListenAndServe(":8080", nil))
	}()

	// Block forever
	select {}
}

// Read a video file from disk and write it to a webrtc.Track
// When the video has been completely read this exits without error.
func writeVideoToTrack(
	ivf *ivfreader.IVFReader, header *ivfreader.IVFFileHeader, track *webrtc.TrackLocalStaticSample,
) {
	// Send our video file frame at a time. Pace our sending so we send it at the same speed it should be played back as.
	// This isn't required since the video is timestamped, but we will such much higher loss if we send all at once.
	//
	// It is important to use a time.Ticker instead of time.Sleep because
	// * avoids accumulating skew, just calling time.Sleep didn't compensate for the time spent parsing the data
	// * works around latency issues with Sleep (see https://github.com/golang/go/issues/44343)
	ticker := time.NewTicker(
		time.Millisecond * time.Duration((float32(header.TimebaseNumerator)/float32(header.TimebaseDenominator))*1000),
	)
	defer ticker.Stop()
	for ; true; <-ticker.C {
		frame, _, err := ivf.ParseNextFrame()
		if err != nil {
			fmt.Printf("Finish writing video track: %s ", err)

			return
		}

		if err = track.WriteSample(media.Sample{Data: frame, Duration: time.Second}); err != nil {
			fmt.Printf("Finish writing video track: %s ", err)

			return
		}
	}
}
