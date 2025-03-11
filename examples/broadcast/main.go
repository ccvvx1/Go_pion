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
	// 命令行参数解析
	port := flag.Int("port", 1090, "HTTP服务端口号")
	flag.Parse()

	// 启动HTTP服务器并创建SDP通道
	fmt.Printf("【初始化】启动HTTP服务器，监听端口：%d\n", *port)
	sdpChan := httpSDPServer(*port)

	// WebRTC配置初始化
	fmt.Println("【WebRTC】初始化配置...")
	offer := webrtc.SessionDescription{}
	fmt.Printf("【WebRTC】初始Offer结构体：%#v\n", offer)

	// 接收SDP Offer
	fmt.Println("【网络】等待接收SDP Offer...")
	tmpSdp := "eyJ0eXBlIjoib2ZmZXIiLCJzZHAiOiJ2PTBcclxubz0tIDEzMzY4MzcwMjkzOTM5MTI3NjAgMiBJTiBJUDQgMTI3LjAuMC4xXHJcbnM9LVxyXG50PTAgMFxyXG5hPWdyb3VwOkJVTkRMRSAwXHJcbmE9ZXh0bWFwLWFsbG93LW1peGVkXHJcbmE9bXNpZC1zZW1hbnRpYzogV01TIGMyMjdiYmFlLTMzMDgtNGViYi1iZjIzLTFiMzBkN2UxMGI2MFxyXG5tPXZpZGVvIDIyNjIgVURQL1RMUy9SVFAvU0FWUEYgOTYgOTcgMTAzIDEwNCAxMDcgMTA4IDEwOSAxMTQgMTE1IDExNiAxMTcgMTE4IDM5IDQwIDQ1IDQ2IDk4IDk5IDEwMCAxMDEgMTIzIDEyNCAxMjVcclxuYz1JTiBJUDQgMjIxLjE3Ni4zMy45NFxyXG5hPXJ0Y3A6OSBJTiBJUDQgMC4wLjAuMFxyXG5hPWNhbmRpZGF0ZToyNzAwNzY2NzAxIDEgdWRwIDIxMTM5MzcxNTEgODk1ODg3NzktZDYwNi00MTZhLTg0ZDUtNDhkODVmZTQwMjA4LmxvY2FsIDY1MTQ3IHR5cCBob3N0IGdlbmVyYXRpb24gMCBuZXR3b3JrLWNvc3QgOTk5XHJcbmE9Y2FuZGlkYXRlOjI5NTA3MTQ5MjUgMSB1ZHAgMTY3NzcyOTUzNSAyMjEuMTc2LjMzLjk0IDIyNjIgdHlwIHNyZmx4IHJhZGRyIDAuMC4wLjAgcnBvcnQgMCBnZW5lcmF0aW9uIDAgbmV0d29yay1jb3N0IDk5OVxyXG5hPWljZS11ZnJhZzp3YmZOXHJcbmE9aWNlLXB3ZDpuNEx4TkZ0eXVpdmRoU1kzTUVxMmdTTHhcclxuYT1pY2Utb3B0aW9uczp0cmlja2xlXHJcbmE9ZmluZ2VycHJpbnQ6c2hhLTI1NiA2ODpBRDpFRjpDODo4RTo5Rjo3MzpCMzpDQToyNDpFNzpEQjpGQzo0RTpCNDo4Njo3MjoxRDozMzo2RTo2MjowODo1RDo4OTo2NzpENDpEQzpENjpFNzozMDpFMzpGNFxyXG5hPXNldHVwOmFjdHBhc3NcclxuYT1taWQ6MFxyXG5hPWV4dG1hcDoxIHVybjppZXRmOnBhcmFtczpydHAtaGRyZXh0OnRvZmZzZXRcclxuYT1leHRtYXA6MiBodHRwOi8vd3d3LndlYnJ0Yy5vcmcvZXhwZXJpbWVudHMvcnRwLWhkcmV4dC9hYnMtc2VuZC10aW1lXHJcbmE9ZXh0bWFwOjMgdXJuOjNncHA6dmlkZW8tb3JpZW50YXRpb25cclxuYT1leHRtYXA6NCBodHRwOi8vd3d3LmlldGYub3JnL2lkL2RyYWZ0LWhvbG1lci1ybWNhdC10cmFuc3BvcnQtd2lkZS1jYy1leHRlbnNpb25zLTAxXHJcbmE9ZXh0bWFwOjUgaHR0cDovL3d3dy53ZWJydGMub3JnL2V4cGVyaW1lbnRzL3J0cC1oZHJleHQvcGxheW91dC1kZWxheVxyXG5hPWV4dG1hcDo2IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L3ZpZGVvLWNvbnRlbnQtdHlwZVxyXG5hPWV4dG1hcDo3IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L3ZpZGVvLXRpbWluZ1xyXG5hPWV4dG1hcDo4IGh0dHA6Ly93d3cud2VicnRjLm9yZy9leHBlcmltZW50cy9ydHAtaGRyZXh0L2NvbG9yLXNwYWNlXHJcbmE9ZXh0bWFwOjkgdXJuOmlldGY6cGFyYW1zOnJ0cC1oZHJleHQ6c2RlczptaWRcclxuYT1leHRtYXA6MTAgdXJuOmlldGY6cGFyYW1zOnJ0cC1oZHJleHQ6c2RlczpydHAtc3RyZWFtLWlkXHJcbmE9ZXh0bWFwOjExIHVybjppZXRmOnBhcmFtczpydHAtaGRyZXh0OnNkZXM6cmVwYWlyZWQtcnRwLXN0cmVhbS1pZFxyXG5hPXNlbmRyZWN2XHJcbmE9bXNpZDpjMjI3YmJhZS0zMzA4LTRlYmItYmYyMy0xYjMwZDdlMTBiNjAgNTA5OTlmYmUtYWM3My00ZmI3LWJjYTUtM2UwMWJhYWNlOWUwXHJcbmE9cnRjcC1tdXhcclxuYT1ydGNwLXJzaXplXHJcbmE9cnRwbWFwOjk2IFZQOC85MDAwMFxyXG5hPXJ0Y3AtZmI6OTYgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjo5NiB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjk2IGNjbSBmaXJcclxuYT1ydGNwLWZiOjk2IG5hY2tcclxuYT1ydGNwLWZiOjk2IG5hY2sgcGxpXHJcbmE9cnRwbWFwOjk3IHJ0eC85MDAwMFxyXG5hPWZtdHA6OTcgYXB0PTk2XHJcbmE9cnRwbWFwOjEwMyBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjoxMDMgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMDMgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMDMgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTAzIG5hY2tcclxuYT1ydGNwLWZiOjEwMyBuYWNrIHBsaVxyXG5hPWZtdHA6MTAzIGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTE7cHJvZmlsZS1sZXZlbC1pZD00MjAwMWZcclxuYT1ydHBtYXA6MTA0IHJ0eC85MDAwMFxyXG5hPWZtdHA6MTA0IGFwdD0xMDNcclxuYT1ydHBtYXA6MTA3IEgyNjQvOTAwMDBcclxuYT1ydGNwLWZiOjEwNyBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjEwNyB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjEwNyBjY20gZmlyXHJcbmE9cnRjcC1mYjoxMDcgbmFja1xyXG5hPXJ0Y3AtZmI6MTA3IG5hY2sgcGxpXHJcbmE9Zm10cDoxMDcgbGV2ZWwtYXN5bW1ldHJ5LWFsbG93ZWQ9MTtwYWNrZXRpemF0aW9uLW1vZGU9MDtwcm9maWxlLWxldmVsLWlkPTQyMDAxZlxyXG5hPXJ0cG1hcDoxMDggcnR4LzkwMDAwXHJcbmE9Zm10cDoxMDggYXB0PTEwN1xyXG5hPXJ0cG1hcDoxMDkgSDI2NC85MDAwMFxyXG5hPXJ0Y3AtZmI6MTA5IGdvb2ctcmVtYlxyXG5hPXJ0Y3AtZmI6MTA5IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6MTA5IGNjbSBmaXJcclxuYT1ydGNwLWZiOjEwOSBuYWNrXHJcbmE9cnRjcC1mYjoxMDkgbmFjayBwbGlcclxuYT1mbXRwOjEwOSBsZXZlbC1hc3ltbWV0cnktYWxsb3dlZD0xO3BhY2tldGl6YXRpb24tbW9kZT0xO3Byb2ZpbGUtbGV2ZWwtaWQ9NDJlMDFmXHJcbmE9cnRwbWFwOjExNCBydHgvOTAwMDBcclxuYT1mbXRwOjExNCBhcHQ9MTA5XHJcbmE9cnRwbWFwOjExNSBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjoxMTUgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMTUgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMTUgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTE1IG5hY2tcclxuYT1ydGNwLWZiOjExNSBuYWNrIHBsaVxyXG5hPWZtdHA6MTE1IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTA7cHJvZmlsZS1sZXZlbC1pZD00MmUwMWZcclxuYT1ydHBtYXA6MTE2IHJ0eC85MDAwMFxyXG5hPWZtdHA6MTE2IGFwdD0xMTVcclxuYT1ydHBtYXA6MTE3IEgyNjQvOTAwMDBcclxuYT1ydGNwLWZiOjExNyBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjExNyB0cmFuc3BvcnQtY2NcclxuYT1ydGNwLWZiOjExNyBjY20gZmlyXHJcbmE9cnRjcC1mYjoxMTcgbmFja1xyXG5hPXJ0Y3AtZmI6MTE3IG5hY2sgcGxpXHJcbmE9Zm10cDoxMTcgbGV2ZWwtYXN5bW1ldHJ5LWFsbG93ZWQ9MTtwYWNrZXRpemF0aW9uLW1vZGU9MTtwcm9maWxlLWxldmVsLWlkPTRkMDAxZlxyXG5hPXJ0cG1hcDoxMTggcnR4LzkwMDAwXHJcbmE9Zm10cDoxMTggYXB0PTExN1xyXG5hPXJ0cG1hcDozOSBIMjY0LzkwMDAwXHJcbmE9cnRjcC1mYjozOSBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjM5IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6MzkgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MzkgbmFja1xyXG5hPXJ0Y3AtZmI6MzkgbmFjayBwbGlcclxuYT1mbXRwOjM5IGxldmVsLWFzeW1tZXRyeS1hbGxvd2VkPTE7cGFja2V0aXphdGlvbi1tb2RlPTA7cHJvZmlsZS1sZXZlbC1pZD00ZDAwMWZcclxuYT1ydHBtYXA6NDAgcnR4LzkwMDAwXHJcbmE9Zm10cDo0MCBhcHQ9MzlcclxuYT1ydHBtYXA6NDUgQVYxLzkwMDAwXHJcbmE9cnRjcC1mYjo0NSBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjQ1IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6NDUgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6NDUgbmFja1xyXG5hPXJ0Y3AtZmI6NDUgbmFjayBwbGlcclxuYT1mbXRwOjQ1IGxldmVsLWlkeD01O3Byb2ZpbGU9MDt0aWVyPTBcclxuYT1ydHBtYXA6NDYgcnR4LzkwMDAwXHJcbmE9Zm10cDo0NiBhcHQ9NDVcclxuYT1ydHBtYXA6OTggVlA5LzkwMDAwXHJcbmE9cnRjcC1mYjo5OCBnb29nLXJlbWJcclxuYT1ydGNwLWZiOjk4IHRyYW5zcG9ydC1jY1xyXG5hPXJ0Y3AtZmI6OTggY2NtIGZpclxyXG5hPXJ0Y3AtZmI6OTggbmFja1xyXG5hPXJ0Y3AtZmI6OTggbmFjayBwbGlcclxuYT1mbXRwOjk4IHByb2ZpbGUtaWQ9MFxyXG5hPXJ0cG1hcDo5OSBydHgvOTAwMDBcclxuYT1mbXRwOjk5IGFwdD05OFxyXG5hPXJ0cG1hcDoxMDAgVlA5LzkwMDAwXHJcbmE9cnRjcC1mYjoxMDAgZ29vZy1yZW1iXHJcbmE9cnRjcC1mYjoxMDAgdHJhbnNwb3J0LWNjXHJcbmE9cnRjcC1mYjoxMDAgY2NtIGZpclxyXG5hPXJ0Y3AtZmI6MTAwIG5hY2tcclxuYT1ydGNwLWZiOjEwMCBuYWNrIHBsaVxyXG5hPWZtdHA6MTAwIHByb2ZpbGUtaWQ9MlxyXG5hPXJ0cG1hcDoxMDEgcnR4LzkwMDAwXHJcbmE9Zm10cDoxMDEgYXB0PTEwMFxyXG5hPXJ0cG1hcDoxMjMgcmVkLzkwMDAwXHJcbmE9cnRwbWFwOjEyNCBydHgvOTAwMDBcclxuYT1mbXRwOjEyNCBhcHQ9MTIzXHJcbmE9cnRwbWFwOjEyNSB1bHBmZWMvOTAwMDBcclxuYT1zc3JjLWdyb3VwOkZJRCAxOTk2NDg1MTYwIDU2NTA0ODYxMFxyXG5hPXNzcmM6MTk5NjQ4NTE2MCBjbmFtZTpLVXBhL3dCckw0MU5Qa3VGXHJcbmE9c3NyYzoxOTk2NDg1MTYwIG1zaWQ6YzIyN2JiYWUtMzMwOC00ZWJiLWJmMjMtMWIzMGQ3ZTEwYjYwIDUwOTk5ZmJlLWFjNzMtNGZiNy1iY2E1LTNlMDFiYWFjZTllMFxyXG5hPXNzcmM6NTY1MDQ4NjEwIGNuYW1lOktVcGEvd0JyTDQxTlBrdUZcclxuYT1zc3JjOjU2NTA0ODYxMCBtc2lkOmMyMjdiYmFlLTMzMDgtNGViYi1iZjIzLTFiMzBkN2UxMGI2MCA1MDk5OWZiZS1hYzczLTRmYjctYmNhNS0zZTAxYmFhY2U5ZTBcclxuIn0="

	decode(tmpSdp, &offer)
	fmt.Printf("【网络】成功解码SDP Offer，类型：%s\n", offer.Type)

	// ICE服务器配置
	peerConnectionConfig := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	fmt.Println("【ICE】配置STUN服务器：stun.l.google.com:19302")

	// 媒体引擎初始化
	mediaEngine := &webrtc.MediaEngine{}
	fmt.Println("【媒体】注册默认编解码器...")
	if err := mediaEngine.RegisterDefaultCodecs(); err != nil {
		fmt.Printf("【错误】编解码器注册失败：%v\n", err)
		panic(err)
	}

	// 拦截器配置
	interceptorRegistry := &interceptor.Registry{}
	fmt.Println("【拦截器】注册默认拦截器...")
	if err := webrtc.RegisterDefaultInterceptors(mediaEngine, interceptorRegistry); err != nil {
		fmt.Printf("【错误】拦截器注册失败：%v\n", err)
		panic(err)
	}

	// 配置PLI拦截器
	fmt.Println("【视频】配置PLI间隔拦截器...")
	intervalPliFactory, err := intervalpli.NewReceiverInterceptor()
	if err != nil {
		fmt.Printf("【错误】PLI拦截器创建失败：%v\n", err)
		panic(err)
	}
	interceptorRegistry.Add(intervalPliFactory)

	// 创建对等连接
	fmt.Printf("【WebRTC】正在创建对等连接，使用配置：%+v\n", peerConnectionConfig)
	peerConnection, err := webrtc.NewAPI(
		webrtc.WithMediaEngine(mediaEngine),
		webrtc.WithInterceptorRegistry(interceptorRegistry),
	).NewPeerConnection(peerConnectionConfig)
	if err != nil {
		fmt.Printf("【错误】创建对等连接失败：%v\n", err)
		panic(err)
	}
	fmt.Println("【WebRTC】对等连接创建成功")

	// 连接关闭处理
	defer func() {
		fmt.Println("【资源】开始清理对等连接资源...")
		if cErr := peerConnection.Close(); cErr != nil {
			fmt.Printf("【警告】关闭对等连接时发生错误：%v\n", cErr)
		} else {
			fmt.Println("【资源】对等连接已安全关闭")
		}
	}()

	// 配置视频接收器
	fmt.Println("【媒体】添加视频收发器...")
	transceiver, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	if err != nil {
		fmt.Printf("【错误】创建视频收发器失败：%v\n", err)
		panic(err)
	}
	fmt.Printf("【媒体】收发器创建成功，类型：%s 方向：%s\n", 
		transceiver.Kind(), transceiver.Direction())

	// 本地轨道通道初始化
	localTrackChan := make(chan *webrtc.TrackLocalStaticRTP, 1)
	fmt.Println("【通道】创建本地轨道缓冲通道（容量1）")

	// 轨道处理回调
	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		fmt.Printf("【媒体】收到远程轨道：\n"+
			"  SSRC: %d\n"+
			"  PayloadType: %d\n"+
			"  Codec: %s/%d\n",
			remoteTrack.SSRC(),
			remoteTrack.PayloadType(),
			remoteTrack.Codec().MimeType,
			remoteTrack.Codec().ClockRate)

		// 创建本地轨道
		localTrack, err := webrtc.NewTrackLocalStaticRTP(
			remoteTrack.Codec().RTPCodecCapability, 
			"video", 
			"pion",
		)
		if err != nil {
			fmt.Printf("【错误】创建本地轨道失败：%v\n", err)
			panic(err)
		}
		fmt.Printf("【媒体】本地轨道创建成功：%s\n", localTrack.ID())

		// 发送轨道到处理通道
		select {
		case localTrackChan <- localTrack:
			fmt.Println("【通道】成功发送本地轨道到处理通道")
		default:
			fmt.Println("【警告】本地轨道通道阻塞，丢弃轨道")
		}

		// 数据转发循环
		rtpBuf := make([]byte, 1400)
		fmt.Println("【传输】启动RTP数据转发循环...")
		for {
			// 读取远程数据
			n, _, err := remoteTrack.Read(rtpBuf)
			if err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println("【传输】远程轨道已关闭")
					return
				}
				fmt.Printf("【错误】读取远程数据失败：%v\n", err)
				panic(err)
			}

			// 写入本地轨道
			if _, err = localTrack.Write(rtpBuf[:n]); err != nil {
				if !errors.Is(err, io.ErrClosedPipe) {
					fmt.Printf("【错误】写入本地轨道失败：%v\n", err)
					panic(err)
				}
				fmt.Println("【传输】无订阅者，丢弃数据包")
			} else {
				fmt.Printf("【传输】成功转发 %d 字节数据\n", n)
			}
		}
	})


	// 设置远端SDP描述
	fmt.Printf("【SDP】开始设置远端Offer，类型：%s\n", offer.Type)
	// fmt.Printf("【SDP】Offer摘要：\n%s\n", sdpSummary(offer.SDP)) // 自定义函数隐藏敏感信息
	if err = peerConnection.SetRemoteDescription(offer); err != nil {
		fmt.Printf("【错误】设置远端SDP失败：%v\n", err)
		panic(err)
	}
	fmt.Println("【SDP】远端Offer设置成功")

	// 创建应答SDP
	fmt.Println("【协商】开始创建应答Answer...")
	answer, err := peerConnection.CreateAnswer(nil)
	if err != nil {
		fmt.Printf("【错误】创建应答失败：%v\n", err)
		panic(err)
	}
	fmt.Printf("【协商】应答创建成功，初始状态：%s\n", answer.SDP[:50])

	// 准备ICE收集完成同步
	fmt.Println("【ICE】等待候选收集完成...")
	gatherComplete := webrtc.GatheringCompletePromise(peerConnection)
	// gatherTimeout := time.After(15 * time.Second) // 增加超时机制

	// 设置本地SDP描述
	fmt.Println("【SDP】设置本地Answer描述...")
	if err = peerConnection.SetLocalDescription(answer); err != nil {
		fmt.Printf("【错误】设置本地SDP失败：%v\n", err)
		panic(err)
	}

	<-gatherComplete

	// 输出最终SDP
	finalSDP := peerConnection.LocalDescription()
	fmt.Printf("【SDP】最终Answer类型：%s，长度：%d字节\n", finalSDP.Type, len(finalSDP.SDP))
	fmt.Println("【SDP】Base64编码结果：")
	fmt.Println(encode(finalSDP))

	// 接收本地视频轨道
	fmt.Println("【媒体】等待本地视频轨道...")
	// start := time.Now()
	localTrack := <-localTrackChan
	// fmt.Printf("【媒体】收到本地轨道 (等待耗时：%v)：\n", time.Since(start))
	fmt.Printf("  轨道ID：%s\n", localTrack.ID())
	fmt.Printf("  编解码能力：%s/%d\n", 
		localTrack.Codec().MimeType, 
		localTrack.Codec().ClockRate)



	for {// 接收第二个SDP Offer
		fmt.Println("\n【第二阶段】等待接收方SDP Offer...")
		recvOnlyOffer := webrtc.SessionDescription{}
		decode(<-sdpChan, &recvOnlyOffer)
		fmt.Printf("【SDP】收到接收方Offer类型：%s\n", recvOnlyOffer.Type)
		// fmt.Printf("【SDP】Offer摘要：\n%s\n", sdpSummary(recvOnlyOffer.SDP))
		
		// 创建新的对等连接
		fmt.Println("【WebRTC】创建接收方PeerConnection...")
		peerConnection, err := webrtc.NewPeerConnection(peerConnectionConfig)
		if err != nil {
			fmt.Printf("【错误】创建接收方PC失败：%v\n", err)
			panic(err)
		}
		defer func() {
			fmt.Println("【资源】开始清理接收方PC资源...")
			if cErr := peerConnection.Close(); cErr != nil {
				fmt.Printf("【警告】关闭接收方PC失败：%v\n", cErr)
			}
		}()
		
		// 添加本地视频轨道
		fmt.Printf("【媒体】添加本地轨道到发送器 (轨道ID:%s)...\n", localTrack.ID())
		rtpSender, err := peerConnection.AddTrack(localTrack)
		if err != nil {
			fmt.Printf("【错误】添加轨道失败：%v\n", err)
			panic(err)
		}
		// fmt.Printf("【媒体】创建发送器成功 (SSRC:%d)\n", rtpSender.Track().SSRC())
		
		// RTCP监听协程
		fmt.Println("【控制】启动RTCP监听协程...")
		go func() {
			rtcpBuf := make([]byte, 1500)
			rtcpCount := 0
			for {
				n, _, rtcpErr := rtpSender.Read(rtcpBuf)
				if rtcpErr != nil {
					if rtcpErr == io.EOF {
						fmt.Println("【控制】RTCP连接正常关闭")
					} else {
						fmt.Printf("【错误】RTCP读取失败：%v\n", rtcpErr)
					}
					return
				}
				rtcpCount++
				fmt.Printf("【控制】收到第%d个RTCP包 (大小:%d字节)\n", rtcpCount, n)
				// 解析RTCP包示例：
				// if packet, _, _ := rtcp.Unmarshal(rtcpBuf[:n]); packet != nil {
				//     fmt.Printf("RTCP类型：%T\n", packet)
				// }
			}
		}()
		
		// 设置远端描述
		fmt.Println("【协商】设置接收方远端描述...")
		if err = peerConnection.SetRemoteDescription(recvOnlyOffer); err != nil {
			fmt.Printf("【错误】设置接收方远端描述失败：%v\n", err)
			panic(err)
		}
		
		// 创建应答Answer
		fmt.Println("【协商】创建接收方Answer...")
		answer, err := peerConnection.CreateAnswer(nil)
		if err != nil {
			fmt.Printf("【错误】创建接收方Answer失败：%v\n", err)
			panic(err)
		}
		// fmt.Printf("【SDP】Answer初始内容：\n%s\n", sdpSummary(answer.SDP))
		
		// ICE收集同步
		fmt.Println("【ICE】等待接收方候选收集...")
		gatherComplete = webrtc.GatheringCompletePromise(peerConnection)
		// gatherTimeout := time.After(15 * time.Second)
		
		// 设置本地描述
		fmt.Println("【协商】设置接收方本地描述...")
		if err = peerConnection.SetLocalDescription(answer); err != nil {
			fmt.Printf("【错误】设置接收方本地描述失败：%v\n", err)
			panic(err)
		}
		
		// 等待ICE完成
		<-gatherComplete
		
		// 输出最终SDP
		finalAnswer := peerConnection.LocalDescription()
		fmt.Printf("\n【SDP】最终Answer信息：\n"+
			"类型：%s\n"+
			"长度：%d字节\n"+
			"Base64编码结果：\n%s\n",
			finalAnswer.Type,
			len(finalAnswer.SDP),
			encode(finalAnswer))
		
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
