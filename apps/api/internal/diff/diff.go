// Package diff defines the DiffEngine seam: a deterministic, codec-agnostic image
// comparison engine. Per docs/architecture/go-backend.md §10.2 the engine is a
// consumer-side interface operating on decoded image.Image values (NOT []byte) so it
// stays independent of the codec choice and can address content by decoded pixels.
//
// Encoding the diff PNG and computing its content-address are a SEPARATE seam
// (Encoder/CAS): the engine never owns byte serialization. The default implementation
// is injected as a dependency (Mat Ryer style) so the API binary builds without ever
// importing the diff impl, and tests inject a fake.
//
// Phase 0 ships only the interface, the pinned defaults, and a stub. The real pure-Go
// engine (orisano/pixelmatch + image/png) lands in Phase 2; see the doc block on
// stdlibEngine for the planned implementation.
package diff

import (
	"image"
	"image/color"
	"io"

	"github.com/Sergey-Chernyshev/pixela/apps/api/internal/core"
)

// Options are the deterministic knobs; pin them once (rulebook §10.4) and assert
// them in tests. They must be fully reproducible: the same inputs and options always
// yield the same Result, regardless of host, GOMAXPROCS, or invocation order.
type Options struct {
	// PixelThreshold is the pixelmatch per-pixel color-distance threshold in [0,1];
	// lower is stricter (more pixels flagged as different).
	PixelThreshold float64
	// IncludeAA, when true, counts anti-aliased pixels as differences. Default false:
	// anti-aliasing noise is the dominant source of visual-regression flakiness.
	IncludeAA bool
	// DiffColor is the color used to paint differing pixels in the diff image.
	DiffColor color.Color
	// IgnoreRects are regions zeroed in BOTH decoded buffers before diffing, so masked
	// areas (dynamic content, timestamps) never register as changes. Applied
	// deterministically (rulebook §10.4 #6).
	IgnoreRects []image.Rectangle
}

// Result is the outcome of comparing a baseline against a candidate.
type Result struct {
	// Status is the per-snapshot verdict (e.g. SnapshotUnchanged, SnapshotChanged).
	Status core.SnapshotStatus
	// DiffPixels is the count of pixels that differ under the given options.
	DiffPixels int
	// DiffRatio is DiffPixels divided by the total pixel count, in [0,1].
	DiffRatio float64
	// DiffImage is the rendered visualization of the differences. It is decoded pixels
	// only; encoding it to PNG and content-addressing it belong to the Encoder/CAS seam.
	DiffImage image.Image
}

// Engine decodes images and diffs decoded pixels. Implementations MUST be deterministic
// (rulebook §10): identical inputs and options always produce an identical Result.
// Decode and Diff are split so callers can decode once (e.g. baseline + candidate in
// parallel) and feed the same image.Image to multiple comparisons.
type Engine interface {
	// Decode reads an encoded image from r and returns its decoded form. Implementations
	// normalize to a canonical representation before returning (see stdlibEngine docs).
	Decode(r io.Reader) (image.Image, error)
	// Diff compares baseline against candidate under opts and returns the result.
	Diff(baseline, candidate image.Image, opts Options) (Result, error)
}

// DefaultOptions returns the pinned default knobs (rulebook §10.4 #4). These are chosen
// once and locked; the golden-master test asserts them so a change is a deliberate,
// reviewed event rather than a silent flake.
//
//   - PixelThreshold = 0.1 — the pixelmatch library default; a sensible balance between
//     catching real regressions and tolerating sub-perceptual color jitter.
//   - IncludeAA = false — anti-aliased pixels are NOT counted; AA noise is the dominant
//     cause of false positives across font/GPU/driver variation.
//   - DiffColor = opaque red (255,0,0,255) — the conventional, high-contrast diff color.
//   - IgnoreRects = nil — no masking by default; callers opt in per snapshot.
func DefaultOptions() Options {
	return Options{
		PixelThreshold: 0.1,
		IncludeAA:      false,
		DiffColor:      color.RGBA{R: 255, G: 0, B: 0, A: 255},
		IgnoreRects:    nil,
	}
}

// NewStdlibEngine returns the v1 pure-Go engine (image/png + orisano/pixelmatch),
// CGO-free for a single static binary. The implementation lives in stdlib.go.
func NewStdlibEngine() Engine {
	return stdlibEngine{}
}
