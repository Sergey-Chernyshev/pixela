import { createHash } from 'node:crypto';

/** Compute the hex sha256 of a buffer (matches the server's CAS key for the raw PNG bytes). */
export function sha256Hex(buf: Buffer): string {
  return createHash('sha256').update(buf).digest('hex');
}
