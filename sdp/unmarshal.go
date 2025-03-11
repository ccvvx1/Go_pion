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
	version, err := l.readUint64Field()
	if err != nil {
		return nil, err
	}

	// As off the latest draft of the rfc this value is required to be 0.
	// https://tools.ietf.org/html/draft-ietf-rtcweb-jsep-24#section-5.8.1
	if version != 0 {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, version)
	}

	if err := l.nextLine(); err != nil {
		return nil, err
	}

	return s2, nil
}

func unmarshalOrigin(lex *lexer) (stateFn, error) {
	var err error

	lex.desc.Origin.Username, err = lex.readField()
	if err != nil {
		return nil, err
	}

	lex.desc.Origin.SessionID, err = lex.readUint64Field()
	if err != nil {
		return nil, err
	}

	lex.desc.Origin.SessionVersion, err = lex.readUint64Field()
	if err != nil {
		return nil, err
	}

	lex.desc.Origin.NetworkType, err = lex.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-8.2.6
	if !anyOf(lex.desc.Origin.NetworkType, "IN") {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, lex.desc.Origin.NetworkType)
	}

	lex.desc.Origin.AddressType, err = lex.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-8.2.7
	if !anyOf(lex.desc.Origin.AddressType, "IP4", "IP6") {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, lex.desc.Origin.AddressType)
	}

	lex.desc.Origin.UnicastAddress, err = lex.readField()
	if err != nil {
		return nil, err
	}

	if err := lex.nextLine(); err != nil {
		return nil, err
	}

	return s3, nil
}

func unmarshalSessionName(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	l.desc.SessionName = SessionName(value)

	return s4, nil
}

func unmarshalSessionInformation(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	sessionInformation := Information(value)
	l.desc.SessionInformation = &sessionInformation

	return s7, nil
}

func unmarshalURI(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	l.desc.URI, err = url.Parse(value)
	if err != nil {
		return nil, err
	}

	return s10, nil
}

func unmarshalEmail(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	emailAddress := EmailAddress(value)
	l.desc.EmailAddress = &emailAddress

	return s6, nil
}

func unmarshalPhone(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	phoneNumber := PhoneNumber(value)
	l.desc.PhoneNumber = &phoneNumber

	return s8, nil
}

func unmarshalSessionConnectionInformation(l *lexer) (stateFn, error) {
	var err error
	l.desc.ConnectionInformation, err = l.unmarshalConnectionInformation()
	if err != nil {
		return nil, err
	}

	return s5, nil
}

func (l *lexer) unmarshalConnectionInformation() (*ConnectionInformation, error) {
	var err error
	var connInfo ConnectionInformation

	connInfo.NetworkType, err = l.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-8.2.6
	if !anyOf(connInfo.NetworkType, "IN") {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, connInfo.NetworkType)
	}

	connInfo.AddressType, err = l.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-8.2.7
	if !anyOf(connInfo.AddressType, "IP4", "IP6") {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, connInfo.AddressType)
	}

	address, err := l.readField()
	if err != nil {
		return nil, err
	}

	if address != "" {
		connInfo.Address = new(Address)
		connInfo.Address.Address = address
	}

	if err := l.nextLine(); err != nil {
		return nil, err
	}

	return &connInfo, nil
}

func unmarshalSessionBandwidth(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	bandwidth, err := unmarshalBandwidth(value)
	if err != nil {
		return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidValue, value)
	}
	l.desc.Bandwidth = append(l.desc.Bandwidth, *bandwidth)

	return s5, nil
}

func unmarshalBandwidth(value string) (*Bandwidth, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidValue, parts)
	}

	experimental := strings.HasPrefix(parts[0], "X-")
	if experimental {
		parts[0] = strings.TrimPrefix(parts[0], "X-")
	} else if !anyOf(parts[0], "CT", "AS", "TIAS", "RS", "RR") {
		// Set according to currently registered with IANA
		// https://tools.ietf.org/html/rfc4566#section-5.8
		// https://tools.ietf.org/html/rfc3890#section-6.2
		// https://tools.ietf.org/html/rfc3556#section-2
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, parts[0])
	}

	bandwidth, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidNumericValue, parts[1])
	}

	return &Bandwidth{
		Experimental: experimental,
		Type:         parts[0],
		Bandwidth:    bandwidth,
	}, nil
}

func unmarshalTiming(lex *lexer) (stateFn, error) {
	var err error
	var td TimeDescription

	td.Timing.StartTime, err = lex.readUint64Field()
	if err != nil {
		return nil, err
	}

	td.Timing.StopTime, err = lex.readUint64Field()
	if err != nil {
		return nil, err
	}

	if err := lex.nextLine(); err != nil {
		return nil, err
	}

	lex.desc.TimeDescriptions = append(lex.desc.TimeDescriptions, td)

	return s9, nil
}

func unmarshalRepeatTimes(lex *lexer) (stateFn, error) {
	var err error
	var newRepeatTime RepeatTime

	latestTimeDesc := &lex.desc.TimeDescriptions[len(lex.desc.TimeDescriptions)-1]

	field, err := lex.readField()
	if err != nil {
		return nil, err
	}

	newRepeatTime.Interval, err = parseTimeUnits(field)
	if err != nil {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, field)
	}

	field, err = lex.readField()
	if err != nil {
		return nil, err
	}

	newRepeatTime.Duration, err = parseTimeUnits(field)
	if err != nil {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, field)
	}

	for {
		field, err := lex.readField()
		if err != nil {
			return nil, err
		}
		if field == "" {
			break
		}
		offset, err := parseTimeUnits(field)
		if err != nil {
			return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, field)
		}
		newRepeatTime.Offsets = append(newRepeatTime.Offsets, offset)
	}

	if err := lex.nextLine(); err != nil {
		return nil, err
	}

	latestTimeDesc.RepeatTimes = append(latestTimeDesc.RepeatTimes, newRepeatTime)

	return s9, nil
}

