// SPDX-FileCopyrightText: 2023 The Pion community <https://pion.ly>
// SPDX-License-Identifier: MIT

package sdp

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

var (
	errSDPInvalidSyntax       = errors.New("sdp: invalid syntax")
	errSDPInvalidNumericValue = errors.New("sdp: invalid numeric value")
	errSDPInvalidValue        = errors.New("sdp: invalid value")
	errSDPInvalidPortValue    = errors.New("sdp: invalid port value")
	errSDPCacheInvalid        = errors.New("sdp: invalid cache")

	//nolint: gochecknoglobals
	unmarshalCachePool = sync.Pool{
		New: func() interface{} {
			return &unmarshalCache{}
		},
	}
)

// UnmarshalString is the primary function that deserializes the session description
// message and stores it inside of a structured SessionDescription object.
//
// The States Transition Table describes the computation flow between functions
// (namely s1, s2, s3, ...) for a parsing procedure that complies with the
// specifications laid out by the rfc4566#section-5 as well as by JavaScript
// Session Establishment Protocol draft. Links:
//
//	https://tools.ietf.org/html/rfc4566#section-5
//	https://tools.ietf.org/html/draft-ietf-rtcweb-jsep-24
//
// https://tools.ietf.org/html/rfc4566#section-5
// Session description
//
//	v=  (protocol version)
//	o=  (originator and session identifier)
//	s=  (session name)
//	i=* (session information)
//	u=* (URI of description)
//	e=* (email address)
//	p=* (phone number)
//	c=* (connection information -- not required if included in
//	     all media)
//	b=* (zero or more bandwidth information lines)
//	One or more time descriptions ("t=" and "r=" lines; see below)
//	z=* (time zone adjustments)
//	k=* (encryption key)
//	a=* (zero or more session attribute lines)
//	Zero or more media descriptions
//
// Time description
//
//	t=  (time the session is active)
//	r=* (zero or more repeat times)
//
// Media description, if present
//
//	m=  (media name and transport address)
//	i=* (media title)
//	c=* (connection information -- optional if included at
//	     session level)
//	b=* (zero or more bandwidth information lines)
//	k=* (encryption key)
//	a=* (zero or more media attribute lines)
//
// In order to generate the following state table and draw subsequent
// deterministic finite-state automota ("DFA") the following regex was used to
// derive the DFA:
//
//	vosi?u?e?p?c?b*(tr*)+z?k?a*(mi?c?b*k?a*)*
//
// possible place and state to exit:
//
//	**   * * *  ** * * * *
//	99   1 1 1  11 1 1 1 1
//	     3 1 1  26 5 5 4 4
//
// Please pay close attention to the `k`, and `a` parsing states. In the table
// below in order to distinguish between the states belonging to the media
// description as opposed to the session description, the states are marked
// with an asterisk ("a*", "k*").
// +--------+----+-------+----+-----+----+-----+---+----+----+---+---+-----+---+---+----+---+----+
// | STATES | a* | a*,k* | a  | a,k | b  | b,c | e | i  | m  | o | p | r,t | s | t | u  | v | z  |
// +--------+----+-------+----+-----+----+-----+---+----+----+---+---+-----+---+---+----+---+----+
// |   s1   |    |       |    |     |    |     |   |    |    |   |   |     |   |   |    | 2 |    |
// |   s2   |    |       |    |     |    |     |   |    |    | 3 |   |     |   |   |    |   |    |
// |   s3   |    |       |    |     |    |     |   |    |    |   |   |     | 4 |   |    |   |    |
// |   s4   |    |       |    |     |    |   5 | 6 |  7 |    |   | 8 |     |   | 9 | 10 |   |    |
// |   s5   |    |       |    |     |  5 |     |   |    |    |   |   |     |   | 9 |    |   |    |
// |   s6   |    |       |    |     |    |   5 |   |    |    |   | 8 |     |   | 9 |    |   |    |
// |   s7   |    |       |    |     |    |   5 | 6 |    |    |   | 8 |     |   | 9 | 10 |   |    |
// |   s8   |    |       |    |     |    |   5 |   |    |    |   |   |     |   | 9 |    |   |    |
// |   s9   |    |       |    |  11 |    |     |   |    | 12 |   |   |   9 |   |   |    |   | 13 |
// |   s10  |    |       |    |     |    |   5 | 6 |    |    |   | 8 |     |   | 9 |    |   |    |
// |   s11  |    |       | 11 |     |    |     |   |    | 12 |   |   |     |   |   |    |   |    |
// |   s12  |    |    14 |    |     |    |  15 |   | 16 | 12 |   |   |     |   |   |    |   |    |
// |   s13  |    |       |    |  11 |    |     |   |    | 12 |   |   |     |   |   |    |   |    |
// |   s14  | 14 |       |    |     |    |     |   |    | 12 |   |   |     |   |   |    |   |    |
// |   s15  |    |    14 |    |     | 15 |     |   |    | 12 |   |   |     |   |   |    |   |    |
// |   s16  |    |    14 |    |     |    |  15 |   |    | 12 |   |   |     |   |   |    |   |    |
// +--------+----+-------+----+-----+----+-----+---+----+----+---+---+-----+---+---+----+---+----+ .
func (s *SessionDescription) UnmarshalString(value string) error {

    fmt.Printf("\n【SDP解析】开始解析SDP (目标对象=%p 输入长度=%d)\n", s, len(value))
    fmt.Printf("  输入预览: %.40s...\n", value)

    var ok bool
    lex := new(lexer)
    
    // 从对象池获取解析缓存
    fmt.Println("🔄 从缓存池获取解析缓存...")
    lex.cache, ok = unmarshalCachePool.Get().(*unmarshalCache)
    if !ok {
        fmt.Printf("!! 缓存类型断言失败，期望类型: *unmarshalCache\n")
        return errSDPCacheInvalid
    }
    // fmt.Printf("  获取缓存成功 [地址=%p 剩余容量=%d]\n", lex.cache, lex.cache.remainingCapacity())
    defer func() {
        unmarshalCachePool.Put(lex.cache)
        fmt.Printf("♻️ 缓存已回收 [地址=%p]\n", lex.cache)
    }()

    // 初始化词法分析器
    fmt.Println("\n🔄 初始化词法分析器:")
    lex.cache.reset()
    lex.desc = s
    lex.value = value
    fmt.Printf("  目标描述结构: %#v\n", s)
    // fmt.Printf("  当前缓存状态: 行数=%d 媒体块=%d\n", lex.cache.lineNum, len(lex.cache.mediaDescriptions))

    // 状态机处理流程
    fmt.Println("\n🚦 启动状态机解析流程:")
    // stateName := "s1(初始状态)"
    for state := s1; state != nil; {
        // fmt.Printf("  当前状态: %-12s \n", stateName)
        
        // 执行状态处理
        nextState, err := state(lex)
        if err != nil {
            // fmt.Printf("\n!! 状态机错误 [状态=%s] 位置:%d 错误:%v\n", 
            //     stateName, lex.cache.pos, err)
            // fmt.Printf("!! 错误上下文: %q\n", errorContext(lex.value, lex.cache.pos))
            return err
        }
        
        // 更新状态名称
        // stateName = getStateName(nextState)
        state = nextState
    }

    // 处理解析结果
    fmt.Println("\n✅ 解析完成，处理属性:")
    fmt.Printf("  会话级属性数: %d\n", len(lex.cache.sessionAttributes))
    s.Attributes = lex.cache.cloneSessionAttributes()
    
    // fmt.Printf("  媒体块数: %d\n", len(lex.cache.mediaDescriptions))
    populateMediaAttributes(lex.cache, lex.desc)
    
    fmt.Printf("\n最终会话描述结构:\n%+v\n", s)
    return nil
}

