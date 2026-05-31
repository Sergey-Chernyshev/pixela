import { Injectable, Logger } from '@nestjs/common';
import { PrismaService } from '../prisma/prisma.service';
import { RedisService } from '../redis/redis.service';

export type ComponentStatus = 'up' | 'down';

export interface HealthResult {
  status: 'ok' | 'error';
  checks: {
    database: ComponentStatus;
    redis: ComponentStatus;
  };
  uptimeSeconds: number;
  timestamp: string;
}

/**
 * Real readiness check: pings Postgres and Redis concurrently. The result drives the
 * HTTP status code (200 only when both are up) — this is the "green baseline" of Phase 0.
 */
@Injectable()
export class HealthService {
  private readonly logger = new Logger(HealthService.name);

  constructor(
    private readonly prisma: PrismaService,
    private readonly redis: RedisService,
  ) {}

  async check(): Promise<HealthResult> {
    const [database, redis] = await Promise.all([
      this.probe('database', () => this.prisma.ping()),
      this.probe('redis', () => this.redis.ping()),
    ]);

    const status: HealthResult['status'] = database === 'up' && redis === 'up' ? 'ok' : 'error';

    return {
      status,
      checks: { database, redis },
      uptimeSeconds: Math.round(process.uptime()),
      timestamp: new Date().toISOString(),
    };
  }

  private async probe(name: string, fn: () => Promise<void>): Promise<ComponentStatus> {
    try {
      await fn();
      return 'up';
    } catch (err) {
      const message = err instanceof Error ? err.message : String(err);
      this.logger.warn(`Health check '${name}' failed: ${message}`);
      return 'down';
    }
  }
}
