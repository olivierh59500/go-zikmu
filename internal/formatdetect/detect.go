package formatdetect

import (
	"bytes"
	"fmt"
	"io"
)

type Kind string

const (
	Unknown Kind = ""
	MOD     Kind = "mod"
	S3M     Kind = "s3m"
	XM      Kind = "xm"
	IT      Kind = "it"
)

const modSignatureOffset = 1080

var modSignatures = map[[4]byte]struct{}{
	{'M', '.', 'K', '.'}: {},
	{'M', '!', 'K', '!'}: {},
	{'M', '&', 'K', '!'}: {},
	{'O', 'K', 'T', 'A'}: {},
	{'C', 'D', '8', '1'}: {},
	{'C', 'D', '6', '1'}: {},
	{'L', 'A', 'R', 'D'}: {},
	{'N', 'S', 'M', 'S'}: {},
}

func Detect(r io.ReaderAt, size int64) (Kind, error) {
	if r == nil {
		return Unknown, fmt.Errorf("formatdetect: nil reader")
	}
	if size <= 0 {
		return Unknown, fmt.Errorf("formatdetect: invalid size %d", size)
	}

	match, err := matchIT(r, size)
	if err != nil {
		return Unknown, err
	}
	if match {
		return IT, nil
	}

	match, err = matchXM(r, size)
	if err != nil {
		return Unknown, err
	}
	if match {
		return XM, nil
	}

	match, err = matchS3M(r, size)
	if err != nil {
		return Unknown, err
	}
	if match {
		return S3M, nil
	}

	match, err = matchMOD(r, size)
	if err != nil {
		return Unknown, err
	}
	if match {
		return MOD, nil
	}

	return Unknown, nil
}

func matchIT(r io.ReaderAt, size int64) (bool, error) {
	if size < 4 {
		return false, nil
	}

	var header [4]byte
	if err := readAt(r, 0, header[:]); err != nil {
		return false, err
	}

	return header == [4]byte{'I', 'M', 'P', 'M'}, nil
}

func matchXM(r io.ReaderAt, size int64) (bool, error) {
	if size < 38 {
		return false, nil
	}

	var header [38]byte
	if err := readAt(r, 0, header[:]); err != nil {
		return false, err
	}

	if !bytes.Equal(header[:17], []byte("Extended Module: ")) {
		return false, nil
	}

	return header[37] == 0x1a, nil
}

func matchS3M(r io.ReaderAt, size int64) (bool, error) {
	if size < 0x30 {
		return false, nil
	}

	var header [4]byte
	if err := readAt(r, 0x2c, header[:]); err != nil {
		return false, err
	}

	return header == [4]byte{'S', 'C', 'R', 'M'}, nil
}

func matchMOD(r io.ReaderAt, size int64) (bool, error) {
	if size < modSignatureOffset+4 {
		return false, nil
	}

	var id [4]byte
	if err := readAt(r, modSignatureOffset, id[:]); err != nil {
		return false, err
	}

	if _, ok := modSignatures[id]; ok {
		return true, nil
	}

	if id[0] >= '1' && id[0] <= '9' && bytes.Equal(id[1:], []byte("CHN")) {
		return true, nil
	}
	if id[0] >= '0' && id[0] <= '9' && id[1] >= '0' && id[1] <= '9' && bytes.Equal(id[2:4], []byte("CH")) {
		return true, nil
	}
	if id[0] >= '0' && id[0] <= '9' && id[1] >= '0' && id[1] <= '9' && bytes.Equal(id[2:4], []byte("CN")) {
		return true, nil
	}
	if bytes.Equal(id[:3], []byte("FLT")) && (id[3] == '4' || id[3] == '8') {
		return true, nil
	}
	if bytes.Equal(id[:3], []byte("EXO")) && (id[3] == '4' || id[3] == '8') {
		return true, nil
	}
	if bytes.Equal(id[:3], []byte("TDZ")) && id[3] >= '1' && id[3] <= '3' {
		return true, nil
	}
	if bytes.Equal(id[:3], []byte("FA0")) && (id[3] == '4' || id[3] == '6' || id[3] == '8') {
		return true, nil
	}

	return false, nil
}

func readAt(r io.ReaderAt, offset int64, dst []byte) error {
	n, err := r.ReadAt(dst, offset)
	if err != nil && err != io.EOF {
		return err
	}
	if n != len(dst) {
		return io.ErrUnexpectedEOF
	}
	return nil
}