// 辅助函数：获取状态名称
func getStateName(state stateFn) string {
    switch fmt.Sprintf("%p", state) {
    case fmt.Sprintf("%p", s1):
        return "s1(解析版本)"
    case fmt.Sprintf("%p", s2):
        return "s2(解析来源)"
    case fmt.Sprintf("%p", s3):
        return "s3(会话名称)"
    // 添加更多状态对应...
    default:
        return "未知状态"
    }
}

// // 辅助函数：错误上下文显示
// func errorContext(s string, pos int) string {
//     start := max(0, pos-20)
//     end := min(len(s), pos+20)
//     return fmt.Sprintf("...%s▶%s...", s[start:pos], s[pos:end])
// }

// Unmarshal converts the value into a []byte and then calls UnmarshalString.
// Callers should use the more performant UnmarshalString.
func (s *SessionDescription) Unmarshal(value []byte) error {
	return s.UnmarshalString(string(value))
}

func s1(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		if key == 'v' {
			return unmarshalProtocolVersion
		}

		return nil
	})
}

func s2(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		if key == 'o' {
			return unmarshalOrigin
		}

		return nil
	})
}

func s3(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		if key == 's' {
			return unmarshalSessionName
		}

		return nil
	})
}

func s4(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'i':
			return unmarshalSessionInformation
		case 'u':
			return unmarshalURI
		case 'e':
			return unmarshalEmail
		case 'p':
			return unmarshalPhone
		case 'c':
			return unmarshalSessionConnectionInformation
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s5(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s6(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'p':
			return unmarshalPhone
		case 'c':
			return unmarshalSessionConnectionInformation
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s7(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'u':
			return unmarshalURI
		case 'e':
			return unmarshalEmail
		case 'p':
			return unmarshalPhone
		case 'c':
			return unmarshalSessionConnectionInformation
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s8(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'c':
			return unmarshalSessionConnectionInformation
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s9(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'z':
			return unmarshalTimeZones
		case 'k':
			return unmarshalSessionEncryptionKey
		case 'a':
			return unmarshalSessionAttribute
		case 'r':
			return unmarshalRepeatTimes
		case 't':
			return unmarshalTiming
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s10(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'e':
			return unmarshalEmail
		case 'p':
			return unmarshalPhone
		case 'c':
			return unmarshalSessionConnectionInformation
		case 'b':
			return unmarshalSessionBandwidth
		case 't':
			return unmarshalTiming
		}

		return nil
	})
}

func s11(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalSessionAttribute
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s12(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalMediaAttribute
		case 'k':
			return unmarshalMediaEncryptionKey
		case 'b':
			return unmarshalMediaBandwidth
		case 'c':
			return unmarshalMediaConnectionInformation
		case 'i':
			return unmarshalMediaTitle
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s13(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalSessionAttribute
		case 'k':
			return unmarshalSessionEncryptionKey
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s14(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalMediaAttribute
		case 'k':
			// Non-spec ordering
			return unmarshalMediaEncryptionKey
		case 'b':
			// Non-spec ordering
			return unmarshalMediaBandwidth
		case 'c':
			// Non-spec ordering
			return unmarshalMediaConnectionInformation
		case 'i':
			// Non-spec ordering
			return unmarshalMediaTitle
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s15(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalMediaAttribute
		case 'k':
			return unmarshalMediaEncryptionKey
		case 'b':
			return unmarshalMediaBandwidth
		case 'c':
			return unmarshalMediaConnectionInformation
		case 'i':
			// Non-spec ordering
			return unmarshalMediaTitle
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func s16(l *lexer) (stateFn, error) {
	return l.handleType(func(key byte) stateFn {
		switch key {
		case 'a':
			return unmarshalMediaAttribute
		case 'k':
			return unmarshalMediaEncryptionKey
		case 'c':
			return unmarshalMediaConnectionInformation
		case 'b':
			return unmarshalMediaBandwidth
		case 'i':
			// Non-spec ordering
			return unmarshalMediaTitle
		case 'm':
			return unmarshalMediaDescription
		}

		return nil
	})
}

func unmarshalProtocolVersion(l *lexer) (stateFn, error) {

    // fmt.Printf("\n=== 开始解析协议版本 (当前行:%d 列:%d) ===\n", l.lineNum, l.colNum)
    defer fmt.Println("=== 协议版本解析结束 ===")

    // 读取协议版本字段
    fmt.Printf("🔍 尝试读取版本号字段...\n")
    version, err := l.readUint64Field()
    if err != nil {
        fmt.Printf("!! 版本号解析失败 | 错误类型:%T\n", err)
        fmt.Printf("!! 错误详情:%v\n", err)
        // fmt.Printf("!! 原始内容上下文:%q\n", errorContext(l.raw, l.pos, 20))
        return nil, fmt.Errorf("%w: %v", errSDPInvalidValue, err)
    }
    fmt.Printf("✅ 读取到协议版本号: %d\n", version)

    // 验证协议版本必须为0
    fmt.Printf("\n🔒 验证RFC规范要求版本必须为0...\n")
    if version != 0 {
        fmt.Printf("!! 非法协议版本 | 预期:0 实际:%d\n", version)
        // fmt.Printf("!! 错误位置: 行%d 列%d\n", l.lineNum, l.colNum)
        // fmt.Printf("!! 错误上下文:%q\n", errorContext(l.raw, l.pos, 40))
        return nil, fmt.Errorf("%w `%v` ")
    }

    // 移动到下一行
    fmt.Printf("\n⏬ 准备移动到下一行...\n")
    if err := l.nextLine(); err != nil {
        fmt.Printf("!! 换行操作失败 | 错误类型:%T\n", err)
        // fmt.Printf("!! 剩余内容:%q\n", l.raw[l.pos:])
        return nil, fmt.Errorf("换行失败: %w", err)
    }
    // fmt.Printf("  成功移动到行%d 新列号:%d\n", l.lineNum+1, l.colNum)

    // 转移到下一个解析状态
    fmt.Printf("\n🔄 切换到下一个解析状态: s2\n")
    return s2, nil
}



func unmarshalOrigin(lex *lexer) (stateFn, error) {
    // fmt.Printf("\n=== 开始解析Origin行 [位置 行%d:%d] ===\n", lex.lineNum, lex.colNum)
    defer fmt.Println("=== Origin行解析结束 ===")

    // 解析用户名
    fmt.Printf("🔍 读取用户名字段...\n")
    username, err := lex.readField()
    if err != nil {
        fmt.Printf("!! 用户名解析失败 | 错误类型:%T\n", err)
        // fmt.Printf("!! 错误上下文:%q\n", errorContext(lex.raw, lex.pos, 20))
        return nil, fmt.Errorf("用户名解析失败: %w", err)
    }
    lex.desc.Origin.Username = username
    fmt.Printf("✅ 用户名: %q\n", username)

    // 解析会话ID
    fmt.Printf("\n🔍 读取会话ID...\n")
    sessionID, err := lex.readUint64Field()
    if err != nil {
        fmt.Printf("!! 会话ID解析失败 | 错误类型:%T\n", err)
        // fmt.Printf("!! 原始内容:%q\n", lex.currentLine())
        return nil, fmt.Errorf("会话ID解析失败: %w", err)
    }
    lex.desc.Origin.SessionID = sessionID
    fmt.Printf("✅ 会话ID: %d\n", sessionID)

    // 解析会话版本
    fmt.Printf("\n🔍 读取会话版本...\n")
    sessionVer, err := lex.readUint64Field()
    if err != nil {
        // fmt.Printf("!! 会话版本解析失败 | 错误值:%q\n", lex.currentField())
        fmt.Printf("!! 错误详情:%v\n", err)
        return nil, fmt.Errorf("会话版本解析失败: %w", err)
    }
    lex.desc.Origin.SessionVersion = sessionVer
    fmt.Printf("✅ 会话版本: %d\n", sessionVer)

    // 解析网络类型
    fmt.Printf("\n🔍 读取网络类型...\n")
    netType, err := lex.readField()
    if err != nil {
        fmt.Printf("!! 网络类型读取失败 | 错误位置:%d\n", lex.pos)
        return nil, fmt.Errorf("网络类型读取失败: %w", err)
    }
    lex.desc.Origin.NetworkType = netType
    fmt.Printf("✅ 网络类型: %q\n", netType)

    // 验证网络类型
    fmt.Printf("\n🔒 验证网络类型(RFC4566#8.2.6)...\n")
    if !anyOf(netType, "IN") {
        fmt.Printf("!! 非法网络类型 | 允许值:IN 实际值:%q\n", netType)
        // fmt.Printf("!! 错误行内容:%q\n", lex.currentLine())
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, netType)
    }

    // 解析地址类型
    fmt.Printf("\n🔍 读取地址类型...\n")
    addrType, err := lex.readField()
    if err != nil {
        // fmt.Printf("!! 地址类型读取失败 | 剩余内容:%q\n", lex.remaining())
        return nil, fmt.Errorf("地址类型读取失败: %w", err)
    }
    lex.desc.Origin.AddressType = addrType
    fmt.Printf("✅ 地址类型: %q\n", addrType)

    // 验证地址类型
    fmt.Printf("\n🔒 验证地址类型(RFC4566#8.2.7)...\n")
    if !anyOf(addrType, "IP4", "IP6") {
        fmt.Printf("!! 非法地址类型 | 允许值:IP4/IP6 实际值:%q\n", addrType)
        // fmt.Printf("!! 错误上下文:%q\n", errorContext(lex.raw, lex.pos, 30))
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, addrType)
    }

    // 解析单播地址
    fmt.Printf("\n🔍 读取单播地址...\n")
    unicastAddr, err := lex.readField()
    if err != nil {
        fmt.Printf("!! 单播地址解析失败 | 错误类型:%T\n", err)
        return nil, fmt.Errorf("单播地址解析失败: %w", err)
    }
    lex.desc.Origin.UnicastAddress = unicastAddr
    fmt.Printf("✅ 单播地址: %q\n", unicastAddr)

    // 移动到下一行
    fmt.Printf("\n⏬ 准备换行...\n")
    if err := lex.nextLine(); err != nil {
        // fmt.Printf("!! 换行失败 | 剩余内容:%q\n", lex.remaining())
        return nil, fmt.Errorf("换行失败: %w", err)
    }
    // fmt.Printf("  成功移动到行%d\n", lex.lineNum+1)

    fmt.Printf("\n🔄 切换到会话名称解析阶段(s3)\n")
    return s3, nil
}

func unmarshalSessionName(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionName】进入函数，开始解析会话名称...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalSessionName】读取到会话名称原始值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalSessionName】错误! 读取失败: %v\n", err)
        return nil, err
    }

    l.desc.SessionName = SessionName(value)
    fmt.Printf("【unmarshalSessionName】成功设置会话名称: %#v\n", l.desc.SessionName)

    fmt.Printf("【unmarshalSessionName】状态转移: s4\n") // 假设 s4 是下一个状态函数名
    return s4, nil
}


func unmarshalSessionInformation(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionInformation】进入函数，开始解析会话附加信息...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalSessionInformation】读取到信息原始值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalSessionInformation】错误! 读取失败: %v\n", err)
        return nil, err
    }

    sessionInformation := Information(value)
    fmt.Printf("【unmarshalSessionInformation】创建 Information 对象: %#v\n", sessionInformation)

    l.desc.SessionInformation = &sessionInformation
    fmt.Printf("【unmarshalSessionInformation】成功设置指针: %p -> 内容:%#v\n", l.desc.SessionInformation, *l.desc.SessionInformation)

    fmt.Printf("【unmarshalSessionInformation】状态转移: s7\n")
    return s7, nil
}

// ================== URI 解析 ==================
func unmarshalURI(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalURI】进入函数，开始解析URI...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalURI】读取到URI原始值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalURI】错误! 读取失败: %v\n", err)
        return nil, err
    }

    l.desc.URI, err = url.Parse(value)
    if err != nil {
        fmt.Printf("【unmarshalURI】错误! URI格式无效: %v | 输入值:%#v\n", err, value)
        return nil, err
    }
    fmt.Printf("【unmarshalURI】成功解析URI结构: Scheme:%q Host:%q Path:%q\n", 
        l.desc.URI.Scheme, l.desc.URI.Host, l.desc.URI.Path)

    fmt.Printf("【unmarshalURI】状态转移: s10\n")
    return s10, nil
}

// ================== 邮箱解析 ==================
func unmarshalEmail(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalEmail】进入函数，开始解析电子邮件地址...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalEmail】读取到邮箱原始值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalEmail】错误! 读取失败: %v\n", err)
        return nil, err
    }

    emailAddress := EmailAddress(value)
    fmt.Printf("【unmarshalEmail】创建 EmailAddress 对象: %#v\n", emailAddress)

    l.desc.EmailAddress = &emailAddress
    fmt.Printf("【unmarshalEmail】成功设置指针: %p -> 内容:%#v\n", l.desc.EmailAddress, *l.desc.EmailAddress)

    fmt.Printf("【unmarshalEmail】状态转移: s6\n")
    return s6, nil
}

// ================== 电话解析 ==================
func unmarshalPhone(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalPhone】进入函数，开始解析电话号码...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalPhone】读取到电话原始值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalPhone】错误! 读取失败: %v\n", err)
        return nil, err
    }

    phoneNumber := PhoneNumber(value)
    fmt.Printf("【unmarshalPhone】创建 PhoneNumber 对象: %#v\n", phoneNumber)

    l.desc.PhoneNumber = &phoneNumber
    fmt.Printf("【unmarshalPhone】成功设置指针: %p -> 内容:%#v\n", l.desc.PhoneNumber, *l.desc.PhoneNumber)

    fmt.Printf("【unmarshalPhone】状态转移: s8\n")
    return s8, nil
}
// ================== 会话连接信息解析 ==================
func unmarshalSessionConnectionInformation(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionConnectionInformation】进入函数，开始解析会话连接信息...\n")

    connInfo, err := l.unmarshalConnectionInformation()
    if err != nil {
        fmt.Printf("【unmarshalSessionConnectionInformation】错误! 连接信息解析失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalSessionConnectionInformation】成功获取连接信息对象: %+v\n", *connInfo)

    l.desc.ConnectionInformation = connInfo
    fmt.Printf("【unmarshalSessionConnectionInformation】已存储连接信息指针: %p\n", connInfo)

    fmt.Printf("【unmarshalSessionConnectionInformation】状态转移: s5\n")
    return s5, nil
}

// ================== 连接信息详细解析 ==================
func (l *lexer) unmarshalConnectionInformation() (*ConnectionInformation, error) {
    fmt.Printf("【unmarshalConnectionInformation】开始解析连接信息字段...\n")
    var connInfo ConnectionInformation

    // 解析网络类型
    networkType, err := l.readField()
    if err != nil {
        fmt.Printf("【unmarshalConnectionInformation】错误! 读取NetworkType失败: %v\n", err)
        return nil, err
    }
    connInfo.NetworkType = networkType
    fmt.Printf("【unmarshalConnectionInformation】读取NetworkType: %q\n", networkType)

    // 验证网络类型 (RFC4566 8.2.6)
    if !anyOf(networkType, "IN") {
        fmt.Printf("【unmarshalConnectionInformation】错误! 无效网络类型 (需为IN): %q\n", networkType)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, networkType)
    }

    // 解析地址类型
    addrType, err := l.readField()
    if err != nil {
        fmt.Printf("【unmarshalConnectionInformation】错误! 读取AddressType失败: %v\n", err)
        return nil, err
    }
    connInfo.AddressType = addrType
    fmt.Printf("【unmarshalConnectionInformation】读取AddressType: %q\n", addrType)

    // 验证地址类型 (RFC4566 8.2.7)
    validAddrTypes := []string{"IP4", "IP6"}
    if !anyOf(addrType, validAddrTypes...) {
        fmt.Printf("【unmarshalConnectionInformation】错误! 无效地址类型 (需为IP4/IP6): %q\n", addrType)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, addrType)
    }

    // 解析地址
    address, err := l.readField()
    if err != nil {
        fmt.Printf("【unmarshalConnectionInformation】错误! 读取Address失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalConnectionInformation】读取原始地址值: %q\n", address)

    if address != "" {
        connInfo.Address = &Address{Address: address}
        fmt.Printf("【unmarshalConnectionInformation】创建地址对象: %#v\n", *connInfo.Address)
    } else {
        fmt.Printf("【unmarshalConnectionInformation】警告! 地址字段为空\n")
    }

    // 推进到下一行
    if err := l.nextLine(); err != nil {
        fmt.Printf("【unmarshalConnectionInformation】错误! 换行失败: %v\n", err)
        return nil, err
    }

    fmt.Printf("【unmarshalConnectionInformation】连接信息完整解析: %+v\n", connInfo)
    return &connInfo, nil
}

// ================== 会话带宽解析 ==================
func unmarshalSessionBandwidth(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionBandwidth】进入函数，开始解析带宽信息...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalSessionBandwidth】读取原始带宽值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalSessionBandwidth】错误! 读取失败: %v\n", err)
        return nil, err
    }

    bandwidth, err := unmarshalBandwidth(value)
    if err != nil {
        fmt.Printf("【unmarshalSessionBandwidth】错误! 格式无效: %v | 原始输入:%#v\n", err, value)
        return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidValue, value)
    }
    fmt.Printf("【unmarshalSessionBandwidth】解析成功: 类型=%s 实验性=%t 带宽值=%d\n", 
        bandwidth.Type, bandwidth.Experimental, bandwidth.Bandwidth)

    l.desc.Bandwidth = append(l.desc.Bandwidth, *bandwidth)
    fmt.Printf("【unmarshalSessionBandwidth】已添加到带宽列表 (当前长度:%d)\n", len(l.desc.Bandwidth))

    fmt.Printf("【unmarshalSessionBandwidth】状态转移: s5\n")
    return s5, nil
}

