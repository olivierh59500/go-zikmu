package binary

import (
	"bytes"
	stdbinary "encoding/binary"
	"fmt"
	"io"
)

type Section struct {
	Offset int64
	Size   int64
}

func (s Section) End() int64 {
	return s.Offset + s.Size
}

type Reader struct {
	r    io.ReaderAt
	base int64
	size int64
	off  int64
}

func ReadAllAt(r io.ReaderAt, size int64) ([]byte, error) {
	if r == nil {
		return nil, fmt.Errorf("binary: nil reader")
	}
	if size < 0 {
		return nil, fmt.Errorf("binary: negative size %d", size)
	}
	if uint64(size) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("binary: size does not fit in int: %d", size)
	}

	buf := make([]byte, int(size))
	offset := 0
	for offset < len(buf) {
		n, err := r.ReadAt(buf[offset:], int64(offset))
		if n < 0 || n > len(buf)-offset {
			return nil, fmt.Errorf("binary: invalid read count %d", n)
		}
		offset += n
		if offset == len(buf) {
			return buf, nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
	return buf, nil
}

func NewReader(r io.ReaderAt, size int64) *Reader {
	return &Reader{
		r:    r,
		size: size,
	}
}

func NewBuffer(buf []byte) *Reader {
	return NewReader(bytes.NewReader(buf), int64(len(buf)))
}

func (r *Reader) Size() int64 {
	return r.size
}

func (r *Reader) Offset() int64 {
	return r.off
}

func (r *Reader) Remaining() int64 {
	return r.size - r.off
}

func (r *Reader) Seek(offset int64, whence int) (int64, error) {
	var next int64

	switch whence {
	case io.SeekStart:
		next = offset
	case io.SeekCurrent:
		next = r.off + offset
	case io.SeekEnd:
		next = r.size + offset
	default:
		return r.off, fmt.Errorf("binary: invalid whence %d", whence)
	}

	if next < 0 || next > r.size {
		return r.off, fmt.Errorf("binary: seek out of range: %d", next)
	}

	r.off = next
	return r.off, nil
}

func (r *Reader) Skip(size int64) error {
	_, err := r.Seek(size, io.SeekCurrent)
	return err
}

func (r *Reader) Section(offset, size int64) (*Reader, error) {
	if err := r.checkRange(offset, size); err != nil {
		return nil, err
	}

	return &Reader{
		r:    r.r,
		base: r.base + offset,
		size: size,
	}, nil
}

func (r *Reader) Read(dst []byte) (int, error) {
	if len(dst) == 0 {
		return 0, nil
	}
	if r.off >= r.size {
		return 0, io.EOF
	}

	available := r.size - r.off
	if int64(len(dst)) > available {
		dst = dst[:available]
	}

	n, err := r.r.ReadAt(dst, r.base+r.off)
	r.off += int64(n)
	if err != nil && err != io.EOF {
		return n, err
	}
	if n < len(dst) {
		return n, io.EOF
	}
	if int64(n) < available {
		return n, nil
	}
	return n, nil
}

func (r *Reader) ReadFull(dst []byte) error {
	n, err := r.Read(dst)
	if err == nil && n == len(dst) {
		return nil
	}
	if err == io.EOF || n != len(dst) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func (r *Reader) ReadAt(offset int64, dst []byte) error {
	if err := r.checkRange(offset, int64(len(dst))); err != nil {
		return err
	}

	n, err := r.r.ReadAt(dst, r.base+offset)
	if err != nil && err != io.EOF {
		return err
	}
	if n != len(dst) {
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (r *Reader) Bytes(size int64) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("binary: negative size %d", size)
	}

	buf := make([]byte, size)
	if err := r.ReadFull(buf); err != nil {
		return nil, err
	}

	return buf, nil
}

func (r *Reader) Uint8() (uint8, error) {
	var buf [1]byte
	if err := r.ReadFull(buf[:]); err != nil {
		return 0, err
	}
	return buf[0], nil
}

func (r *Reader) Int8() (int8, error) {
	v, err := r.Uint8()
	return int8(v), err
}

func (r *Reader) Uint16LE() (uint16, error) {
	return r.readUint16(stdbinary.LittleEndian)
}

func (r *Reader) Uint16BE() (uint16, error) {
	return r.readUint16(stdbinary.BigEndian)
}

func (r *Reader) Uint32LE() (uint32, error) {
	return r.readUint32(stdbinary.LittleEndian)
}

func (r *Reader) Uint32BE() (uint32, error) {
	return r.readUint32(stdbinary.BigEndian)
}

func (r *Reader) String(size int) (string, error) {
	buf, err := r.Bytes(int64(size))
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func (r *Reader) readUint16(order stdbinary.ByteOrder) (uint16, error) {
	var buf [2]byte
	if err := r.ReadFull(buf[:]); err != nil {
		return 0, err
	}
	return order.Uint16(buf[:]), nil
}

func (r *Reader) readUint32(order stdbinary.ByteOrder) (uint32, error) {
	var buf [4]byte
	if err := r.ReadFull(buf[:]); err != nil {
		return 0, err
	}
	return order.Uint32(buf[:]), nil
}

func (r *Reader) checkRange(offset, size int64) error {
	if offset < 0 {
		return fmt.Errorf("binary: negative offset %d", offset)
	}
	if size < 0 {
		return fmt.Errorf("binary: negative size %d", size)
	}
	if offset > r.size {
		return fmt.Errorf("binary: offset out of range: %d > %d", offset, r.size)
	}
	if size > r.size-offset {
		return fmt.Errorf("binary: section out of range: offset=%d size=%d limit=%d", offset, size, r.size)
	}
	return nil
}
