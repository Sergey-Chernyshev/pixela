import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { SessionService } from './session';

/**
 * authGuard protects the dashboard. It resolves the session once (server-side /auth/me) and, if the
 * caller is anonymous, redirects to /login preserving the intended URL in `returnUrl`. Resolving before
 * deciding avoids a flash of the dashboard for an unauthenticated visitor.
 */
export const authGuard: CanActivateFn = async (_route, state) => {
  const session = inject(SessionService);
  const router = inject(Router);
  if (await session.ensureLoaded()) return true;
  return router.createUrlTree(['/login'], { queryParams: { returnUrl: state.url } });
};

/**
 * guestGuard keeps an already-authenticated user out of /login (sends them to the dashboard root).
 */
export const guestGuard: CanActivateFn = async () => {
  const session = inject(SessionService);
  const router = inject(Router);
  if (await session.ensureLoaded()) return router.createUrlTree(['/']);
  return true;
};
