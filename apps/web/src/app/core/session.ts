import { Injectable, computed, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import type { User } from '@pixela/shared';
import { ApiService } from './api';

/**
 * SessionService is the single source of truth for the authenticated dashboard user. The session itself
 * lives server-side in Redis (HttpOnly cookie); this service only mirrors *who* is logged in as a
 * signal, so the shell, guard and pages react to login/logout without prop-drilling.
 *
 * `status` distinguishes "haven't checked yet" (unknown) from "checked, nobody" (anon) so the guard can
 * gate on the resolved truth — never a flash of the wrong UI on first load (loading-state modelling).
 */
@Injectable({ providedIn: 'root' })
export class SessionService {
  private readonly api = inject(ApiService);

  private readonly _user = signal<User | null>(null);
  private readonly _status = signal<'unknown' | 'authed' | 'anon'>('unknown');

  /** The current user, or null when not authenticated / not yet resolved. */
  readonly user = this._user.asReadonly();
  readonly status = this._status.asReadonly();
  readonly isAuthed = computed(() => this._status() === 'authed');

  /** Resolve the session once (called by the guard). Caches the result in the signal. */
  async ensureLoaded(): Promise<boolean> {
    if (this._status() !== 'unknown') return this._status() === 'authed';
    return this.refresh();
  }

  /** Force a fresh /auth/me. A 401 (or any error) resolves to anonymous, never throws to the caller. */
  async refresh(): Promise<boolean> {
    try {
      const user = await firstValueFrom(this.api.me());
      this._user.set(user);
      this._status.set('authed');
      return true;
    } catch {
      this._user.set(null);
      this._status.set('anon');
      return false;
    }
  }

  /** Log in with email + password. On success the cookie is set and the user signal is populated. */
  async login(email: string, password: string): Promise<void> {
    await firstValueFrom(this.api.login(email, password));
    await this.refresh();
  }

  /** Log out: revoke the server session, then clear local state. */
  async logout(): Promise<void> {
    try {
      await firstValueFrom(this.api.logout());
    } finally {
      this._user.set(null);
      this._status.set('anon');
    }
  }
}
