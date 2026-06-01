import type {
  ApiErrorEnvelope,
  CreateBuildRequest,
  CreateBuildResponse,
  DeclareSnapshotRequest,
  DeclareSnapshotResponse,
  FinalizeBuildResponse,
} from './types';

/** Error thrown when the Pixela API returns a non-2xx response or is unreachable. */
export class PixelaApiError extends Error {
  readonly status: number | undefined;
  readonly code: string | undefined;

  constructor(message: string, status?: number, code?: string) {
    super(message);
    this.name = 'PixelaApiError';
    this.status = status;
    this.code = code;
  }

  /** Transient errors are worth retrying (network failure or 5xx / 429). */
  get transient(): boolean {
    if (this.status === undefined) {
      return true; // network-level failure
    }
    return this.status === 429 || this.status >= 500;
  }
}

export interface ClientOptions {
  /** API base URL, e.g. "http://localhost:3000". Trailing slash is tolerated. */
  baseUrl: string;
  /** Project API key, sent as `Authorization: ApiKey <key>`. */
  apiKey: string;
  /** Max attempts per request (including the first). Default 4. */
  maxAttempts?: number;
  /** Base backoff in ms (exponential). Default 250. */
  backoffBaseMs?: number;
  /** Injectable fetch (defaults to global). Injectable sleep for tests. */
  fetchImpl?: typeof fetch;
  sleep?: (ms: number) => Promise<void>;
}

const defaultSleep = (ms: number): Promise<void> => new Promise((r) => setTimeout(r, ms));

/** Thin typed HTTP client over global fetch with retry-with-backoff on transient errors. */
export class PixelaClient {
  private readonly baseUrl: string;
  private readonly apiKey: string;
  private readonly maxAttempts: number;
  private readonly backoffBaseMs: number;
  private readonly fetchImpl: typeof fetch;
  private readonly sleep: (ms: number) => Promise<void>;

  constructor(opts: ClientOptions) {
    this.baseUrl = opts.baseUrl.replace(/\/+$/, '');
    this.apiKey = opts.apiKey;
    this.maxAttempts = opts.maxAttempts ?? 4;
    this.backoffBaseMs = opts.backoffBaseMs ?? 250;
    this.fetchImpl = opts.fetchImpl ?? fetch;
    this.sleep = opts.sleep ?? defaultSleep;
  }

  private authHeader(): Record<string, string> {
    return { Authorization: `ApiKey ${this.apiKey}` };
  }

  private async request(path: string, init: RequestInit): Promise<Response> {
    const url = `${this.baseUrl}${path}`;
    let lastErr: PixelaApiError = new PixelaApiError('request never executed');

    for (let attempt = 1; attempt <= this.maxAttempts; attempt++) {
      try {
        const res = await this.fetchImpl(url, init);
        if (res.ok) {
          return res;
        }
        const apiErr = await this.toApiError(res);
        if (!apiErr.transient || attempt === this.maxAttempts) {
          throw apiErr;
        }
        lastErr = apiErr;
      } catch (err) {
        const apiErr =
          err instanceof PixelaApiError
            ? err
            : new PixelaApiError(`network error: ${(err as Error).message}`);
        if (!apiErr.transient || attempt === this.maxAttempts) {
          throw apiErr;
        }
        lastErr = apiErr;
      }
      // Exponential backoff with jitter before the next attempt.
      const delay = this.backoffBaseMs * 2 ** (attempt - 1) * (0.5 + Math.random());
      await this.sleep(delay);
    }
    throw lastErr;
  }

  private async toApiError(res: Response): Promise<PixelaApiError> {
    let code: string | undefined;
    let message = `HTTP ${res.status}`;
    try {
      const body = (await res.json()) as Partial<ApiErrorEnvelope>;
      if (body.error) {
        code = body.error.code;
        message = `${res.status} ${body.error.code}: ${body.error.message}`;
      }
    } catch {
      // non-JSON body; keep the generic message
    }
    return new PixelaApiError(message, res.status, code);
  }

  private async json<T>(res: Response): Promise<T> {
    return (await res.json()) as T;
  }

  /** `POST /api/v1/builds`. */
  async createBuild(body: CreateBuildRequest): Promise<CreateBuildResponse> {
    const res = await this.request('/api/v1/builds', {
      method: 'POST',
      headers: { ...this.authHeader(), 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return this.json<CreateBuildResponse>(res);
  }

  /** `POST /api/v1/builds/:buildId/snapshots` — declare a snapshot by hash (phase 1). */
  async declareSnapshot(
    buildId: string,
    body: DeclareSnapshotRequest,
  ): Promise<DeclareSnapshotResponse> {
    const res = await this.request(`/api/v1/builds/${encodeURIComponent(buildId)}/snapshots`, {
      method: 'POST',
      headers: { ...this.authHeader(), 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    return this.json<DeclareSnapshotResponse>(res);
  }

  /** `PUT /api/v1/images/:sha256` — upload raw PNG bytes (phase 2, only when needUpload). */
  async uploadImage(sha256: string, bytes: Buffer): Promise<void> {
    await this.request(`/api/v1/images/${encodeURIComponent(sha256)}`, {
      method: 'PUT',
      headers: { ...this.authHeader(), 'Content-Type': 'image/png' },
      // Uint8Array is an accepted BodyInit; Buffer extends it.
      body: new Uint8Array(bytes),
    });
  }

  /** `PATCH /api/v1/builds/:buildId` with `{ status: "FINALIZE" }`. */
  async finalizeBuild(buildId: string): Promise<FinalizeBuildResponse> {
    const res = await this.request(`/api/v1/builds/${encodeURIComponent(buildId)}`, {
      method: 'PATCH',
      headers: { ...this.authHeader(), 'Content-Type': 'application/json' },
      body: JSON.stringify({ status: 'FINALIZE' }),
    });
    return this.json<FinalizeBuildResponse>(res);
  }
}
