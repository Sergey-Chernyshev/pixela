import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import type { ProjectView } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';

/**
 * Projects — the organization-level overview: every repository the current user is a member of, shown
 * as a grid of cards (name, slug, default branch, the member's role, created date). The design mock
 * also carried per-project health %, sparklines, open-review tallies, flaky counts and team avatars —
 * none of which the API exposes, so they are honestly omitted (see apiGapsHandled).
 *
 * State is the loading/error/data trio: `data` defaults to null (not []) so the template can tell
 * "not loaded yet" (skeleton) apart from "loaded, empty" (empty-state) — loading-state modelling.
 */
@Component({
  selector: 'px-projects',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, AppShell],
  templateUrl: './projects.html',
  styleUrl: './projects.scss',
})
export class Projects {
  private readonly api = inject(ApiService);

  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly data = signal<ProjectView[] | null>(null);

  protected readonly count = computed(() => this.data()?.length ?? 0);

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
}
