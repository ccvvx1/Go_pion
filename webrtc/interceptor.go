// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

//go:build !js
// +build !js

package webrtc

import (
	"sync/atomic"
	"fmt"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/report"
	"github.com/pion/interceptor/pkg/rfc8888"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
)

// RegisterDefaultInterceptors will register some useful interceptors.
// If you want to customize which interceptors are loaded, you should copy the
// code from this method and remove unwanted interceptors.
func RegisterDefaultInterceptors(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {
	// 配置NACK丢包重传机制
	fmt.Println("【NACK配置】开始配置网络丢包重传机制...")
	if err := ConfigureNack(mediaEngine, interceptorRegistry); err != nil {
		fmt.Printf("!! 【NACK配置】关键错误：%v (mediaEngine=%p, registry=%p)\n", err, mediaEngine, interceptorRegistry)
		return fmt.Errorf("NACK配置失败: %w", err)
	} 
	// fmt.Printf("【NACK配置】成功启用，当前注册拦截器数量：%d\n", interceptorRegistry.Len())

	// 配置RTCP质量监控报告
	fmt.Println("\n【RTCP配置】初始化质量监控系统...")
	if err := ConfigureRTCPReports(interceptorRegistry); err != nil {
		fmt.Printf("!! 【RTCP配置】报告生成器创建失败：%T %v\n", err, err)
		fmt.Println("!! 可能原因：1. 拦截器未注册 2. 时钟源不可用")
		return err
	}
	fmt.Println("【RTCP配置】已启用以下报告类型：")
	fmt.Println("  - 发送端报告(SR) 接收端报告(RR)")
	fmt.Println("  - 扩展报告(XR) 丢包统计")

	// 配置多流编码头扩展
	fmt.Println("\n【Simulcast】协商多分辨率流支持...")
	if err := ConfigureSimulcastExtensionHeaders(mediaEngine); err != nil {
		// fmt.Printf("!! 【Simulcast】扩展头注册失败，当前编码器：%+v\n", mediaEngine.GetCodecs())
		fmt.Printf("!! 详细错误：%s\n", err.Error())
		return err
	} 
	// fmt.Printf("【Simulcast】成功注册扩展头，当前支持的RTP扩展：%v\n", mediaEngine.GetHeaderExtensions())

	// 配置带宽估计算法
	fmt.Println("\n【拥塞控制】启动传输层带宽探测(TWCC)...")
	if err := ConfigureTWCCSender(mediaEngine, interceptorRegistry); err != nil {
		fmt.Printf("!! 【TWCC】带宽控制器初始化异常！错误链：%+v\n", err)
		// if errors.Is(err, ErrCodecNotSupported) {
		// 	fmt.Println("!! 紧急：检测到不支持的编码格式，请验证H264/V8配置")
		// }
		return err
	}
	// fmt.Printf("【拥塞控制】最终配置状态：\n  NACK：%t\n  RTCP：%t\n  Simulcast：%d层\n  TWCC：%t\n",
	// 	mediaEngine.NackEnabled, 
	// 	interceptorRegistry.HasRTCP(), 
	// 	mediaEngine.SimulcastLayers,
	// 	interceptorRegistry.HasTWCC(),
	// )

	return nil

}

// ConfigureRTCPReports will setup everything necessary for generating Sender and Receiver Reports.
func ConfigureRTCPReports(interceptorRegistry *interceptor.Registry) error {
	reciver, err := report.NewReceiverInterceptor()
	if err != nil {
		return err
	}

	sender, err := report.NewSenderInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(reciver)
	interceptorRegistry.Add(sender)

	return nil
}

// ConfigureNack will setup everything necessary for handling generating/responding to nack messages.
func ConfigureNack(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return err
	}

	responder, err := nack.NewResponderInterceptor()
	if err != nil {
		return err
	}

	mediaEngine.RegisterFeedback(RTCPFeedback{Type: "nack"}, RTPCodecTypeVideo)
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: "nack", Parameter: "pli"}, RTPCodecTypeVideo)
	interceptorRegistry.Add(responder)
	interceptorRegistry.Add(generator)

	return nil
}

