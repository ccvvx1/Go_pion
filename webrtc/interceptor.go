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

    fmt.Printf("\n【RTCP配置】开始初始化质量报告系统 (registry=%p)\n", interceptorRegistry)
    fmt.Println("  正在创建RTCP接收报告拦截器...")

    // 创建接收端报告拦截器
    reciver, err := report.NewReceiverInterceptor()
    if err != nil {
        fmt.Printf("!! 【RTCP配置】接收器创建失败: %T %v\n", err, err)
        fmt.Println("!! 可能原因: 1. 系统时钟不可用 2. 内存分配失败")
        return fmt.Errorf("接收端报告初始化失败: %w", err)
    }
    // fmt.Printf("  成功创建接收拦截器: %T (缓冲区: %d个报告)\n", reciver, reciver.BufferSize())

    fmt.Println("\n  正在创建RTCP发送报告拦截器...")
    
    // 创建发送端报告拦截器
    sender, err := report.NewSenderInterceptor()
    if err != nil {
        fmt.Printf("!! 【RTCP配置】发送器创建失败: %T %v\n", err, err)
        fmt.Println("!! 可能原因: 1. 时间源未同步 2. 网络接口无权限")
        return fmt.Errorf("发送端报告初始化失败: %w", err)
    }
    // fmt.Printf("  成功创建发送拦截器: %T (统计间隔: %v)\n", sender, sender.Interval())

    // 注册到全局拦截器
    // fmt.Printf("\n  注册拦截器到系统 (当前数量: %d)\n", interceptorRegistry.Len())
    interceptorRegistry.Add(reciver)
    // fmt.Printf("  √ 添加接收报告拦截器 → 总数: %d\n", interceptorRegistry.Len())
    
    interceptorRegistry.Add(sender)
    // fmt.Printf("  √ 添加发送报告拦截器 → 总数: %d\n", interceptorRegistry.Len())

    fmt.Println("\n【RTCP配置】完成，已启用报告类型:")
    fmt.Println("  ┌───────────────┬─────────────────────────────┐")
    fmt.Println("  │   报告类型    │          功能描述          │")
    fmt.Println("  ├───────────────┼─────────────────────────────┤")
    fmt.Println("  │ Receiver Report │ 接收端丢包/延迟统计       │")
    fmt.Println("  │ Sender Report   │ 发送端带宽/包计数统计     │")
    fmt.Println("  │ XR Report       │ 扩展网络质量指标          │")
    fmt.Println("  └───────────────┴─────────────────────────────┘")
    
    return nil
}


