// Package tlvkeep 是一个只用标准库实现的 TLV 编解码库。
//
// 线格式与语义逐条写在 README 的「对外契约」一节里。本文件是既有实现：
// 只认识自己写出去的几个已知字段。
package tlvkeep

import (
	"errors"
	"sort"
)

// 本包所有失败路径都返回下面四个哨兵错误之一（或其包装）。
// 调用方用 errors.Is 判定，不要比较错误字符串。
var (
	// ErrOrder 表示 Tag 顺序非法（回退，或非 Tag 4 的重复）。
	ErrOrder = errors.New("tlvkeep: tag order violation")

	// ErrMissing 表示顶层消息缺少必填字段（Name）。
	ErrMissing = errors.New("tlvkeep: required field missing")

	// ErrDepth 表示 TLV 嵌套深度超过上限。
	ErrDepth = errors.New("tlvkeep: nesting too deep")

	// ErrTruncated 表示声明的长度超过了实际剩下的字节。
	ErrTruncated = errors.New("tlvkeep: truncated input")
)

// maxDepth 是 TLV 嵌套深度的上限：顶层消息记为深度 1。
const maxDepth = 4

// Message 是一条 TLV 消息。
//
// 已知 Tag：
//
//	1: Name   (string)
//	2: Age    (varint)
//	3: Email  (string)
//	4: Tags   ([]string，同一个 Tag 4 可以出现多段)
//	5: Nested (其 Value 本身是一段完整的 TLV 字节流)
type Message struct {
	Name    string
	Age     uint64
	Email   string
	Tags    []string
	Nested  *Message
	Unknown [][]byte
}

// Decode 把一段 TLV 字节流解析成一条 Message。
func Decode(b []byte) (*Message, error) {
	return decode(b, 1, true)
}

// Encode 把一条 Message 编成 TLV 字节流。
func Encode(m *Message) ([]byte, error) {
	return encode(m, 1)
}

func encode(m *Message, depth int) ([]byte, error) {
	out := []byte{}
	if m == nil {
		return out, nil
	}

	type segment struct {
		tag uint64
		raw []byte
	}

	segments := make([]segment, 0, 4+len(m.Tags)+len(m.Unknown))
	add := func(tag uint64, raw []byte) {
		segments = append(segments, segment{tag: tag, raw: raw})
	}

	if m.Name != "" {
		add(1, makeSeg(1, []byte(m.Name)))
	}
	if m.Age != 0 {
		add(2, makeSeg(2, putVarint(m.Age)))
	}
	if m.Email != "" {
		add(3, makeSeg(3, []byte(m.Email)))
	}
	for _, t := range m.Tags {
		add(4, makeSeg(4, []byte(t)))
	}
	if m.Nested != nil {
		if depth >= maxDepth {
			return nil, ErrDepth
		}
		nested, err := encode(m.Nested, depth+1)
		if err != nil {
			return nil, err
		}
		add(5, makeSeg(5, nested))
	}
	for _, raw := range m.Unknown {
		tag, _, ok := readVarint(raw)
		if !ok {
			return nil, ErrTruncated
		}
		add(tag, raw)
	}

	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].tag < segments[j].tag
	})

	for _, segment := range segments {
		out = append(out, segment.raw...)
	}
	return out, nil
}

// ---------------------------------------------------------------- 内部

// decode 解析一段 TLV 字节流。depth 是当前深度（顶层为 1）；
// requireName 表示本层是否要求 Name 必填。
func decode(b []byte, depth int, requireName bool) (*Message, error) {
	m := &Message{}
	pos := 0
	var prev uint64
	hasPrev := false

	for pos < len(b) {
		segStart := pos

		tag, n, ok := readVarint(b[pos:])
		if !ok {
			return nil, ErrTruncated
		}
		pos += n

		length, n2, ok := readVarint(b[pos:])
		if !ok {
			return nil, ErrTruncated
		}
		pos += n2

		if length > uint64(len(b)-pos) {
			return nil, ErrTruncated
		}
		val := b[pos : pos+int(length)]
		pos += int(length)

		if hasPrev {
			if tag < prev {
				return nil, ErrOrder
			}
			if tag == prev && tag != 4 {
				return nil, ErrOrder
			}
		}
		prev, hasPrev = tag, true

		switch tag {
		case 1:
			m.Name = string(val)
		case 2:
			v, _, ok := readVarint(val)
			if !ok {
				return nil, ErrTruncated
			}
			m.Age = v
		case 3:
			m.Email = string(val)
		case 4:
			m.Tags = append(m.Tags, string(val))
		case 5:
			if depth >= maxDepth {
				return nil, ErrDepth
			}
			nested, err := decode(val, depth+1, false)
			if err != nil {
				return nil, err
			}
			m.Nested = nested
		default:
			m.Unknown = append(m.Unknown, append([]byte(nil), b[segStart:pos]...))
		}
	}

	if requireName && m.Name == "" {
		return nil, ErrMissing
	}
	return m, nil
}

// makeSeg 编出一段 Tag|Length|Value。
func makeSeg(tag uint64, val []byte) []byte {
	out := putVarint(tag)
	out = append(out, putVarint(uint64(len(val)))...)
	return append(out, val...)
}

// putVarint 用 LEB128 编出一个无符号整数。
func putVarint(x uint64) []byte {
	var out []byte
	for {
		c := byte(x & 0x7f)
		x >>= 7
		if x != 0 {
			c |= 0x80
		}
		out = append(out, c)
		if x == 0 {
			return out
		}
	}
}

// readVarint 读一个 LEB128 无符号整数，返回 (值, 消耗字节数, 是否成功)。
func readVarint(b []byte) (uint64, int, bool) {
	var x uint64
	var shift uint
	for i := 0; i < len(b); i++ {
		c := b[i]
		if shift >= 64 {
			return 0, 0, false
		}
		x |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return x, i + 1, true
		}
		shift += 7
	}
	return 0, 0, false
}
