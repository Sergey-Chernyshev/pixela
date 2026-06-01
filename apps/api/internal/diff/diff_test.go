package diff

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

// solid returns a w×h NRGBA filled with c.
func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// withRect returns a copy of base with rect painted in c.
func withRect(base *image.NRGBA, rect image.Rectangle, c color.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(base.Bounds())
	copy(out.Pix, base.Pix)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

var (
	white = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	green = color.NRGBA{R: 0, G: 255, B: 0, A: 255}
)

func TestDiff(t *testing.T) {
	const w, h = 16, 16
	base := solid(w, h, white)

	// A 4×4 black block in the top-left corner.
	changedRect := image.Rect(0, 0, 4, 4)
	changed := withRect(base, changedRect, black)

	tests := []struct {
		name           string
		baseline       *image.NRGBA
		candidate      *image.NRGBA
		opts           Options
		wantStatus     core.SnapshotStatus
		wantDiffPixels int
		wantRatio      float64
		wantDiffImage  bool
	}{
		{
			name:           "identical images are unchanged",
			baseline:       base,
			candidate:      solid(w, h, white),
			opts:           DefaultOptions(),
			wantStatus:     core.SnapshotUnchanged,
			wantDiffPixels: 0,
			wantRatio:      0,
			// pixelmatch's identical fast-path returns before assigning WriteTo,
			// so an unchanged comparison yields no diff image.
			wantDiffImage: false,
		},
		{
			name:           "differing block is changed",
			baseline:       base,
			candidate:      changed,
			opts:           DefaultOptions(),
			wantStatus:     core.SnapshotChanged,
			wantDiffPixels: 16, // full 4×4 block, no AA on a hard edge interior
			wantRatio:      16.0 / float64(w*h),
			wantDiffImage:  true,
		},
		{
			name:           "ignore-rect masks the differing region",
			baseline:       base,
			candidate:      changed,
			opts:           withIgnore(DefaultOptions(), changedRect),
			wantStatus:     core.SnapshotUnchanged,
			wantDiffPixels: 0,
			wantRatio:      0,
			wantDiffImage:  false,
		},
	}

	eng := NewStdlibEngine()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := eng.Diff(tc.baseline, tc.candidate, tc.opts)
			if err != nil {
				t.Fatalf("Diff: %v", err)
			}
			if res.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", res.Status, tc.wantStatus)
			}
			if res.DiffPixels != tc.wantDiffPixels {
				t.Errorf("diffPixels = %d, want %d", res.DiffPixels, tc.wantDiffPixels)
			}
			if res.DiffRatio != tc.wantRatio {
				t.Errorf("diffRatio = %v, want %v", res.DiffRatio, tc.wantRatio)
			}
			if (res.DiffImage != nil) != tc.wantDiffImage {
				t.Errorf("diffImage present = %v, want %v", res.DiffImage != nil, tc.wantDiffImage)
			}
		})
	}
}

// withIgnore is a small test helper that returns a copy of opts with rect appended
// to IgnoreRects.
func withIgnore(opts Options, rect image.Rectangle) Options {
	opts.IgnoreRects = append([]image.Rectangle(nil), opts.IgnoreRects...)
	opts.IgnoreRects = append(opts.IgnoreRects, rect)
	return opts
}

func TestDiffSizeMismatch(t *testing.T) {
	eng := NewStdlibEngine()
	baseline := solid(8, 8, white)
	candidate := solid(10, 12, white)

	res, err := eng.Diff(baseline, candidate, DefaultOptions())
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if res.Status != core.SnapshotChanged {
		t.Errorf("status = %q, want CHANGED", res.Status)
	}
	if res.DiffRatio != 1.0 {
		t.Errorf("diffRatio = %v, want 1.0", res.DiffRatio)
	}
	if want := 10 * 12; res.DiffPixels != want {
		t.Errorf("diffPixels = %d, want %d", res.DiffPixels, want)
	}
	if res.DiffImage != nil {
		t.Errorf("diffImage = non-nil, want nil on size mismatch")
	}
}

