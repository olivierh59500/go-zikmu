package zikmu

import (
	"fmt"
	"io"

	"github.com/olivierh59500/go-zikmu/internal/formatdetect"
)

func Detect(r io.ReaderAt, size int64) (Format, error) {
	if r == nil {
		return FormatUnknown, errorf(CodeInvalidArgument, "Detect", FormatUnknown, -1, fmt.Errorf("nil reader"))
	}
	if size <= 0 {
		return FormatUnknown, errorf(CodeInvalidArgument, "Detect", FormatUnknown, -1, fmt.Errorf("invalid size %d", size))
	}

	kind, err := formatdetect.Detect(r, size)
	if err != nil {
		return FormatUnknown, errorf(CodeReadFailure, "Detect", FormatUnknown, -1, err)
	}

	return mapFormat(kind), nil
}

func mapFormat(kind formatdetect.Kind) Format {
	switch kind {
	case formatdetect.MOD:
		return FormatMOD
	case formatdetect.S3M:
		return FormatS3M
	case formatdetect.XM:
		return FormatXM
	case formatdetect.IT:
		return FormatIT
	default:
		return FormatUnknown
	}
}