// ================== 带宽详细解析 ==================
func unmarshalBandwidth(value string) (*Bandwidth, error) {
    fmt.Printf("【unmarshalBandwidth】开始解析带宽字段: %q\n", value)

    parts := strings.Split(value, ":")
    fmt.Printf("【unmarshalBandwidth】分割结果: %#v (段数:%d)\n", parts, len(parts))
    if len(parts) != 2 {
        fmt.Printf("【unmarshalBandwidth】错误! 需要2个字段，实际得到%d个\n", len(parts))
        return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidValue, parts)
    }

    experimental := strings.HasPrefix(parts[0], "X-")
    if experimental {
        fmt.Printf("【unmarshalBandwidth】检测到实验性类型: %q\n", parts[0])
        parts[0] = strings.TrimPrefix(parts[0], "X-")
        fmt.Printf("【unmarshalBandwidth】标准化类型名称: %q\n", parts[0])
    }

    validTypes := []string{"CT", "AS", "TIAS", "RS", "RR"}
    fmt.Printf("【unmarshalBandwidth】验证类型有效性 (RFC4566/5.8)...\n")
    if !experimental && !anyOf(parts[0], validTypes...) {
        fmt.Printf("【unmarshalBandwidth】错误! 无效类型: %q 有效值:%v\n", parts[0], validTypes)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, parts[0])
    }

    fmt.Printf("【unmarshalBandwidth】转换带宽数值: %q -> uint64\n", parts[1])
    bandwidth, err := strconv.ParseUint(parts[1], 10, 64)
    if err != nil {
        fmt.Printf("【unmarshalBandwidth】错误! 数值转换失败: %v | 原始值:%q\n", err, parts[1])
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidNumericValue, parts[1])
    }

    result := &Bandwidth{
        Experimental: experimental,
        Type:         parts[0],
        Bandwidth:    bandwidth,
    }
    fmt.Printf("【unmarshalBandwidth】生成带宽对象: %+v\n", *result)
    return result, nil
}

