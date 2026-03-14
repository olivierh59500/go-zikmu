package zikmu_test

import (
	"testing"

	"github.com/olivierh59500/go-zikmu/internal/validation"
)

func TestRenderRegressionCorpus(t *testing.T) {
	expected := map[string]struct {
		hash      string
		nonSilent bool
	}{
		"mod-minimal": {hash: "7fa51b44bf37b0e973f007528b7b068250c1b711b4418dc8223572c4967ad89d", nonSilent: true},
		"mod-adpcm":   {hash: "ea29a7654d9fd9a7dff3d5b1088d201c4e4f7ec755bc5ee4b01553208ab253b4", nonSilent: true},
		"mod-flt8":    {hash: "3457ab0c8df688c8d560d5e3ddec69532e046b452885aae26ad1bb6a41403ebb", nonSilent: true},
		"s3m-minimal": {hash: "7563684d90322c504ad0c38c575c38a8c4d3e82fcad4804c5fe59bc8f787931e", nonSilent: true},
		"s3m-adpcm":   {hash: "cddddd0f5a58b4f0fb867983d407686eaacb6fd8ccb17e398094d4f059a59366", nonSilent: true},
		"xm-minimal":  {hash: "f4d44b4e8db86193e420bcedfd365eb8a84efd34f37c901eaf75b701b0f0f0ac", nonSilent: true},
		"xm-adpcm":    {hash: "4fe7b59af6de3b665b67788cc2f99892ab827efae3a467342b3bb4e3bc8e5bfe", nonSilent: false},
		"xm-103":      {hash: "4fe7b59af6de3b665b67788cc2f99892ab827efae3a467342b3bb4e3bc8e5bfe", nonSilent: false},
		"it-minimal":  {hash: "4fe7b59af6de3b665b67788cc2f99892ab827efae3a467342b3bb4e3bc8e5bfe", nonSilent: false},
		"it-packed":   {hash: "4fe7b59af6de3b665b67788cc2f99892ab827efae3a467342b3bb4e3bc8e5bfe", nonSilent: false},
	}

	for _, entry := range validation.RegressionCorpus() {
		entry := entry
		t.Run(entry.Name, func(t *testing.T) {
			result, err := validation.RenderPCM16(entry.Data, validation.DefaultConfig(), validation.DefaultFrames)
			if err != nil {
				t.Fatalf("RenderPCM16 failed: %v", err)
			}
			if result.Frames != validation.DefaultFrames {
				t.Fatalf("unexpected frame count: got=%d want=%d", result.Frames, validation.DefaultFrames)
			}
			want := expected[entry.Name]
			if want.nonSilent && (result.NonZeroSamples == 0 || result.Peak == 0) {
				t.Fatalf("expected non-silent audio, got peak=%d nonzero=%d", result.Peak, result.NonZeroSamples)
			}
			if !want.nonSilent && (result.NonZeroSamples != 0 || result.Peak != 0) {
				t.Fatalf("expected silent audio, got peak=%d nonzero=%d", result.Peak, result.NonZeroSamples)
			}
			if got := result.SHA256; got != want.hash {
				t.Fatalf("unexpected regression hash: got=%s want=%s", got, want.hash)
			}
		})
	}
}
