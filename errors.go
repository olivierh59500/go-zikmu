package zikmu

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidArgument ErrorCode = "invalid_argument"
	CodeUnsupported     ErrorCode = "unsupported_format"
	CodeReadFailure     ErrorCode = "read_failure"
	CodeNotImplemented  ErrorCode = "not_implemented"
)

type Error struct {
	Code   ErrorCode
	Op     string
	Format Format
	Offset int64
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	msg := "zikmu"
	if e.Op != "" {
		msg += ": " + e.Op
	}
	if e.Code != "" {
		msg += ": " + string(e.Code)
	}
	if e.Format != FormatUnknown {
		msg += fmt.Sprintf(" (%s)", e.Format)
	}
	if e.Offset >= 0 {
		msg += fmt.Sprintf(" at offset %d", e.Offset)
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Code != "" && e.Code != t.Code {
		return false
	}
	if t.Op != "" && e.Op != t.Op {
		return false
	}
	if t.Format != FormatUnknown && e.Format != t.Format {
		return false
	}
	return true
}

var (
	ErrNilModule         = errors.New("zikmu: nil module")
	ErrUnsupportedFormat = &Error{Code: CodeUnsupported}
	ErrNotImplemented    = &Error{Code: CodeNotImplemented}
)

func errorf(code ErrorCode, op string, format Format, offset int64, err error) error {
	return &Error{
		Code:   code,
		Op:     op,
		Format: format,
		Offset: offset,
		Err:    err,
	}
}
