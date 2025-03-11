// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package webrtc

import (
	"context"
	"fmt"
)

// GatheringCompletePromise is a Pion specific helper function that returns a channel that is closed
// when gathering is complete.
// This function may be helpful in cases where you are unable to trickle your ICE Candidates.
//
// It is better to not use this function, and instead trickle candidates.
// If you use this function you will see longer connection startup times.
// When the call is connected you will see no impact however.
func GatheringCompletePromise(pc *PeerConnection) (gatherComplete <-chan struct{}) {

    fmt.Printf("\n【ICE收集】开始创建收集完成承诺通道 (PC=%p)\n", pc)
    fmt.Println("  创建可取消的context...")

    // 创建带有取消功能的上下文
    gatheringComplete, done := context.WithCancel(context.Background())
    fmt.Printf("  上下文地址=%p 取消函数=%v\n", gatheringComplete, done)

    // 设置收集完成回调处理器
    fmt.Println("\n⚙️ 设置ICE收集完成回调处理器")
    pc.setGatherCompleteHandler(func() {
        fmt.Printf("  🔔 ICE收集完成回调触发 (PC=%p)\n", pc)
        done()
        fmt.Println("  已执行取消函数，关闭承诺通道")
    })

    // 检查当前收集状态
    currentState := pc.ICEGatheringState()
    fmt.Printf("\n🔍 检查当前ICE收集状态: %s\n", currentState)
    if currentState == ICEGatheringStateComplete {
        fmt.Println("  ✅ 检测到已完成状态，立即触发完成信号")
        done()
        fmt.Println("  注意：可能错过异步事件，已同步处理")
    } else {
        fmt.Printf("  ⏳ 等待异步完成信号，当前状态: %s\n", currentState)
    }

    // 返回通道并记录信息
    fmt.Printf("\n📨 返回承诺通道 (通道类型: %T)\n", gatheringComplete.Done())
    fmt.Printf("  当前通道状态: %s\n", channelStatus(gatheringComplete.Done()))
    return gatheringComplete.Done()
}

// 辅助函数：判断通道状态
func channelStatus(ch <-chan struct{}) string {
    select {
    case <-ch:
        return "已关闭"
    default:
        return "等待中"
    }
}