// ================== 时间描述解析 ==================
func unmarshalTiming(lex *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalTiming】进入函数，开始解析时间范围...\n")
    var td TimeDescription

    // 解析起始时间
    startTime, err := lex.readUint64Field()
    if err != nil {
        fmt.Printf("【unmarshalTiming】错误! 读取起始时间失败: %v\n", err)
        return nil, err
    }
    td.Timing.StartTime = startTime
    fmt.Printf("【unmarshalTiming】读取起始时间: %d (0x%x)\n", startTime, startTime)

    // 解析结束时间
    stopTime, err := lex.readUint64Field()
    if err != nil {
        fmt.Printf("【unmarshalTiming】错误! 读取结束时间失败: %v\n", err)
        return nil, err
    }
    td.Timing.StopTime = stopTime
    fmt.Printf("【unmarshalTiming】读取结束时间: %d (差值:%d)\n", stopTime, stopTime-startTime)

    // 换行处理
    if err := lex.nextLine(); err != nil {
        fmt.Printf("【unmarshalTiming】错误! 换行失败: %v\n", err)
        return nil, err
    }

    lex.desc.TimeDescriptions = append(lex.desc.TimeDescriptions, td)
    fmt.Printf("【unmarshalTiming】成功添加时间描述项 (当前总数:%d)\n", len(lex.desc.TimeDescriptions))
    
    fmt.Printf("【unmarshalTiming】状态转移: s9\n")
    return s9, nil
}

