package formatdetect

import (
	"bytes"
	"fmt"
	"io"

	"github.com/olivierh59500/go-zikmu/internal/binary"
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

var modSignatures = map[string]struct{}{
	"M.K.": {},
	"M!K!": {},
	"M&K!": {},
	"OKTA": {},
	"CD81": {},
	"CD61": {},
	"LARD": {},
	"NSMS": {},
}

func Detect(r io.ReaderAt, size int64) (Kind, error) {
	if r == nil {
		return Unknown, fmt.Errorf("formatdetect: nil reader")
	}
	if size <= 0 {
		return Unknown, fmt.Errorf("formatdetect: invalid size %d", size)
	}

	reader := binary.NewReader(r, size)

	match, err := matchIT(reader)
	if err != nil {
		return Unknown, err
	}
	if match {
		return IT, nil
	}

	match, err = matchXM(reader)
	if err != nil {
		return Unknown, err
	}
	if match {
		return XM, nil
	}

	match, err = matchS3M(reader)
	if err != nil {
		return Unknown, err
	}
	if match {
		return S3M, nil
	}

	match, err = matchMOD(reader)
	if err != nil {
		return Unknown, err
	}
	if match {
		return MOD, nil
	}

	return Unknown, nil
}

func matchIT(r *binary.Reader) (bool, error) {
	if r.Size() < 4 {
		return false, nil
	}

	var header [4]byte
	if err := r.ReadAt(0, header[:]); err != nil {
		return false, err
	}

	return bytes.Equal(header[:], []byte("IMPM")), nil
}

func matchXM(r *binary.Reader) (bool, error) {
	if r.Size() < 38 {
		return false, nil
	}

	var header [38]byte
	if err := r.ReadAt(0, header[:]); err != nil {
		return false, err
	}

	if !bytes.Equal(header[:17], []byte("Extended Module: ")) {
		return false, nil
	}

	return header[37] == 0x1a, nil
}

func matchS3M(r *binary.Reader) (bool, error) {
	if r.Size() < 0x30 {
		return false, nil
	}

	var header [4]byte
	if err := r.ReadAt(0x2c, header[:]); err != nil {
		return false, err
	}

	return bytes.Equal(header[:], []byte("SCRM")), nil
}

func matchMOD(r *binary.Reader) (bool, error) {
	if r.Size() < modSignatureOffset+4 {
		return false, nil
	}

	var id [4]byte
	if err := r.ReadAt(modSignatureOffset, id[:]); err != nil {
		return false, err
	}

	if _, ok := modSignatures[string(id[:])]; ok {
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
