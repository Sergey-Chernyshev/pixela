import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import type { ProjectView } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';
import { StatusPill } from '../../shared/status-pill';

/**
 * Projects — the organization-level overview: every repository the current user is a member of, shown
 * as a grid of cards with real aggregates from the API (name, slug, default branch, role, created date,
 * latest-build health ratio, member count, open reviews, last build status). The mock's sparklines,
 * flaky counts and team-avatar stack have no backing endpoint, so they remain honestly omitted.
 *
 * State is the loading/error/data trio: `data` defaults to null (not []) so the template can tell
 * "not loaded yet" (skeleton) apart from "loaded, empty" (empty-state) — loading-state modelling.
 */
@Component({
  selector: 'px-projects',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, AppShell, StatusPill],
  templateUrl: './projects.html',
  styleUrl: './projects.scss',
})
export class Projects {
  private readonly api = inject(ApiService);

  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly data = signal<ProjectView[] | null>(null);

  protected readonly count = computed(() => this.data()?.length ?? 0);

  /** Org-wide open reviews — a real sum across the user's projects (KPI strip). */
  protected readonly totalOpenReviews = computed(() =>
    (this.data() ?? []).reduce((acc, p) => acc + p.openReviews, 0),
  );

  /** Placeholder rows for the loading skeleton — six cards mirrors a typical org page. */
  protected readonly skeletons = [0, 1, 2, 3, 4, 5];

  constructor() {
    void this.load();
  }

  protected async load(): Promise<void> {
    this.loading.set(true);
    this.error.set(null);
    try {
      const res = await firstValueFrom(this.api.projects());
      this.data.set(res.projects ?? []);
    } catch {
      this.error.set('Не удалось загрузить проекты');
      this.data.set(null);
    } finally {
      this.loading.set(false);
    }
  }

  /**
   * Member role → role-badge classes + Russian label. Only OWNER/MEMBER are issued by the API.
   * Returns base + modifier together so a single `[class]` binding keeps `.role` (a whole-element
   * `[class]` binding replaces the static `class` attribute rather than merging with it).
   */
  protected roleClass(role: string): string {
    if (role === 'OWNER') return 'role role--admin';
    if (role === 'MEMBER') return 'role role--dev';
    return 'role role--viewer';
  }

  protected roleLabel(role: string): string {
    if (role === 'OWNER') return 'Владелец';
    if (role === 'MEMBER') return 'Участник';
    return role;
  }

  /** Created date as a short ru date (the API gives an ISO timestamp; no relative-time fabrication). */
  protected createdLabel(iso: string): string {
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short', year: 'numeric' });
  }

  /** Health = latest build's in-norm snapshot ratio (healthOk/healthTotal). null when no build yet. */
  protected healthPct(p: ProjectView): number | null {
    if (!p.healthTotal) return null;
    return Math.round((p.healthOk / p.healthTotal) * 1000) / 10;
  }

  /** Bar segment widths (%), derived from the real latest-build counts. */
  protected okWidth(p: ProjectView): number {
    return p.healthTotal ? (p.healthOk / p.healthTotal) * 100 : 0;
  }

  /** Plural "проверка/проверки/проверок" for the open-reviews tag. */
  protected reviewsWord(n: number): string {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return 'проверка';
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return 'проверки';
    return 'проверок';
  }

  /** Plural "участник/участника/участников". */
  protected membersWord(n: number): string {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return 'участник';
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return 'участника';
    return 'участников';
  }
}