// ================== 周期时间解析 ==================
func unmarshalRepeatTimes(lex *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalRepeatTimes】进入函数，开始解析周期时间规则...\n")
    var newRepeatTime RepeatTime

    // 获取最新时间描述
    if len(lex.desc.TimeDescriptions) == 0 {
        fmt.Printf("【unmarshalRepeatTimes】错误! 时间描述列表为空\n")
        return nil, fmt.Errorf("missing time description")
    }
    latestTimeDesc := &lex.desc.TimeDescriptions[len(lex.desc.TimeDescriptions)-1]
    fmt.Printf("【unmarshalRepeatTimes】关联到最新时间描述项 (索引:%d)\n", len(lex.desc.TimeDescriptions)-1)

    // 解析间隔时间
    intervalField, err := lex.readField()
    if err != nil {
        fmt.Printf("【unmarshalRepeatTimes】错误! 读取间隔时间失败: %v\n", err)
        return nil, err
    }
    interval, err := parseTimeUnits(intervalField)
    if err != nil {
        fmt.Printf("【unmarshalRepeatTimes】错误! 间隔时间格式无效: %q | 错误: %v\n", intervalField, err)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, intervalField)
    }
    newRepeatTime.Interval = interval
    fmt.Printf("【unmarshalRepeatTimes】解析间隔时间: %q -> %d units\n", intervalField, interval)

    // 解析持续时间
    durationField, err := lex.readField()
    if err != nil {
        fmt.Printf("【unmarshalRepeatTimes】错误! 读取持续时间失败: %v\n", err)
        return nil, err
    }
    duration, err := parseTimeUnits(durationField)
    if err != nil {
        fmt.Printf("【unmarshalRepeatTimes】错误! 持续时间格式无效: %q | 错误: %v\n", durationField, err)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, durationField)
    }
    newRepeatTime.Duration = duration
    fmt.Printf("【unmarshalRepeatTimes】解析持续时间: %q -> %d units\n", durationField, duration)

    // 解析偏移量列表
    fmt.Printf("【unmarshalRepeatTimes】开始解析偏移量...\n")
    offsetCount := 0
    for {
        field, err := lex.readField()
        if err != nil {
            fmt.Printf("【unmarshalRepeatTimes】错误! 读取偏移量失败: %v\n", err)
            return nil, err
        }
        if field == "" {
            fmt.Printf("【unmarshalRepeatTimes】偏移量解析完成 (总数:%d)\n", offsetCount)
            break
        }

        offset, err := parseTimeUnits(field)
        if err != nil {
            fmt.Printf("【unmarshalRepeatTimes】错误! 偏移量格式无效: %q | 错误: %v\n", field, err)
            return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, field)
        }
        newRepeatTime.Offsets = append(newRepeatTime.Offsets, offset)
        offsetCount++
        fmt.Printf("【unmarshalRepeatTimes】[偏移量%d] %q -> %d units\n", offsetCount, field, offset)
    }

    // 换行处理
    if err := lex.nextLine(); err != nil {
        fmt.Printf("【unmarshalRepeatTimes】错误! 换行失败: %v\n", err)
        return nil, err
    }

    latestTimeDesc.RepeatTimes = append(latestTimeDesc.RepeatTimes, newRepeatTime)
    fmt.Printf("【unmarshalRepeatTimes】成功添加周期规则 (当前规则数:%d)\n", len(latestTimeDesc.RepeatTimes))
    
    fmt.Printf("【unmarshalRepeatTimes】状态转移: s9\n")
    return s9, nil
}

