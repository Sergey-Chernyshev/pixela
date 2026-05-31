import { Injectable, Logger, OnModuleDestroy, OnModuleInit } from '@nestjs/common';
import { PrismaClient } from '@prisma/client';

/**
 * Thin wrapper over PrismaClient wired into the Nest lifecycle.
 * Boot is resilient: if the DB is unreachable at startup we log and continue so that
 * `/health` can still answer 503 (rather than the whole process failing to boot).
 */
@Injectable()
export class PrismaService extends PrismaClient implements OnModuleInit, OnModuleDestroy {
  private readonly logger = new Logger(PrismaService.name);

  async onModuleInit(): Promise<void> {
    try {
      await this.$connect();
      this.logger.log('Postgres connected');
    } catch (err) {
      this.logger.warn(`Postgres not reachable at startup: ${(err as Error).message}`);
    }
  }

  async onModuleDestroy(): Promise<void> {
    await this.$disconnect();
  }

  /** Liveness probe — a real round-trip to Postgres. Throws if unreachable. */
  async ping(): Promise<void> {
    await this.$queryRaw`SELECT 1`;
  }
}
