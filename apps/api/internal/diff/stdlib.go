package diff

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"

	pixelmatch "github.com/orisano/pixelmatch"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

// stdlibEngine is the v1 pure-Go diff engine: stdlib image/png for decode plus
// orisano/pixelmatch (pinned by exact commit) for per-pixel comparison. It is
// CGO-free so the binary stays a single static artifact (rulebook §10.1).
//
// Every decode is normalized to a canonical *image.NRGBA (8-bit, straight alpha,
// no gamma, no ICC). That canonical form is what BOTH diffing and the Encoder/CAS
// content-addressing operate on (§10.3, §10.4 #5), so results are reproducible
// across hosts and decoder versions.
type stdlibEngine struct{}

// Decode reads a PNG from r and returns it normalized to a canonical *image.NRGBA.
// All optional decode transforms are avoided: the image is simply drawn into a
// fresh NRGBA buffer, which performs 16→8 truncation and straight-alpha
// conversion deterministically without gamma or color-management (§10.4 #5).
func (stdlibEngine) Decode(r io.Reader) (image.Image, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("stdlib decode png: %w", err)
	}
	return toCanonicalNRGBA(img), nil
}

// Diff compares baseline against candidate under opts (rulebook §10, spec 07).
//
// Both images are re-normalized to canonical NRGBA (idempotent if already so). On a
// size mismatch the result is CHANGED with DiffRatio 1.0 and DiffPixels = candidate
// width*height and no diff image — the MVP decision (a): a structural change is not a
// pixel diff, and we never smart-resize (§10.4). Otherwise IgnoreRects are zeroed in
// BOTH buffers before pixelmatch runs, so masked regions can never register as
// changes (§10.4 #6).
func (stdlibEngine) Diff(baseline, candidate image.Image, opts Options) (Result, error) {
	base := toCanonicalNRGBA(baseline)
	cand := toCanonicalNRGBA(candidate)

	cb := cand.Bounds()
	candW, candH := cb.Dx(), cb.Dy()

	bb := base.Bounds()
	if bb.Dx() != candW || bb.Dy() != candH {
		// Size mismatch = structural change; do not resize, do not render a diff.
		return Result{
			Status:     core.SnapshotChanged,
			DiffRatio:  1.0,
			DiffPixels: candW * candH,
			DiffImage:  nil,
		}, nil
	}

	total := candW * candH
	if total == 0 {
		// Degenerate empty image: nothing to diff, nothing differs.
		return Result{
			Status:     core.SnapshotUnchanged,
			DiffRatio:  0,
			DiffPixels: 0,
			DiffImage:  nil,
		}, nil
	}

	// Apply ignore-rects deterministically to BOTH buffers before diffing.
	for _, rect := range opts.IgnoreRects {
		zeroRect(base, rect)
		zeroRect(cand, rect)
	}

	matchOpts := []pixelmatch.MatchOption{
		pixelmatch.Threshold(opts.PixelThreshold),
	}
	if opts.IncludeAA {
		matchOpts = append(matchOpts, pixelmatch.IncludeAntiAlias)
	}
	if opts.DiffColor != nil {
		matchOpts = append(matchOpts, pixelmatch.DiffColor(opts.DiffColor))
	}

	var diffImg image.Image
	matchOpts = append(matchOpts, pixelmatch.WriteTo(&diffImg))

	diffPixels, err := pixelmatch.MatchPixel(base, cand, matchOpts...)
	if err != nil {
		// Bounds are guaranteed equal above; any error here is unexpected.
		return Result{}, fmt.Errorf("stdlib pixelmatch: %w", err)
	}

	status := core.SnapshotChanged
	if diffPixels == 0 {
		status = core.SnapshotUnchanged
	}

	return Result{
		Status:     status,
		DiffPixels: diffPixels,
		DiffRatio:  float64(diffPixels) / float64(total),
		DiffImage:  diffImg,
	}, nil
}

// errNilImage guards the public Encoder/CAS helpers against nil inputs.
var errNilImage = errors.New("nil image")

// toCanonicalNRGBA returns img as a canonical *image.NRGBA whose bounds start at the
// origin. If img is already such an NRGBA it is returned unchanged; otherwise its
// pixels are drawn into a fresh origin-anchored NRGBA buffer (8-bit, straight alpha).
// draw.Draw performs deterministic 16→8 and premultiplied→straight conversion with no
// gamma or color management, matching the determinism rules (§10.4 #5).
func toCanonicalNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Bounds().Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// zeroRect sets every pixel of img inside rect (intersected with the image bounds) to
// transparent black. Masking both buffers identically keeps ignore-regions out of the
// comparison deterministically (§10.4 #6).
func zeroRect(img *image.NRGBA, rect image.Rectangle) {
	r := rect.Intersect(img.Bounds())
	zero := color.NRGBA{}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.SetNRGBA(x, y, zero)
		}
	}
}
