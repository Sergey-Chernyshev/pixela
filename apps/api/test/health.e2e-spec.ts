// Reaper (ryuk) isn't needed — Testcontainers stops containers in afterAll. Disabling it
// avoids pulling an extra image and speeds the cold run. Must be set before any container start.
process.env.TESTCONTAINERS_RYUK_DISABLED = 'true';

import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { HttpStatus, INestApplication } from '@nestjs/common';
import { Test } from '@nestjs/testing';
import { PostgreSqlContainer, StartedPostgreSqlContainer } from '@testcontainers/postgresql';
import { RedisContainer, StartedRedisContainer } from '@testcontainers/redis';
import request from 'supertest';
import { PrismaService } from '../src/prisma/prisma.service';
import { AppModule } from '../src/app.module';
import type { HealthResult } from '../src/health/health.service';

/**
 * Phase 0 "green baseline": boots the Nest app against ephemeral Postgres + Redis,
 * applies the migration on a clean DB, and proves /health reports both dependencies up.
 * Hermetic — needs a Docker daemon but no pre-started infra; leaves no state behind.
 */
describe('Phase 0 smoke — /health against real Postgres + Redis', () => {
  let postgres: StartedPostgreSqlContainer;
  let redis: StartedRedisContainer;
  let app: INestApplication;

  beforeAll(async () => {
    // Start sequentially, not via Promise.all: concurrent first-time container starts race
    // on Docker Desktop's runtime-client init / port publishing and surface as
    // "No host port found for host IP".
    postgres = await new PostgreSqlContainer('postgres:16')
      .withDatabase('pixela')
      .withUsername('pixela')
      .withPassword('pixela')
      .start();
    redis = await new RedisContainer('redis:7').start();

    process.env.DATABASE_URL = postgres.getConnectionUri();
    process.env.REDIS_URL = redis.getConnectionUrl();

    // Prove the migration applies on a clean database (Phase 0 DoD).
    // execFile (fixed args, no shell) — not exec — so there is no command-injection surface.
    execFileSync('npx', ['prisma', 'migrate', 'deploy'], {
      cwd: join(__dirname, '..'),
      env: process.env,
      stdio: 'inherit',
    });

    const moduleRef = await Test.createTestingModule({ imports: [AppModule] }).compile();
    app = moduleRef.createNestApplication();
    await app.init();
  }, 180_000);

  afterAll(async () => {
    await app?.close();
    await postgres?.stop();
    await redis?.stop();
  });

  it('GET /health returns 200 with both dependencies up', async () => {
    const res = await request(app.getHttpServer()).get('/health');

    expect(res.status).toBe(HttpStatus.OK);
    const body = res.body as HealthResult;
    expect(body.status).toBe('ok');
    expect(body.checks.database).toBe('up');
    expect(body.checks.redis).toBe('up');
    expect(typeof body.uptimeSeconds).toBe('number');
    expect(typeof body.timestamp).toBe('string');
  });

  it('migration created the schema on a clean DB (Project table queryable, empty)', async () => {
    const prisma = app.get(PrismaService);
    await expect(prisma.project.count()).resolves.toBe(0);
  });
});
