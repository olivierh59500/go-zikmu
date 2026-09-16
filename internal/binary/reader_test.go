package binary

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestReadAllAt(t *testing.T) {
	want := []byte{1, 2, 3, 4}
	got, err := ReadAllAt(bytes.NewReader(want), int64(len(want)))
	if err != nil {
		t.Fatalf("ReadAllAt failed: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected data: got=%v want=%v", got, want)
	}

	if _, err := ReadAllAt(bytes.NewReader(want), int64(len(want)+1)); !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF for a short reader, got %v", err)
	}
}

func TestReaderEndianAndOffsets(t *testing.T) {
	r := NewBuffer([]byte{
		0x01,
		0x02, 0x03,
		0x04, 0x05,
		'a', 'b', 'c',
	})

	v8, err := r.Uint8()
	if err != nil {
		t.Fatalf("Uint8 failed: %v", err)
	}
	if v8 != 0x01 {
		t.Fatalf("unexpected Uint8 value: %x", v8)
	}

	v16le, err := r.Uint16LE()
	if err != nil {
		t.Fatalf("Uint16LE failed: %v", err)
	}
	if v16le != 0x0302 {
		t.Fatalf("unexpected Uint16LE value: %x", v16le)
	}

	v16be, err := r.Uint16BE()
	if err != nil {
		t.Fatalf("Uint16BE failed: %v", err)
	}
	if v16be != 0x0405 {
		t.Fatalf("unexpected Uint16BE value: %x", v16be)
	}

	s, err := r.String(3)
	if err != nil {
		t.Fatalf("String failed: %v", err)
	}
	if s != "abc" {
		t.Fatalf("unexpected string: %q", s)
	}

	if r.Offset() != r.Size() {
		t.Fatalf("unexpected offset after reads: got=%d size=%d", r.Offset(), r.Size())
	}
}

func TestReaderSectionAndReadAt(t *testing.T) {
	r := NewBuffer([]byte{0, 1, 2, 3, 4, 5, 6, 7})

	section, err := r.Section(2, 4)
	if err != nil {
		t.Fatalf("Section failed: %v", err)
	}

	buf := make([]byte, 2)
	if err := section.ReadAt(1, buf); err != nil {
		t.Fatalf("ReadAt failed: %v", err)
	}
	if buf[0] != 3 || buf[1] != 4 {
		t.Fatalf("unexpected read bytes: %v", buf)
	}

	if err := section.Skip(2); err != nil {
		t.Fatalf("Skip failed: %v", err)
	}

	v, err := section.Uint8()
	if err != nil {
		t.Fatalf("Uint8 in section failed: %v", err)
	}
	if v != 4 {
		t.Fatalf("unexpected section value: %d", v)
	}
}

func TestReaderUnexpectedEOF(t *testing.T) {
	r := NewBuffer([]byte{0x01})

	_, err := r.Uint16LE()
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

func TestReaderRejectsOutOfRangeSection(t *testing.T) {
	r := NewBuffer([]byte{0x01, 0x02})

	_, err := r.Section(1, 2)
	if err == nil {
		t.Fatal("expected out-of-range error")
	}
}