// ================== 时区解析 ==================
func unmarshalTimeZones(lex *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalTimeZones】进入函数，开始解析时区调整规则...\n")
    pairCount := 0

    for {
        var timeZone TimeZone
        fmt.Printf("【unmarshalTimeZones】解析第 %d 组时区规则\n", pairCount+1)

        // 读取调整时间
        adjTime, err := lex.readUint64Field()
        if err != nil {
            fmt.Printf("【unmarshalTimeZones】错误! 读取调整时间失败: %v\n", err)
            return nil, err
        }
        timeZone.AdjustmentTime = adjTime
        fmt.Printf("【unmarshalTimeZones】[组%d] 调整时间: %d (0x%x)\n", pairCount+1, adjTime, adjTime)

        // 读取偏移量
        offsetStr, err := lex.readField()
        if err != nil {
            fmt.Printf("【unmarshalTimeZones】错误! 读取偏移量失败: %v\n", err)
            return nil, err
        }
        if offsetStr == "" {
            fmt.Printf("【unmarshalTimeZones】检测到空偏移量，终止解析 (已解析组数:%d)\n", pairCount)
            break
        }

        // 解析偏移量
        offset, err := parseTimeUnits(offsetStr)
        if err != nil {
            fmt.Printf("【unmarshalTimeZones】错误! 偏移量格式无效: %q | 错误: %v\n", offsetStr, err)
            return nil, err
        }
        timeZone.Offset = offset
        fmt.Printf("【unmarshalTimeZones】[组%d] 解析偏移量: %q -> %d 单位\n", pairCount+1, offsetStr, offset)

        lex.desc.TimeZones = append(lex.desc.TimeZones, timeZone)
        pairCount++
    }

    if err := lex.nextLine(); err != nil {
        fmt.Printf("【unmarshalTimeZones】错误! 换行失败: %v\n", err)
        return nil, err
    }

    fmt.Printf("【unmarshalTimeZones】成功添加 %d 组时区规则\n", pairCount)
    fmt.Printf("【unmarshalTimeZones】状态转移: s13\n")
    return s13, nil
}

// ================== 加密密钥解析 ==================
func unmarshalSessionEncryptionKey(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionEncryptionKey】进入函数，开始解析加密密钥...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalSessionEncryptionKey】读取原始密钥值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalSessionEncryptionKey】错误! 读取失败: %v\n", err)
        return nil, err
    }

    encryptionKey := EncryptionKey(value)
    l.desc.EncryptionKey = &encryptionKey
    fmt.Printf("【unmarshalSessionEncryptionKey】设置密钥指针: %p -> %#v\n", 
        l.desc.EncryptionKey, *l.desc.EncryptionKey)

    fmt.Printf("【unmarshalSessionEncryptionKey】状态转移: s11\n")
    return s11, nil
}

// ================== 会话属性解析 ==================
func unmarshalSessionAttribute(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalSessionAttribute】进入函数，开始解析会话属性...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalSessionAttribute】读取原始属性值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalSessionAttribute】错误! 读取失败: %v\n", err)
        return nil, err
    }

    a := l.cache.getSessionAttribute()
    fmt.Printf("【unmarshalSessionAttribute】获取属性对象缓存地址: %p\n", a)

    i := strings.IndexRune(value, ':')
    if i > 0 {
        a.Key = value[:i]
        a.Value = value[i+1:]
        fmt.Printf("【unmarshalSessionAttribute】分割键值对成功 | 键:%q 值:%q\n", a.Key, a.Value)
    } else {
        a.Key = value
        fmt.Printf("【unmarshalSessionAttribute】警告! 未找到分隔符，仅设置键名: %q\n", a.Key)
    }

    fmt.Printf("【unmarshalSessionAttribute】最终属性对象: %+v\n", *a)
    fmt.Printf("【unmarshalSessionAttribute】状态转移: s11\n")
    return s11, nil
}

func unmarshalMediaDescription(lex *lexer) (stateFn, error) { //nolint:cyclop
    fmt.Printf("【unmarshalMediaDescription】进入函数，开始解析媒体描述...\n")
    populateMediaAttributes(lex.cache, lex.desc)
    var newMediaDesc MediaDescription

    // 解析媒体类型
    mediaType, err := lex.readField()
    if err != nil {
        fmt.Printf("【unmarshalMediaDescription】错误! 读取媒体类型失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalMediaDescription】读取媒体类型: %q\n", mediaType)

    // 验证IANA注册类型 (RFC4566 5.14)
    validMediaTypes := []string{"audio", "video", "text", "application", "message"}
    if !anyOf(mediaType, validMediaTypes...) {
        fmt.Printf("【unmarshalMediaDescription】错误! 无效媒体类型 (有效值:%v): %q\n", validMediaTypes, mediaType)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, mediaType)
    }
    newMediaDesc.MediaName.Media = mediaType
    fmt.Printf("【unmarshalMediaDescription】设置媒体类型: %s\n", mediaType)

    // 解析端口信息
    portField, err := lex.readField()
    if err != nil {
        fmt.Printf("【unmarshalMediaDescription】错误! 读取端口字段失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalMediaDescription】原始端口字段: %q\n", portField)

    portParts := strings.Split(portField, "/")
    fmt.Printf("【unmarshalMediaDescription】分割端口字段: %#v (段数:%d)\n", portParts, len(portParts))

    basePort, err := parsePort(portParts[0])
    if err != nil {
        fmt.Printf("【unmarshalMediaDescription】错误! 端口号无效: %q | 错误: %v\n", portParts[0], err)
        return nil, fmt.Errorf("%w `%v`", errSDPInvalidPortValue, portParts[0])
    }
    newMediaDesc.MediaName.Port.Value = basePort
    fmt.Printf("【unmarshalMediaDescription】解析基础端口: %d\n", basePort)

    if len(portParts) > 1 {
        portRange, err := strconv.Atoi(portParts[1])
        if err != nil {
            fmt.Printf("【unmarshalMediaDescription】错误! 端口范围无效: %q | 错误: %v\n", portParts[1], err)
            return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, portParts)
        }
        newMediaDesc.MediaName.Port.Range = &portRange
        fmt.Printf("【unmarshalMediaDescription】设置端口范围: %d\n", portRange)
    }

    // 解析协议栈
    protoField, err := lex.readField()
    if err != nil {
        fmt.Printf("【unmarshalMediaDescription】错误! 读取协议字段失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalMediaDescription】原始协议字段: %q\n", protoField)

    protoList := strings.Split(protoField, "/")
    validProtos := []string{"UDP", "RTP", "AVP", "SAVP", "SAVPF", "TLS", "DTLS", "SCTP", "AVPF", "TCP", "MSRP", "BFCP", "UDT", "IX", "MRCPv2"}
    fmt.Printf("【unmarshalMediaDescription】开始协议验证 (RFC4566 5.14)...\n")
    for i, proto := range protoList {
        if !anyOf(proto, validProtos...) {
            fmt.Printf("【unmarshalMediaDescription】错误! 第%d个协议无效 (有效值:%v): %q\n", i+1, validProtos, proto)
            return nil, fmt.Errorf("%w `%v`", errSDPInvalidNumericValue, protoField)
        }
        newMediaDesc.MediaName.Protos = append(newMediaDesc.MediaName.Protos, proto)
        fmt.Printf("【unmarshalMediaDescription】添加协议[%d]: %s\n", i+1, proto)
    }

    // 解析媒体格式
    fmt.Printf("【unmarshalMediaDescription】开始解析格式字段...\n")
    formatCount := 0
    for {
        format, err := lex.readField()
        if err != nil {
            fmt.Printf("【unmarshalMediaDescription】错误! 读取格式字段失败: %v\n", err)
            return nil, err
        }
        if format == "" {
            fmt.Printf("【unmarshalMediaDescription】格式字段解析完成 (总数:%d)\n", formatCount)
            break
        }
        newMediaDesc.MediaName.Formats = append(newMediaDesc.MediaName.Formats, format)
        formatCount++
        fmt.Printf("【unmarshalMediaDescription】添加格式[%d]: %q\n", formatCount, format)
    }

    if err := lex.nextLine(); err != nil {
        fmt.Printf("【unmarshalMediaDescription】错误! 换行失败: %v\n", err)
        return nil, err
    }

    lex.desc.MediaDescriptions = append(lex.desc.MediaDescriptions, &newMediaDesc)
    fmt.Printf("【unmarshalMediaDescription】成功添加媒体描述 (当前总数:%d)\n", len(lex.desc.MediaDescriptions))
    
    fmt.Printf("【unmarshalMediaDescription】状态转移: s12\n")
    return s12, nil
}

// ================== 媒体标题解析 ==================
func unmarshalMediaTitle(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalMediaTitle】进入函数，开始解析媒体标题...\n")

    // 安全获取最新媒体描述
    if len(l.desc.MediaDescriptions) == 0 {
        fmt.Printf("【unmarshalMediaTitle】错误! 媒体描述列表为空\n")
        return nil, fmt.Errorf("no media description available")
    }
    latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
    fmt.Printf("【unmarshalMediaTitle】关联到最新媒体描述 (索引:%d)\n", len(l.desc.MediaDescriptions)-1)

    value, err := l.readLine()
    fmt.Printf("【unmarshalMediaTitle】读取原始标题值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalMediaTitle】错误! 读取失败: %v\n", err)
        return nil, err
    }

    mediaTitle := Information(value)
    latestMediaDesc.MediaTitle = &mediaTitle
    fmt.Printf("【unmarshalMediaTitle】设置媒体标题指针: %p -> %#v\n", 
        latestMediaDesc.MediaTitle, *latestMediaDesc.MediaTitle)

    fmt.Printf("【unmarshalMediaTitle】状态转移: s16\n")
    return s16, nil
}