func unmarshalTimeZones(lex *lexer) (stateFn, error) {
	// These fields are transimitted in pairs
	// z=<adjustment time> <offset> <adjustment time> <offset> ....
	// so we are making sure that there are actually multiple of 2 total.
	for {
		var err error
		var timeZone TimeZone

		timeZone.AdjustmentTime, err = lex.readUint64Field()
		if err != nil {
			return nil, err
		}

		offset, err := lex.readField()
		if err != nil {
			return nil, err
		}

		if offset == "" {
			break
		}

		timeZone.Offset, err = parseTimeUnits(offset)
		if err != nil {
			return nil, err
		}

		lex.desc.TimeZones = append(lex.desc.TimeZones, timeZone)
	}

	if err := lex.nextLine(); err != nil {
		return nil, err
	}

	return s13, nil
}

func unmarshalSessionEncryptionKey(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	encryptionKey := EncryptionKey(value)
	l.desc.EncryptionKey = &encryptionKey

	return s11, nil
}

func unmarshalSessionAttribute(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	i := strings.IndexRune(value, ':')
	a := l.cache.getSessionAttribute()
	if i > 0 {
		a.Key = value[:i]
		a.Value = value[i+1:]
	} else {
		a.Key = value
	}

	return s11, nil
}

func unmarshalMediaDescription(lex *lexer) (stateFn, error) { //nolint:cyclop
	populateMediaAttributes(lex.cache, lex.desc)
	var newMediaDesc MediaDescription

	// <media>
	field, err := lex.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-5.14
	if !anyOf(field, "audio", "video", "text", "application", "message") {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, field)
	}
	newMediaDesc.MediaName.Media = field

	// <port>
	field, err = lex.readField()
	if err != nil {
		return nil, err
	}
	parts := strings.Split(field, "/")
	newMediaDesc.MediaName.Port.Value, err = parsePort(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w `%v`", errSDPInvalidPortValue, parts[0])
	}

	if len(parts) > 1 {
		var portRange int
		portRange, err = strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("%w `%v`", errSDPInvalidValue, parts)
		}
		newMediaDesc.MediaName.Port.Range = &portRange
	}

	// <proto>
	field, err = lex.readField()
	if err != nil {
		return nil, err
	}

	// Set according to currently registered with IANA
	// https://tools.ietf.org/html/rfc4566#section-5.14
	// https://tools.ietf.org/html/rfc4975#section-8.1
	for _, proto := range strings.Split(field, "/") {
		if !anyOf(
			proto,
			"UDP",
			"RTP",
			"AVP",
			"SAVP",
			"SAVPF",
			"TLS",
			"DTLS",
			"SCTP",
			"AVPF",
			"TCP",
			"MSRP",
			"BFCP",
			"UDT",
			"IX",
			"MRCPv2",
		) {
			return nil, fmt.Errorf("%w `%v`", errSDPInvalidNumericValue, field)
		}
		newMediaDesc.MediaName.Protos = append(newMediaDesc.MediaName.Protos, proto)
	}

	// <fmt>...
	for {
		field, err = lex.readField()
		if err != nil {
			return nil, err
		}
		if field == "" {
			break
		}
		newMediaDesc.MediaName.Formats = append(newMediaDesc.MediaName.Formats, field)
	}

	if err := lex.nextLine(); err != nil {
		return nil, err
	}

	lex.desc.MediaDescriptions = append(lex.desc.MediaDescriptions, &newMediaDesc)

	return s12, nil
}

func unmarshalMediaTitle(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
	mediaTitle := Information(value)
	latestMediaDesc.MediaTitle = &mediaTitle

	return s16, nil
}

func unmarshalMediaConnectionInformation(l *lexer) (stateFn, error) {
	var err error
	latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
	latestMediaDesc.ConnectionInformation, err = l.unmarshalConnectionInformation()
	if err != nil {
		return nil, err
	}

	return s15, nil
}

func unmarshalMediaBandwidth(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
	bandwidth, err := unmarshalBandwidth(value)
	if err != nil {
		return nil, fmt.Errorf("%w `b=%v`", errSDPInvalidSyntax, value)
	}
	latestMediaDesc.Bandwidth = append(latestMediaDesc.Bandwidth, *bandwidth)

	return s15, nil
}

func unmarshalMediaEncryptionKey(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	latestMediaDesc := l.desc.MediaDescriptions[len(l.desc.MediaDescriptions)-1]
	encryptionKey := EncryptionKey(value)
	latestMediaDesc.EncryptionKey = &encryptionKey

	return s14, nil
}

func unmarshalMediaAttribute(l *lexer) (stateFn, error) {
	value, err := l.readLine()
	if err != nil {
		return nil, err
	}

	i := strings.IndexRune(value, ':')
	a := l.cache.getMediaAttribute()
	if i > 0 {
		a.Key = value[:i]
		a.Value = value[i+1:]
	} else {
		a.Key = value
	}

	return s14, nil
}

func parseTimeUnits(value string) (num int64, err error) {
	if len(value) == 0 {
		return 0, fmt.Errorf("%w `%v`", errSDPInvalidValue, value)
	}
	k := timeShorthand(value[len(value)-1])
	if k > 0 {
		num, err = strconv.ParseInt(value[:len(value)-1], 10, 64)
	} else {
		k = 1
		num, err = strconv.ParseInt(value, 10, 64)
	}
	if err != nil {
		return 0, fmt.Errorf("%w `%v`", errSDPInvalidValue, value)
	}

	return num * k, nil
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
