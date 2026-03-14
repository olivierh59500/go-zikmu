package formatdetect

import (
	"bytes"
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/testfixtures"
)

func TestDetectKnownFormats(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want Kind
	}{
		{name: "mod mk", data: testfixtures.MOD("M.K."), want: MOD},
		{name: "mod 6chn", data: testfixtures.MOD("6CHN"), want: MOD},
		{name: "mod flt8", data: testfixtures.MOD("FLT8"), want: MOD},
		{name: "s3m", data: testfixtures.S3M(), want: S3M},
		{name: "xm", data: testfixtures.XM(), want: XM},
		{name: "it", data: testfixtures.IT(), want: IT},
		{name: "unknown", data: testfixtures.Unknown(), want: Unknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Detect(bytes.NewReader(tt.data), int64(len(tt.data)))
			if err != nil {
				t.Fatalf("Detect failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("unexpected kind: got=%q want=%q", got, tt.want)
			}
		})
	}
}

func TestDetectRejectsInvalidSize(t *testing.T) {
	_, err := Detect(bytes.NewReader(nil), 0)
	if err == nil {
		t.Fatal("expected an error for invalid size")
	}
}
