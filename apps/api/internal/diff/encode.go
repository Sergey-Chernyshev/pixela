package diff

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
)

// contentKeyTag namespaces the diff content-address. Bumping it lets the diff
// renderer be migrated intentionally instead of silently re-keying every diff
// (rulebook §10.3).
const contentKeyTag = "pixela-diff/v1"

// diffEncoder is the single, reused PNG encoder for diff images. Reusing ONE
// encoder value with a fixed CompressionLevel is what makes the encoded bytes
// deterministic across calls (rulebook §10.4 #2) — DefaultCompression must never
// be used here.
var diffEncoder = png.Encoder{CompressionLevel: png.BestCompression}

// EncodeDiffPNG encodes img to PNG bytes using the shared deterministic encoder.
// This is the Encoder seam: the diff engine never owns byte serialization (§10.2).
func EncodeDiffPNG(img image.Image) ([]byte, error) {
	if img == nil {
		return nil, fmt.Errorf("encode diff png: %w", errNilImage)
	}
	var buf bytes.Buffer
	if err := diffEncoder.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode diff png: %w", err)
	}
	return buf.Bytes(), nil
}

// ContentKey computes the content-address of img by its DECODED canonical pixels —
// NOT its compressed bytes — so the key survives toolchain/encoder changes (§10.3).
//
// The hashed serialization is fixed:
//
//	sha256( contentKeyTag || uint32(width) || uint32(height) || canonical NRGBA pixels )
//
// width/height are big-endian uint32; the pixels are the origin-anchored canonical
// NRGBA Pix bytes (rows packed tightly, no stride padding). The result is lowercase
// hex.
func ContentKey(img image.Image) string {
	canonical := toCanonicalNRGBA(img)
	b := canonical.Bounds()
	w, h := b.Dx(), b.Dy()

	hsh := sha256.New()
	hsh.Write([]byte(contentKeyTag))

	var dims [8]byte
	//nolint:gosec // image dimensions are non-negative and well below 2^32
	binary.BigEndian.PutUint32(dims[0:4], uint32(w))
	//nolint:gosec // image dimensions are non-negative and well below 2^32
	binary.BigEndian.PutUint32(dims[4:8], uint32(h))
	hsh.Write(dims[:])

	// Write pixels row-by-row to exclude any stride padding from the hash, so the
	// serialization depends only on logical pixels, never on buffer layout.
	rowLen := w * 4
	for y := 0; y < h; y++ {
		off := canonical.PixOffset(b.Min.X, b.Min.Y+y)
		hsh.Write(canonical.Pix[off : off+rowLen])
	}

	return hex.EncodeToString(hsh.Sum(nil))
}
