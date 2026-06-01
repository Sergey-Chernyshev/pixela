/**
 * Minimal PNG header parsing — extract intrinsic pixel dimensions without decoding the image.
 *
 * PNG layout: 8-byte signature, then the IHDR chunk first. IHDR's data begins at byte 16
 * with a big-endian uint32 width then height. We only need those 8 bytes.
 */

const PNG_SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

export interface PngDimensions {
  width: number;
  height: number;
}

/** True if the buffer starts with the PNG magic signature. */
export function isPng(buf: Buffer): boolean {
  return (
    buf.length >= PNG_SIGNATURE.length &&
    buf.subarray(0, PNG_SIGNATURE.length).equals(PNG_SIGNATURE)
  );
}

/**
 * Read intrinsic width/height from a PNG buffer's IHDR chunk.
 * @throws if the buffer is not a valid PNG / is too short to contain IHDR.
 */
export function parsePngDimensions(buf: Buffer): PngDimensions {
  if (!isPng(buf)) {
    throw new Error('parsePngDimensions: not a PNG (bad signature)');
  }
  // Signature (8) + chunk length (4) + chunk type "IHDR" (4) = 16; width at 16, height at 20.
  if (buf.length < 24) {
    throw new Error('parsePngDimensions: buffer too short for IHDR');
  }
  const chunkType = buf.subarray(12, 16).toString('ascii');
  if (chunkType !== 'IHDR') {
    throw new Error(`parsePngDimensions: first chunk is "${chunkType}", expected IHDR`);
  }
  const width = buf.readUInt32BE(16);
  const height = buf.readUInt32BE(20);
  return { width, height };
}
