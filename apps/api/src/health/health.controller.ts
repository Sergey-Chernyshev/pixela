import { Controller, Get, HttpStatus, Res } from '@nestjs/common';
import type { Response } from 'express';
import { HealthResult, HealthService } from './health.service';

@Controller('health')
export class HealthController {
  constructor(private readonly health: HealthService) {}

  /**
   * GET /health — 200 when Postgres AND Redis are reachable, 503 otherwise.
   * Mounted at the root (excluded from the /api global prefix) so it doubles as the
   * container/CI liveness probe.
   */
  @Get()
  async check(@Res({ passthrough: true }) res: Response): Promise<HealthResult> {
    const result = await this.health.check();
    res.status(result.status === 'ok' ? HttpStatus.OK : HttpStatus.SERVICE_UNAVAILABLE);
    return result;
  }
}