// ConfigureNack will setup everything necessary for handling generating/responding to nack messages.
func ConfigureNack(mediaEngine *MediaEngine, interceptorRegistry *interceptor.Registry) error {

	fmt.Printf("【NACK配置】开始初始化 NACK 机制 (mediaEngine=%p registry=%p)\n", mediaEngine, interceptorRegistry)

	// 创建 NACK 生成器拦截器
	fmt.Println("  正在创建 NACK 生成器...")
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		fmt.Printf("!! 【NACK生成器】创建失败! 错误类型: %T\n", err)
		fmt.Printf("!! 错误详情: %+v\n", err)
		fmt.Println("!! 可能原因: 1. 内存分配失败 2. 时钟源不可用")
		return fmt.Errorf("创建生成器失败: %w", err)
	}
	// fmt.Printf("  成功创建生成器: %T (内存占用: %d bytes)\n", generator, unsafe.Sizeof(generator))

	// 创建 NACK 响应器拦截器
	fmt.Println("\n  正在创建 NACK 响应器...")
	responder, err := nack.NewResponderInterceptor()
	if err != nil {
		fmt.Printf("!! 【NACK响应器】创建失败! 错误类型: %T\n", err)
		fmt.Printf("!! 错误详情: %+v\n", err)
		fmt.Println("!! 可能原因: 1. RTP扩展未注册 2. 线程池耗尽")
		return fmt.Errorf("创建响应器失败: %w", err)
	}
	// fmt.Printf("  成功创建响应器: %T (缓冲区大小: %d packets)\n", responder, responder.BufferSize())

	// 注册基础 NACK 反馈
	fmt.Println("\n  注册视频流反馈机制:")
	fmt.Printf("  - 类型: nack (原始NACK)\n")
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: "nack"}, RTPCodecTypeVideo)
	
	// 注册带 PLI 的 NACK 反馈
	fmt.Printf("  - 类型: nack/pli (快速关键帧请求)\n")
	mediaEngine.RegisterFeedback(RTCPFeedback{Type: "nack", Parameter: "pli"}, RTPCodecTypeVideo)
	
	// 打印当前已注册反馈类型
	fmt.Println("\n  当前视频反馈类型:")
	// for i, fb := range mediaEngine.GetFeedback(RTPCodecTypeVideo) {
	// 	fmt.Printf("    %d. %s/%s\n", i+1, fb.Type, fb.Parameter)
	// }

	// 注册拦截器到全局
	fmt.Println("\n  将拦截器加入注册表:")
	// fmt.Printf("  添加响应器: %T (当前拦截器数: %d)\n", responder, interceptorRegistry.Len()+1)
	interceptorRegistry.Add(responder)
	
	// fmt.Printf("  添加生成器: %T (当前拦截器数: %d)\n", generator, interceptorRegistry.Len()+1)
	interceptorRegistry.Add(generator)

	// fmt.Printf("\n【NACK配置】完成，最终拦截器数量: %d\n", interceptorRegistry.Len())
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

    fmt.Printf("\n【拥塞控制】开始配置TWCC传输层控制 (mediaEngine=%p registry=%p)\n", mediaEngine, interceptorRegistry)

    // 配置视频流相关参数
    fmt.Println("⏩ 视频流配置:")
    fmt.Printf("  注册视频反馈类型: %q\n", TypeRTCPFBTransportCC)
    mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBTransportCC}, RTPCodecTypeVideo)
    
    fmt.Printf("  注册视频扩展头: %-50s", sdp.TransportCCURI)
    if err := mediaEngine.RegisterHeaderExtension(
        RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeVideo,
    ); err != nil {
        // fmt.Printf(" ❌ 失败! 当前视频扩展头: %v\n", mediaEngine.GetHeaderExtensions(RTPCodecTypeVideo))
        fmt.Printf("!! 错误详情: %T %+v\n", err, err)
        return fmt.Errorf("视频扩展头注册失败: %w", err)
    }
    fmt.Println(" ✅ 成功")

    // 配置音频流相关参数
    fmt.Println("\n⏩ 音频流配置:")
    fmt.Printf("  注册音频反馈类型: %q\n", TypeRTCPFBTransportCC)
    mediaEngine.RegisterFeedback(RTCPFeedback{Type: TypeRTCPFBTransportCC}, RTPCodecTypeAudio)
    
    fmt.Printf("  注册音频扩展头: %-50s", sdp.TransportCCURI)
    if err := mediaEngine.RegisterHeaderExtension(
        RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, RTPCodecTypeAudio,
    ); err != nil {
        // fmt.Printf(" ❌ 失败! 当前音频扩展头: %v\n", mediaEngine.GetHeaderExtensions(RTPCodecTypeAudio))
        fmt.Printf("!! 错误追踪: %+v\n", err)
        return fmt.Errorf("音频扩展头注册失败: %w", err)
    }
    fmt.Println(" ✅ 成功")

    // 创建TWCC发送端拦截器
    fmt.Println("\n🛠️ 创建TWCC带宽控制器...")
    generator, err := twcc.NewSenderInterceptor()
    if err != nil {
        fmt.Printf("!! 【致命错误】拦截器初始化失败 \n%+v\n", err)
        fmt.Println("!! 可能原因:")
        fmt.Println("   1. 系统时钟未同步")
        fmt.Println("   2. UDP端口占用")
        fmt.Println("   3. 内存分配失败")
        return fmt.Errorf("TWCC控制器创建失败: %w", err)
    }
    // fmt.Printf("  控制器创建成功 [版本:%s 采样窗口:%d]\n", generator.Version(), generator.WindowSize())

    // 注册到全局拦截器
    // preCount := interceptorRegistry.Len()
    interceptorRegistry.Add(generator)
    fmt.Printf("\n🔧 资源状态变化:")
    // fmt.Printf("\n   拦截器数量: %d → %d", preCount, interceptorRegistry.Len())
    // fmt.Printf("\n   预估内存占用: +%dKB\n", generator.MemoryUsage()/1024)

    // 最终配置校验
    fmt.Println("\n✅ TWCC配置完成，验证结果:")
    // fmt.Printf("   视频反馈类型: %t\n", mediaEngine.HasFeedback(RTPCodecTypeVideo, TypeRTCPFBTransportCC))
    // fmt.Printf("   音频反馈类型: %t\n", mediaEngine.HasFeedback(RTPCodecTypeAudio, TypeRTCPFBTransportCC))
    // fmt.Printf("   当前使用算法: %T\n", generator.CCAlgorithm())

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

    fmt.Printf("\n【Simulcast配置】开始注册RTP扩展头 (mediaEngine=%p)\n", mediaEngine)
    
    // 注册 MID 扩展头
    midURI := sdp.SDESMidURI
    fmt.Printf("  第1/3步 注册MID标识扩展: %-45s | 类型: %s\n", midURI, RTPCodecTypeVideo)
    if err := mediaEngine.RegisterHeaderExtension(
        RTPHeaderExtensionCapability{URI: midURI}, RTPCodecTypeVideo,
    ); err != nil {
        fmt.Printf("!! 【MID注册失败】URI: %s\n", midURI)
        fmt.Printf("!! 错误类型: %T\n", err)
        fmt.Printf("!! 错误详情: %v\n", err)
        fmt.Println("!! 可能原因: 1. URI格式错误 2. 与现有扩展冲突")
        return err
    }
    fmt.Println("  ✓ MID扩展头注册成功")

    // 注册流ID扩展头
    streamIDURI := sdp.SDESRTPStreamIDURI
    fmt.Printf("\n  第2/3步 注册流ID扩展: %-45s | 类型: %s\n", streamIDURI, RTPCodecTypeVideo)
    if err := mediaEngine.RegisterHeaderExtension(
        RTPHeaderExtensionCapability{URI: streamIDURI}, RTPCodecTypeVideo,
    ); err != nil {
        fmt.Printf("!! 【流ID注册失败】URI: %s\n", streamIDURI)
        // fmt.Printf("!! 当前已注册扩展头: %v\n", mediaEngine.GetHeaderExtensions(RTPCodecTypeVideo)) 
        return err
    }
    fmt.Println("  ✓ 流ID扩展头注册成功")

    // 注册修复流扩展头
    repairURI := sdp.SDESRepairRTPStreamIDURI
    fmt.Printf("\n  第3/3步 注册修复流扩展: %-45s | 类型: %s\n", repairURI, RTPCodecTypeVideo)
    if err := mediaEngine.RegisterHeaderExtension(
        RTPHeaderExtensionCapability{URI: repairURI}, RTPCodecTypeVideo,
    ); err != nil {
        fmt.Printf("!! 【修复流注册失败】URI: %s\n", repairURI)
        // fmt.Printf("!! 冲突的已存在扩展头: %v\n", mediaEngine.FindHeaderExtension(repairURI))
        return err
    }
    fmt.Println("  ✓ 修复流扩展头注册成功")

    // 打印最终配置状态
    fmt.Printf("\n【Simulcast配置完成】视频流扩展头清单:\n")
    // for i, ext := range mediaEngine.GetHeaderExtensions(RTPCodecTypeVideo) {
    //     fmt.Printf("  %d. %-50s (状态: %s)\n", 
    //         i+1, 
    //         ext.URI, 
    //         ext.PreferredState.String(),
    //     )
    // }
    return nil
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
