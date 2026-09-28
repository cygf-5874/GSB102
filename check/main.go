// Command check 是 tlvkeep 的固定验收程序。
//
// ⚠️ 不要修改本文件。它是判定「题目有没有做对」的依据；
// 修改它只会让判定失效，不会让实现变对。
//
// 9 个场景分四组：
//
//	known  2：已知字段解码 / 缺失取零值且 Name 必填
//	keep   3：未知字段按序插回 / 非最短 varint 原样保留 / 逐字节往返
//	nested 2：嵌套内未知字段透传 / 深度 5 返回 ErrDepth
//	edge   2：全未知字段输入 / 截断输入返回 ErrTruncated
//
// 每个场景都在本进程里跑，panic 会被逐个捕获并记成 FAIL —— 固定件必须把
// 9 个场景全部跑完再收尾。判据全部是硬编码字节串，不依赖墙钟与 map 迭代顺序。
//
// 用法：
//
//	go run ./check                  # 跑全部场景
//	go run ./check --only keep      # 只跑一组
//	go run ./check -list            # 列出场景
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"tlvkeep"
)

type scenario struct {
	Group string
	Name  string
	Run   func() error
}

var scenarios = []scenario{
	{"known", "KN1_known_decode", kn1},
	{"known", "KN2_missing_zero_and_name_required", kn2},
	{"keep", "K1_unknown_reinserted_in_order", k1},
	{"keep", "K2_nonminimal_varint_preserved", k2},
	{"keep", "K3_byte_roundtrip", k3},
	{"nested", "N1_nested_unknown_passthrough", n1},
	{"nested", "N2_depth_exceeded", n2},
	{"edge", "ED1_all_unknown_input", ed1},
	{"edge", "ED2_truncated_input", ed2},
}

var groups = []string{"known", "keep", "nested", "edge"}

func main() {
	only := flag.String("only", "", "只跑指定组：known / keep / nested / edge")
	list := flag.Bool("list", false, "列出全部场景")
	flag.Parse()

	if *list {
		for _, sc := range scenarios {
			fmt.Printf("%-7s %s\n", sc.Group, sc.Name)
		}
		return
	}

	selected := groups
	if *only != "" {
		found := false
		for _, g := range groups {
			if g == *only {
				found = true
			}
		}
		if !found {
			fmt.Printf("未知分组 %q（可选：known / keep / nested / edge）\n", *only)
			os.Exit(2)
		}
		selected = []string{*only}
	}

	total, passed := 0, 0
	for _, g := range selected {
		fmt.Printf("== 组 %s ==\n", g)
		for _, sc := range scenarios {
			if sc.Group != g {
				continue
			}
			total++
			if err := runScenario(sc); err != nil {
				fmt.Printf("FAIL %s/%s  %s\n", sc.Group, sc.Name, err.Error())
				continue
			}
			passed++
			fmt.Printf("PASS %s/%s\n", sc.Group, sc.Name)
		}
	}

	fmt.Println()
	fmt.Printf("结果：通过 %d/%d\n", passed, total)
	if passed != total {
		os.Exit(1)
	}
}

func runScenario(sc scenario) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = expect("场景正常结束（不 panic）", fmt.Sprintf("panic: %v", r))
		}
	}()
	return sc.Run()
}

// ---------------------------------------------------------------- 失败描述

type detailError struct{ msg string }

func (e *detailError) Error() string { return e.msg }

func expect(want, got string) error {
	return &detailError{msg: "期望=" + want + " 实际=" + got}
}

func errStr(err error) string {
	if err == nil {
		return "nil（没有返回错误）"
	}
	return err.Error()
}

// ------------------------------------------------------- 固定件自己的编码器

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

// seg 编一段规范（最短 varint）的 Tag|Length|Value。
func seg(tag uint64, val []byte) []byte {
	out := putVarint(tag)
	out = append(out, putVarint(uint64(len(val)))...)
	return append(out, val...)
}

// segRawLen 用给定的字节作为 Length 字段，其余与 seg 一致（可用非最短 varint）。
func segRawLen(tag uint64, val []byte, lenBytes []byte) []byte {
	out := putVarint(tag)
	out = append(out, lenBytes...)
	return append(out, val...)
}