// ================== 媒体连接信息解析 ==================
func unmarshalMediaConnectionInformation(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalMediaConnectionInformation】进入函数，开始解析媒体级连接信息...\n")

    // 安全获取最新媒体描述
    if len(l.desc.MediaDescriptions) == 0 {
        fmt.Printf("【unmarshalMediaConnectionInformation】错误! 媒体描述列表为空\n")
        return nil, fmt.Errorf("no media description available")
    }
    latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
    fmt.Printf("【unmarshalMediaConnectionInformation】关联到最新媒体描述 (索引:%d)\n", len(l.desc.MediaDescriptions)-1)

    connInfo, err := l.unmarshalConnectionInformation()
    if err != nil {
        fmt.Printf("【unmarshalMediaConnectionInformation】错误! 连接信息解析失败: %v\n", err)
        return nil, err
    }
    fmt.Printf("【unmarshalMediaConnectionInformation】获取连接信息对象: %+v\n", *connInfo)

    latestMediaDesc.ConnectionInformation = connInfo
    fmt.Printf("【unmarshalMediaConnectionInformation】已存储连接信息指针: %p\n", connInfo)

    fmt.Printf("【unmarshalMediaConnectionInformation】状态转移: s15\n")
    return s15, nil
}

// ================== 媒体带宽解析 ==================
func unmarshalMediaBandwidth(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalMediaBandwidth】进入函数，开始解析媒体级带宽...\n")

    // 安全获取最新媒体描述
    if len(l.desc.MediaDescriptions) == 0 {
        fmt.Printf("【unmarshalMediaBandwidth】错误! 媒体描述列表为空\n")
        return nil, fmt.Errorf("no media description available")
    }
    latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
    fmt.Printf("【unmarshalMediaBandwidth】关联到最新媒体描述 (索引:%d)\n", len(l.desc.MediaDescriptions)-1)

    value, err := l.readLine()
    fmt.Printf("【unmarshalMediaBandwidth】读取原始带宽值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalMediaBandwidth】错误! 读取失败: %v\n", err)
        return nil, err
    }

    bandwidth, err := unmarshalBandwidth(value)
    if err != nil {
        fmt.Printf("【unmarshalMediaBandwidth】错误! 带宽解析失败: %v | 原始输入:%#v\n", err, value)
        return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidSyntax, value)
    }
    fmt.Printf("【unmarshalMediaBandwidth】解析成功: 类型=%s 实验性=%t 值=%d\n",
        bandwidth.Type, bandwidth.Experimental, bandwidth.Bandwidth)

    latestMediaDesc.Bandwidth = append(latestMediaDesc.Bandwidth, *bandwidth)
    fmt.Printf("【unmarshalMediaBandwidth】添加到带宽列表 (当前数量:%d)\n", len(latestMediaDesc.Bandwidth))

    fmt.Printf("【unmarshalMediaBandwidth】状态转移: s15\n")
    return s15, nil
}

// ================== 媒体加密密钥解析 ==================
func unmarshalMediaEncryptionKey(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalMediaEncryptionKey】进入函数，开始解析媒体加密密钥...\n")

    // 安全获取最新媒体描述
    if len(l.desc.MediaDescriptions) == 0 {
        fmt.Printf("【unmarshalMediaEncryptionKey】错误! 媒体描述列表为空\n")
        return nil, fmt.Errorf("no media description available")
    }
    latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
    fmt.Printf("【unmarshalMediaEncryptionKey】关联到最新媒体描述 (索引:%d)\n", len(l.desc.MediaDescriptions)-1)

    value, err := l.readLine()
    secureValue := "******" + value[len(value)-4:] // 敏感信息脱敏
    fmt.Printf("【unmarshalMediaEncryptionKey】读取密钥值 (脱敏): %q (原始长度:%d)\n", secureValue, len(value))
    if err != nil {
        fmt.Printf("【unmarshalMediaEncryptionKey】错误! 读取失败: %v\n", err)
        return nil, err
    }

    encryptionKey := EncryptionKey(value)
    latestMediaDesc.EncryptionKey = &encryptionKey
    fmt.Printf("【unmarshalMediaEncryptionKey】设置密钥指针地址: %p | 存储状态: %t\n", 
        latestMediaDesc.EncryptionKey, latestMediaDesc.EncryptionKey != nil)

    fmt.Printf("【unmarshalMediaEncryptionKey】状态转移: s14\n")
    return s14, nil
}