func TestDecodeCanonicalNRGBA(t *testing.T) {
	src := solid(5, 7, green)
	eng := NewStdlibEngine()

	img, err := eng.Decode(bytes.NewReader(encodePNG(t, src)))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	n, ok := img.(*image.NRGBA)
	if !ok {
		t.Fatalf("Decode returned %T, want *image.NRGBA", img)
	}
	if n.Bounds() != image.Rect(0, 0, 5, 7) {
		t.Errorf("bounds = %v, want 0,0,5,7", n.Bounds())
	}
	if got := n.NRGBAAt(0, 0); got != green {
		t.Errorf("pixel = %v, want %v", got, green)
	}
}

// TestDeterminism is the golden-master check (rulebook §10.4 #7): a fixed PNG pair
// must yield stable diffPixels, diffRatio AND a stable diff-image ContentKey across
// repeated runs.
func TestDeterminism(t *testing.T) {
	const w, h = 24, 24
	base := solid(w, h, white)
	cand := withRect(base, image.Rect(2, 2, 9, 9), black)

	basePNG := encodePNG(t, base)
	candPNG := encodePNG(t, cand)

	eng := NewStdlibEngine()

	run := func() (Result, string) {
		b, err := eng.Decode(bytes.NewReader(basePNG))
		if err != nil {
			t.Fatalf("decode base: %v", err)
		}
		c, err := eng.Decode(bytes.NewReader(candPNG))
		if err != nil {
			t.Fatalf("decode cand: %v", err)
		}
		res, err := eng.Diff(b, c, DefaultOptions())
		if err != nil {
			t.Fatalf("diff: %v", err)
		}
		return res, ContentKey(res.DiffImage)
	}

	res1, key1 := run()
	res2, key2 := run()

	if res1.DiffPixels == 0 {
		t.Fatalf("expected a non-empty diff, got 0 pixels")
	}
	if res1.DiffPixels != res2.DiffPixels {
		t.Errorf("diffPixels not stable: %d vs %d", res1.DiffPixels, res2.DiffPixels)
	}
	if res1.DiffRatio != res2.DiffRatio {
		t.Errorf("diffRatio not stable: %v vs %v", res1.DiffRatio, res2.DiffRatio)
	}
	if key1 != key2 {
		t.Errorf("diff-image ContentKey not stable:\n %s\n %s", key1, key2)
	}
	if res1.Status != core.SnapshotChanged {
		t.Errorf("status = %q, want CHANGED", res1.Status)
	}
}

func TestEncodeDiffPNGDeterministic(t *testing.T) {
	img := withRect(solid(12, 12, white), image.Rect(1, 1, 5, 5), black)

	a, err := EncodeDiffPNG(img)
	if err != nil {
		t.Fatalf("EncodeDiffPNG: %v", err)
	}
	b, err := EncodeDiffPNG(img)
	if err != nil {
		t.Fatalf("EncodeDiffPNG: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("EncodeDiffPNG bytes not deterministic across calls")
	}

	// Re-decoding the encoded PNG must round-trip to the same ContentKey.
	got, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatalf("decode encoded png: %v", err)
	}
	if ContentKey(got) != ContentKey(img) {
		t.Errorf("ContentKey not stable across PNG round-trip")
	}
}

func TestContentKeyNamespacing(t *testing.T) {
	img := solid(3, 3, green)
	if ContentKey(img) == "" {
		t.Fatal("ContentKey returned empty string")
	}
	// Distinct pixels => distinct keys.
	if ContentKey(solid(3, 3, white)) == ContentKey(solid(3, 3, black)) {
		t.Error("different images produced the same ContentKey")
	}
	// Same pixels => same key (computed twice to assert determinism, not a tautology).
	k1 := ContentKey(solid(3, 3, green))
	k2 := ContentKey(solid(3, 3, green))
	if k1 != k2 {
		t.Error("identical images produced different ContentKeys")
	}
}
