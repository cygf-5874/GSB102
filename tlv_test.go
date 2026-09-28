package tlvkeep

import (
	"errors"
	"reflect"
	"testing"
)

func TestEncodeDecodeName(t *testing.T) {
	b, err := Encode(&Message{Name: "alice"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Name != "alice" {
		t.Fatalf("Name=%q, want alice", m.Name)
	}
}

func TestEncodeDecodeNameAge(t *testing.T) {
	b, err := Encode(&Message{Name: "bob", Age: 42})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Name != "bob" || m.Age != 42 {
		t.Fatalf("Name=%q Age=%d, want bob/42", m.Name, m.Age)
	}
}

func TestAgeMultiByteVarint(t *testing.T) {
	b, err := Encode(&Message{Name: "carol", Age: 300})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Age != 300 {
		t.Fatalf("Age=%d, want 300", m.Age)
	}
}

func TestEncodeDecodeEmail(t *testing.T) {
	b, err := Encode(&Message{Name: "dave", Email: "d@example.com"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Email != "d@example.com" {
		t.Fatalf("Email=%q, want d@example.com", m.Email)
	}
}

func TestTagsRepeatedSegments(t *testing.T) {
	b, err := Encode(&Message{Name: "erin", Tags: []string{"go", "rust"}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(m.Tags, []string{"go", "rust"}) {
		t.Fatalf("Tags=%v, want [go rust]", m.Tags)
	}
}

func TestTagsEmpty(t *testing.T) {
	b, err := Encode(&Message{Name: "frank"})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(m.Tags) != 0 {
		t.Fatalf("Tags=%v, want empty", m.Tags)
	}
}

func TestTagsDuplicateValues(t *testing.T) {
	b, err := Encode(&Message{Name: "gina", Tags: []string{"x", "x"}})
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	m, err := Decode(b)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !reflect.DeepEqual(m.Tags, []string{"x", "x"}) {
		t.Fatalf("Tags=%v, want [x x]", m.Tags)
	}
}

func TestMissingOptionalFieldsZero(t *testing.T) {
	m, err := Decode(makeSeg(1, []byte("hank")))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if m.Age != 0 || m.Email != "" || len(m.Tags) != 0 {
		t.Fatalf("缺失字段应取零值，得到 Age=%d Email=%q Tags=%v", m.Age, m.Email, m.Tags)
	}
}

func TestMissingNameReturnsErrMissing(t *testing.T) {
	_, err := Decode(makeSeg(2, putVarint(7)))
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("err=%v, want ErrMissing", err)
	}
}

func TestEmptyInputReturnsErrMissing(t *testing.T) {
	_, err := Decode(nil)
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("err=%v, want ErrMissing", err)
	}
}

func TestOutOfOrderReturnsErrOrder(t *testing.T) {
	b := append(makeSeg(3, []byte("e")), makeSeg(2, putVarint(1))...)
	_, err := Decode(b)
	if !errors.Is(err, ErrOrder) {
		t.Fatalf("err=%v, want ErrOrder", err)
	}
}

func TestDuplicateKnownTagReturnsErrOrder(t *testing.T) {
	b := append(makeSeg(1, []byte("a")), makeSeg(1, []byte("b"))...)
	_, err := Decode(b)
	if !errors.Is(err, ErrOrder) {
		t.Fatalf("err=%v, want ErrOrder", err)
	}
}

func TestTruncatedReturnsErrTruncated(t *testing.T) {
	b := putVarint(1)
	b = append(b, putVarint(5)...) // 声明 5 字节
	b = append(b, []byte("ab")...) // 实际只有 2 字节
	_, err := Decode(b)
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("err=%v, want ErrTruncated", err)
	}
}
