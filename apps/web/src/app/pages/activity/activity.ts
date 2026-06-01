import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import type { ActivityEntry } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';

/** One day-bucket of events with its Russian header label ("Сегодня" / "Вчера" / absolute date). */
interface DayGroup {
  key: string;
  label: string;
  events: ActivityEntry[];
}

/**
 * Activity — the organization-wide approval feed (design: project/activity.html). A timeline of
 * approve/reject decisions across every project the user can see, newest-first and bucketed by day
 * (Сегодня / Вчера / absolute date from the real `at` timestamp). Each row carries the action marker
 * (green check for APPROVE, red x for REJECT), the actor (avatar initials + email — the API gives no
 * display name), what happened ("одобрил/отклонил снимок …"), the project + branch as a link to the
 * snapshot review, and a relative time.
 *
 * Honest-to-API: the mock's baseline / failed-build / flaky / member-invite / comment event types,
 * per-author display names and project colours have no backing fields here, so they are omitted rather
 * than fabricated. State is the loading/error/data trio — `data` defaults to null so the skeleton holds
 * the layout until the first response (loading-state modelling).
 */
@Component({
  selector: 'px-activity',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, AppShell],
  templateUrl: './activity.html',
  styleUrl: './activity.scss',
})
export class Activity {
  private readonly api = inject(ApiService);

  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly data = signal<ActivityEntry[] | null>(null);

  protected readonly isEmpty = computed(
    () => !this.loading() && !this.error() && (this.data()?.length ?? 0) === 0,
  );

  /** Placeholder rows for the loading skeleton (one day-group, a few events). */
  protected readonly skeletonRows = [0, 1, 2, 3];

  /** Events bucketed into day-groups, newest-first within and across groups. */
  protected readonly groups = computed<DayGroup[]>(() => {
    const items = this.data();
    if (!items) return [];
    const sorted = [...items].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());
    const map = new Map<string, DayGroup>();
    for (const ev of sorted) {
      const key = this.dayKey(ev.at);
      let group = map.get(key);
      if (!group) {
        group = { key, label: this.dayLabel(ev.at), events: [] };
        map.set(key, group);
      }
      group.events.push(ev);
    }
    return [...map.values()];
  });

  constructor() {
    void this.load();
  }

  protected async load(): Promise<void> {
    this.loading.set(true);
    this.error.set(null);
    try {
      const res = await firstValueFrom(this.api.activity());
      this.data.set(res.activity ?? []);
    } catch {
      this.error.set('Не удалось загрузить события');
      this.data.set(null);
    } finally {
      this.loading.set(false);
    }
  }

  protected isApprove(entry: ActivityEntry): boolean {
    return entry.action === 'APPROVE';
  }

  /** "одобрил снимок" / "отклонил снимок" — what happened, before the snapshot name. */
  protected actionText(entry: ActivityEntry): string {
    return this.isApprove(entry) ? 'одобрил снимок' : 'отклонил снимок';
  }

  /** Avatar initials from an email local-part ("mira.kan@…" → "MK", "ari@…" → "AR"). */
  protected initials(email: string): string {
    const local = (email ?? '').split('@')[0]?.trim() ?? '';
    if (!local) return '??';
    const parts = local.split(/[._-]+/).filter(Boolean);
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
    return local.slice(0, 2).toUpperCase();
  }

  /** Stable per-email avatar colour (deterministic hue from a hashed email — no invented identity). */
  protected avatarColor(email: string): string {
    let hash = 0;
    for (let i = 0; i < email.length; i++) {
      hash = (hash * 31 + email.charCodeAt(i)) | 0;
    }
    const hue = Math.abs(hash) % 360;
    return `hsl(${hue} 42% 48%)`;
  }

  /** Calendar-day bucket key (local time), so events on the same day group together. */
  private dayKey(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return 'unknown';
    return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
  }

  /** Day-group header: "Сегодня" / "Вчера" / absolute ru date ("28 мая" or "28 мая 2025"). */
  private dayLabel(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    const today = new Date();
    const yesterday = new Date();
    yesterday.setDate(today.getDate() - 1);
    if (this.dayKey(iso) === this.dayKey(today.toISOString())) return 'Сегодня';
    if (this.dayKey(iso) === this.dayKey(yesterday.toISOString())) return 'Вчера';
    const sameYear = d.getFullYear() === today.getFullYear();
    return d.toLocaleDateString('ru-RU', {
      day: 'numeric',
      month: 'long',
      ...(sameYear ? {} : { year: 'numeric' }),
    });
  }

  /**
   * Relative time. Same-day events show the wall-clock (HH:mm); older events fall back to a coarse
   * Russian relative label so day-grouping stays the primary axis.
   */
  protected relTime(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return '';
    const diff = Date.now() - then;
    const sec = Math.round(diff / 1000);
    if (sec < 45) return 'сейчас';
    const min = Math.round(sec / 60);
    if (min < 60) return `${min} мин`;
    const hr = Math.round(min / 60);
    if (hr < 12) return `${hr} ч`;
    return new Date(then).toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit' });
  }
}
