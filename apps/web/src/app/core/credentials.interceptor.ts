import { HttpInterceptorFn } from '@angular/common/http';

/**
 * credentialsInterceptor attaches `withCredentials` to every request so the HttpOnly `pixela_session`
 * cookie is sent with API calls (the dashboard session is server-side; the browser holds only the
 * cookie). 401 handling lives in the route guard, not here — a blanket redirect-on-401 would fight the
 * login flow (a failed /auth/me during guard resolution is expected, not an error to act on globally).
 */
export const credentialsInterceptor: HttpInterceptorFn = (req, next) => {
  return next(req.clone({ withCredentials: true }));
};