// ================== 媒体属性解析 ==================
func unmarshalMediaAttribute(l *lexer) (stateFn, error) {
    fmt.Printf("【unmarshalMediaAttribute】进入函数，开始解析媒体属性...\n")

    value, err := l.readLine()
    fmt.Printf("【unmarshalMediaAttribute】读取原始属性值: %q (长度:%d)\n", value, len(value))
    if err != nil {
        fmt.Printf("【unmarshalMediaAttribute】错误! 读取失败: %v\n", err)
        return nil, err
    }

    a := l.cache.getMediaAttribute()
    fmt.Printf("【unmarshalMediaAttribute】获取属性缓存对象地址: %p\n", a)

    i := strings.IndexRune(value, ':')
    if i > 0 {
        a.Key = value[:i]
        a.Value = value[i+1:]
        fmt.Printf("【unmarshalMediaAttribute】键值分割成功 | 位置:%d 键:%q 值长度:%d\n", 
            i, a.Key, len(a.Value))
    } else {
        a.Key = value
        fmt.Printf("【unmarshalMediaAttribute】警告! 未找到分隔符，仅设置键名: %q\n", a.Key)
    }

    fmt.Printf("【unmarshalMediaAttribute】最终属性对象: %s=%s\n", a.Key, a.Value)
    fmt.Printf("【unmarshalMediaAttribute】状态转移: s14\n")
    return s14, nil
}

// ================== 时间单位解析 ==================
var timeUnitMap = map[byte]struct {
    Name string
    Mult int64
}{
    's': {"秒", 1},
    'm': {"分钟", 60},
    'h': {"小时", 60 * 60},
    'd': {"天", 24 * 60 * 60},
}

func parseTimeUnits(value string) (num int64, err error) {
    fmt.Printf("【parseTimeUnits】开始解析时间单位: %q\n", value)
    defer func() {
        if err == nil {
            fmt.Printf("【parseTimeUnits】转换结果: %d 秒\n", num)
        }
    }()

    if len(value) == 0 {
        fmt.Printf("【parseTimeUnits】错误! 输入为空\n")
        return 0, fmt.Errorf("%w `%v`", errSDPInvalidValue, value)
    }

    lastChar := value[len(value)-1]
    unit, isUnit := timeUnitMap[lastChar]
    
    var numStr string
    if isUnit {
        numStr = value[:len(value)-1]
        fmt.Printf("【parseTimeUnits】检测到时间单位: %s (%c)\n", unit.Name, lastChar)
    } else {
        numStr = value
        fmt.Printf("【parseTimeUnits】未检测到单位符号，默认使用秒\n")
    }

    num, err = strconv.ParseInt(numStr, 10, 64)
    if err != nil {
        fmt.Printf("【parseTimeUnits】错误! 数值转换失败: %v | 原始输入: %q\n", err, numStr)
        return 0, fmt.Errorf("%w `%v`", errSDPInvalidValue, value)
    }

    if isUnit {
        num *= unit.Mult
        fmt.Printf("【parseTimeUnits】应用单位系数: %d × %d\n", num/unit.Mult, unit.Mult)
    }

    return num, nil
}


func timeShorthand(b byte) int64 {
	// Some time offsets in the protocol can be provided with a shorthand
	// notation. This code ensures to convert it to NTP timestamp format.
	switch b {
	case 'd': // days
		return 86400
	case 'h': // hours
		return 3600
	case 'm': // minutes
		return 60
	case 's': // seconds (allowed for completeness)
		return 1
	default:
		return 0
	}
}

func parsePort(value string) (int, error) {
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w `%v`", errSDPInvalidPortValue, port)
	}

	if port < 0 || port > 65536 {
		return 0, fmt.Errorf("%w -- out of range `%v`", errSDPInvalidPortValue, port)
	}

	return port, nil
}

func populateMediaAttributes(c *unmarshalCache, s *SessionDescription) {
    fmt.Printf("\n=== 开始填充媒体属性 [会话描述地址:%p 缓存地址:%p] ===\n", s, c)
    defer fmt.Println("=== 媒体属性填充完成 ===")

    // 检查媒体描述是否存在
    if len(s.MediaDescriptions) == 0 {
        fmt.Println("⚠️ 警告：没有媒体描述需要处理")
        return
    }

    // 获取最后一个媒体描述
    lastIndex := len(s.MediaDescriptions) - 1
    lastMediaDesc := s.MediaDescriptions[lastIndex]
    fmt.Printf("  目标媒体描述：第 %d 个（共 %d 个媒体块）\n", lastIndex+1, len(s.MediaDescriptions))
    fmt.Printf("  原始属性数量：%d\n", len(lastMediaDesc.Attributes))

    // 克隆缓存中的属性
    fmt.Println("\n🔧 从缓存克隆媒体属性...")
    clonedAttrs := c.cloneMediaAttributes()
    fmt.Printf("  克隆属性数量：%d\n", len(clonedAttrs))

    // 打印前3个属性示例（避免泄露敏感信息）
    if len(clonedAttrs) > 0 {
        fmt.Println("  示例属性：")
        for i, attr := range clonedAttrs[:min(len(clonedAttrs), len(clonedAttrs))] {
            fmt.Printf("  %d.  %s : %s\n", i+1, sanitizeAttribute(attr.Key), sanitizeAttribute(attr.Value))
        }
    }

    // 更新媒体描述属性
    lastMediaDesc.Attributes = clonedAttrs
    fmt.Printf("\n✅ 属性更新完成 最终属性数：%d\n", len(lastMediaDesc.Attributes))
}

// 辅助函数：敏感信息过滤
func sanitizeAttribute(attr string) string {
    if strings.HasPrefix(strings.ToLower(attr), "crypto:") {
        parts := strings.Split(attr, " ")
        if len(parts) > 2 {
            parts[2] = "[REDACTED]"
            return strings.Join(parts, " ")
        }
    }
    return attr
}

// 辅助函数：取最小值
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