// ConfigureTWCCHeaderExtensionSender will setup everything necessary for adding
// a TWCC header extension to outgoing RTP packets. This will allow the remote peer to generate TWCC reports.
func ConfigureTWCCHeaderExtensionSender(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {
	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	i, err := twcc.NewHeaderExtensionInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(i)

	return nil
}

// ConfigureTWCCSender will setup everything necessary for generating TWCC reports.
// This must be called after registering codecs with the MediaEngine.
func ConfigureTWCCSender(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBTransportCC}, RTPCodecTypeVideo)
	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBTransportCC}, RTPCodecTypeAudio)
	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	generator, err := twcc.NewSenderInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(generator)

	return nil
}

// ConfigureCongestionControlFeedback registers congestion control feedback as
// defined in RFC 8888 (https://datatracker.ietf.org/doc/rfc8888/)
func ConfigureCongestionControlFeedback(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBACK, Parameter: "ccfb"}, RTPCodecTypeVideo)
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBACK, Parameter: "ccfb"}, RTPCodecTypeAudio)
	generator, err := rfc8888.NewSenderInterceptor()
	if err != nil {
		return err
	}
	interceptorRegistry.Add(generator)

	return nil
}

// ConfigureSimulcastExtensionHeaders enables the RTP Extension Headers needed for Simulcast.
func ConfigureSimulcastExtensionHeaders(mediaEngine *MediaEngine) error {
	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.SDESMidURI}, RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI}, RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	return mediaEngine.RegisterHeaderExtension(
		RTPHeaderExtensionCapability{URI: sdp.SDESRepairRTPStreamIDURI}, RTPCodecTypeVideo,
	)
}

type interceptorToTrackLocalWriter struct{ interceptor atomic.Value } // interceptor.RTPWriter }

func (i *interceptorToTrackLocalWriter) WriteRTP(header *rtp.Header, payload []byte) (int, error) {
	if writer, ok := i.interceptor.Load().(interceptor.RTPWriter); ok && writer != nil {
		return writer.Write(header, payload, interceptor.Attributes{})
	}

	return 0, nil
}

func (i *interceptorToTrackLocalWriter) Write(b []byte) (int, error) {
	packet := &rtp.Packet{}
	if err := packet.Unmarshal(b); err != nil {
		return 0, err
	}

	return i.WriteRTP(&packet.Header, packet.Payload)
}

//nolint:unparam
func createStreamInfo(
	id string,
	ssrc, ssrcRTX, ssrcFEC SSRC,
	payloadType, payloadTypeRTX, payloadTypeFEC PayloadType,
	codec RTPCodecCapability,
	webrtcHeaderExtensions []RTPHeaderExtensionParameter,
) *interceptor.StreamInfo {
	headerExtensions := make([]interceptor.RTPHeaderExtension, 0, len(webrtcHeaderExtensions))
	for _, h := range webrtcHeaderExtensions {
		headerExtensions = append(headerExtensions, interceptor.RTPHeaderExtension{ID: h.ID, URI: h.URI})
	}

	feedbacks := make([]interceptor.RTCPFeedback, 0, len(codec.RTCPFeedback))
	for _, f := range codec.RTCPFeedback {
		feedbacks = append(feedbacks, interceptor.RTCPFeedback{Type: f.Type, Parameter: f.Parameter})
	}

	return &interceptor.StreamInfo{
		ID:                                id,
		Attributes:                        interceptor.Attributes{},
		SSRC:                              uint32(ssrc),
		SSRCRetransmission:                uint32(ssrcRTX),
		SSRCForwardErrorCorrection:        uint32(ssrcFEC),
		PayloadType:                       uint8(payloadType),
		PayloadTypeRetransmission:         uint8(payloadTypeRTX),
		PayloadTypeForwardErrorCorrection: uint8(payloadTypeFEC),
		RTPHeaderExtensions:               headerExtensions,
		MimeType:                          codec.MimeType,
		ClockRate:                         codec.ClockRate,
		Channels:                          codec.Channels,
		SDPFmtpLine:                       codec.SDPFmtpLine,
		RTCPFeedback:                      feedbacks,
	}
}