func cat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func eqStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func eqRawList(a [][]byte, b ...[]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

func fmtVals(v []byte) string { return fmt.Sprintf("% x", v) }

// -------------------------------------------------------------------- known

func kn1() error {
	b := cat(
		seg(1, []byte("alice")),
		seg(2, putVarint(30)),
		seg(3, []byte("a@b")),
		seg(4, []byte("x")),
		seg(4, []byte("y")),
	)
	m, err := tlvkeep.Decode(b)
	if err != nil {
		return expect("已知字段解码成功", errStr(err))
	}
	if m.Name != "alice" {
		return expect("Name=alice", fmt.Sprintf("%q", m.Name))
	}
	if m.Age != 30 {
		return expect("Age=30", fmt.Sprintf("%d", m.Age))
	}
	if m.Email != "a@b" {
		return expect("Email=a@b", fmt.Sprintf("%q", m.Email))
	}
	if !eqStrings(m.Tags, []string{"x", "y"}) {
		return expect("Tags=[x y]", fmt.Sprintf("%v", m.Tags))
	}
	return nil
}

func kn2() error {
	m, err := tlvkeep.Decode(seg(1, []byte("bob")))
	if err != nil {
		return expect("只带 Name 的消息解码成功", errStr(err))
	}
	if m.Age != 0 || m.Email != "" || len(m.Tags) != 0 {
		return expect("缺失可选字段取零值",
			fmt.Sprintf("Age=%d Email=%q Tags=%v", m.Age, m.Email, m.Tags))
	}
	_, err = tlvkeep.Decode(cat(seg(2, putVarint(1)), seg(3, []byte("z"))))
	if !errors.Is(err, tlvkeep.ErrMissing) {
		return expect("顶层缺 Name 返回 ErrMissing", errStr(err))
	}
	return nil
}

// --------------------------------------------------------------------- keep

func k1() error {
	raw6 := seg(6, []byte{0xaa, 0xbb})
	raw9 := seg(9, []byte("deep"))
	b := cat(
		seg(1, []byte("carol")),
		seg(2, putVarint(7)),
		raw6,
		raw9,
	)
	m, err := tlvkeep.Decode(b)
	if err != nil {
		return expect("带未知字段的消息解码成功", errStr(err))
	}
	if !eqRawList(m.Unknown, raw6, raw9) {
		return expect("Unknown 按 Tag 升序保留两段原始字节",
			fmt.Sprintf("%d 段：%s", len(m.Unknown), dumpRaw(m.Unknown)))
	}
	enc, err := tlvkeep.Encode(m)
	if err != nil {
		return expect("Encode 成功", errStr(err))
	}
	if !bytes.Equal(enc, b) {
		return expect("Encode(Decode(b)) 逐字节等于 b（原始 "+fmtVals(b)+"）", fmtVals(enc))
	}
	return nil
}

func k2() error {
	// Tag 7 的 Length 用非最短 varint 0x83 0x00 表示 3。
	raw := segRawLen(7, []byte("abc"), []byte{0x83, 0x00})
	b := cat(seg(1, []byte("dave")), raw)
	m, err := tlvkeep.Decode(b)
	if err != nil {
		return expect("非最短 Length 的未知段解码成功", errStr(err))
	}
	if !eqRawList(m.Unknown, raw) {
		return expect("未知段原样保留（含非最短 Length）",
			fmt.Sprintf("%d 段：%s", len(m.Unknown), dumpRaw(m.Unknown)))
	}
	enc, err := tlvkeep.Encode(m)
	if err != nil {
		return expect("Encode 成功", errStr(err))
	}
	if !bytes.Equal(enc, b) {
		return expect("Encode(Decode(b)) 逐字节等于 b（原始 "+fmtVals(b)+"）", fmtVals(enc))
	}
	return nil
}

func k3() error {
	nested := cat(seg(1, []byte("nest")), seg(8, []byte("u8")))
	b := cat(
		seg(1, []byte("erin")),
		seg(2, putVarint(300)),
		seg(3, []byte("e@x")),
		seg(4, []byte("p")),
		seg(5, nested),
		seg(6, []byte{0x01}),
		seg(7, []byte("q")),
	)
	m, err := tlvkeep.Decode(b)
	if err != nil {
		return expect("混合消息解码成功", errStr(err))
	}
	enc, err := tlvkeep.Encode(m)
	if err != nil {
		return expect("Encode 成功", errStr(err))
	}
	if !bytes.Equal(enc, b) {
		return expect("Encode(Decode(b)) 逐字节等于 b（原始 "+fmtVals(b)+"）", fmtVals(enc))
	}
	return nil
}

// ------------------------------------------------------------------- nested

func n1() error {
	innerRaw := seg(9, []byte("innerunknown"))
	inner := cat(seg(2, putVarint(9)), innerRaw)
	b := cat(seg(1, []byte("frank")), seg(5, inner))

	m, err := tlvkeep.Decode(b)
	if err != nil {
		return expect("嵌套消息解码成功", errStr(err))
	}
	if m.Nested == nil {
		return expect("Tag 5 解成 Nested", "Nested=nil")
	}
	if m.Nested.Age != 9 {
		return expect("Nested.Age=9", fmt.Sprintf("%d", m.Nested.Age))
	}
	if !eqRawList(m.Nested.Unknown, innerRaw) {
		return expect("嵌套内的未知字段也被透传",
			fmt.Sprintf("%d 段：%s", len(m.Nested.Unknown), dumpRaw(m.Nested.Unknown)))
	}
	enc, err := tlvkeep.Encode(m)
	if err != nil {
		return expect("Encode 成功", errStr(err))
	}
	if !bytes.Equal(enc, b) {
		return expect("Encode(Decode(b)) 逐字节等于 b（原始 "+fmtVals(b)+"）", fmtVals(enc))
	}
	return nil
}

func n2() error {
	// 5 层嵌套：顶层深度 1，直到第 5 层超过上限 4。
	l5 := seg(1, []byte("l5"))
	l4 := cat(seg(1, []byte("l4")), seg(5, l5))
	l3 := cat(seg(1, []byte("l3")), seg(5, l4))
	l2 := cat(seg(1, []byte("l2")), seg(5, l3))
	top := cat(seg(1, []byte("l1")), seg(5, l2))

	_, err := tlvkeep.Decode(top)
	if !errors.Is(err, tlvkeep.ErrDepth) {
		return expect("深度 5 返回 ErrDepth", errStr(err))
	}
	// 4 层嵌套必须仍然成功。
	d4 := seg(1, []byte("d4"))
	d3 := cat(seg(1, []byte("d3")), seg(5, d4))
	d2 := cat(seg(1, []byte("d2")), seg(5, d3))
	d1 := cat(seg(1, []byte("d1")), seg(5, d2)) // 深度 1→2→3→4
	if _, err := tlvkeep.Decode(d1); err != nil {
		return expect("深度 4 的嵌套解码成功", errStr(err))
	}
	return nil
}

// --------------------------------------------------------------------- edge

func ed1() error {
	b := cat(seg(6, []byte{}), seg(7, []byte("x")))
	_, err := tlvkeep.Decode(b)
	if !errors.Is(err, tlvkeep.ErrMissing) {
		return expect("全未知字段输入返回 ErrMissing（没有 Name）", errStr(err))
	}
	return nil
}

func ed2() error {
	b := putVarint(1)
	b = append(b, putVarint(10)...) // 声明 10 字节
	b = append(b, []byte("ab")...)  // 实际只有 2 字节
	_, err := tlvkeep.Decode(b)
	if !errors.Is(err, tlvkeep.ErrTruncated) {
		return expect("截断输入返回 ErrTruncated", errStr(err))
	}
	return nil
}

func dumpRaw(list [][]byte) string {
	parts := make([]string, len(list))
	for i, r := range list {
		parts[i] = "[" + fmtVals(r) + "]"
	}
	return strings.Join(parts, " ")
}
