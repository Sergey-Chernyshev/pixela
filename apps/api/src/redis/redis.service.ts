import { Injectable, Logger, OnModuleDestroy } from '@nestjs/common';
import { ConfigService } from '@nestjs/config';
import Redis from 'ioredis';

const PING_TIMEOUT_MS = 2500;

/**
 * Shared Redis connection (used by BullMQ + server-side sessions in later phases).
 * ioredis connects in the background; the process always boots. `ping()` races a real
 * PING against a timeout so `/health` fails fast (503) when Redis is unreachable.
 */
@Injectable()
export class RedisService implements OnModuleDestroy {
  private readonly logger = new Logger(RedisService.name);
  private readonly client: Redis;

  constructor(config: ConfigService) {
    const url = config.getOrThrow<string>('REDIS_URL');
    this.client = new Redis(url, {
      maxRetriesPerRequest: 2,
      connectTimeout: 3000,
    });
    this.client.on('error', (err) => this.logger.warn(`Redis: ${err.message}`));
    this.client.on('ready', () => this.logger.log('Redis connected'));
  }

  async onModuleDestroy(): Promise<void> {
    this.client.disconnect();
  }

  /** Liveness probe — a real PING to Redis, bounded by a timeout. Throws if unreachable. */
  async ping(): Promise<void> {
    const reply = await this.withTimeout(this.client.ping(), PING_TIMEOUT_MS);
    if (reply !== 'PONG') {
      throw new Error(`Unexpected Redis ping reply: ${reply}`);
    }
  }

  /** The raw ioredis client, for queue/session wiring in later phases. */
  get raw(): Redis {
    return this.client;
  }

  private withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Redis ping timed out after ${ms}ms`)), ms);
      promise.then(
        (value) => {
          clearTimeout(timer);
          resolve(value);
        },
        (err) => {
          clearTimeout(timer);
          reject(err instanceof Error ? err : new Error(String(err)));
        },
      );
    });
  }
}
