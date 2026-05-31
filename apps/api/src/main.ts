import 'reflect-metadata';
import { Logger, ValidationPipe } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app.module';

type ApiMode = 'http' | 'worker';

function resolveMode(): ApiMode {
  const mode = (process.env.API_MODE ?? 'http').toLowerCase();
  if (mode !== 'http' && mode !== 'worker') {
    throw new Error(`Invalid API_MODE='${mode}' (expected 'http' or 'worker')`);
  }
  return mode;
}

async function bootstrapHttp(logger: Logger): Promise<void> {
  const app = await NestFactory.create(AppModule, { bufferLogs: false });

  // All feature routes live under /api; /health stays at the root for probes.
  app.setGlobalPrefix('api', { exclude: ['health'] });
  app.useGlobalPipes(new ValidationPipe({ whitelist: true, transform: true }));
  app.enableShutdownHooks();

  const origin = process.env.CORS_ORIGIN;
  if (origin) {
    app.enableCors({ origin, credentials: true });
  }

  const port = Number(process.env.PORT ?? 3000);
  await app.listen(port, '0.0.0.0');
  logger.log(`Pixela API listening on :${port} (HTTP mode). Health: GET /health`);
}

async function bootstrapWorker(logger: Logger): Promise<void> {
  // Worker mode boots the DI context (Prisma/Redis connect; BullMQ consumers register
  // here from Phase 2) without an HTTP listener. The open Redis socket keeps it alive.
  // enableShutdownHooks() drains onModuleDestroy on SIGINT/SIGTERM and re-raises the signal
  // for a correct exit code — so no manual signal handlers (which would double-run destroy
  // hooks and race process.exit against Nest's process.kill).
  const app = await NestFactory.createApplicationContext(AppModule);
  app.enableShutdownHooks();
  logger.log('Pixela API booted in WORKER mode (no HTTP listener).');
}

async function bootstrap(): Promise<void> {
  const mode = resolveMode();
  const logger = new Logger('Bootstrap');
  if (mode === 'worker') {
    await bootstrapWorker(logger);
  } else {
    await bootstrapHttp(logger);
  }
}

bootstrap().catch((err) => {
  new Logger('Bootstrap').error(err);
  process.exit(1);
});
