import assert from 'node:assert/strict';
import { test } from 'node:test';

import { isPng, parsePngDimensions } from './png';

/** Build a minimal valid PNG header (signature + IHDR length/type + width/height). */
function pngHeader(width: number, height: number): Buffer {
  const sig = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
  const ihdr = Buffer.alloc(16);
  ihdr.writeUInt32BE(13, 0); // IHDR data length
  ihdr.write('IHDR', 4, 'ascii');
  ihdr.writeUInt32BE(width, 8);
  ihdr.writeUInt32BE(height, 12);
  return Buffer.concat([sig, ihdr]);
}

test('isPng accepts the PNG signature', () => {
  assert.equal(isPng(pngHeader(1, 1)), true);
});

test('isPng rejects non-PNG bytes', () => {
  assert.equal(isPng(Buffer.from('not a png')), false);
});

test('parsePngDimensions reads width and height from IHDR', () => {
  const { width, height } = parsePngDimensions(pngHeader(1280, 720));
  assert.equal(width, 1280);
  assert.equal(height, 720);
});

test('parsePngDimensions throws on bad signature', () => {
  assert.throws(() => parsePngDimensions(Buffer.from('nope-nope-nope-nope-nope')), /not a PNG/);
});

test('parsePngDimensions throws on truncated buffer', () => {
  assert.throws(
    () => parsePngDimensions(Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])),
    /too short/,
  );
});
