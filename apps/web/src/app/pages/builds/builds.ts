import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
} from '@angular/core';
import { RouterLink } from '@angular/router';
import { type Observable, catchError, of, switchMap } from 'rxjs';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import type { BuildListItem, BuildsPage, ProjectList } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';
import { CountChips } from '../../shared/count-chips';
import { StatusPill } from '../../shared/status-pill';

/**
 * Builds — the project-level CI feed (design: project/buildlist.html). A dense list of build runs:
 * branch + commit, status pill, per-status count chips, relative time and an optional CI link. Each row
 * links to the build detail. Loads page-by-page from the API (loading-state modelling: data defaults to
 * null and the skeleton holds the layout until the first page resolves). The design's author avatar/name
 * and run duration have no backing API data, so they are omitted rather than fabricated.
 */
@Component({
  selector: 'px-builds',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, AppShell, StatusPill, CountChips],
  templateUrl: './builds.html',
  styleUrl: './builds.scss',
})
export class Builds {
  private readonly api = inject(ApiService);

  /** Route param (auto-bound by withComponentInputBinding). */
  readonly projectId = input.required<string>();

  /** Current 1-based page (starts at 1; the component is re-created per project route). */
  protected readonly page = signal(1);

  /** Project name for the shell header — resolved from the projects list, falls back to the id string. */
  private readonly projects$: Observable<ProjectList | null> = this.api
    .projects()
    .pipe(catchError(() => of<ProjectList | null>(null)));
  private readonly projects = toSignal(this.projects$, { initialValue: null });
  protected readonly projectName = computed(() => {
    const id = this.projectId();
    const list = this.projects()?.projects ?? [];
    const found = list.find((p) => p.id === id);
    return found?.name ?? id;
  });

  // ---- builds feed (loading / error / data trio) ----
  protected readonly loading = signal(true);
  protected readonly error = signal(false);

  private readonly request = computed(() => ({ projectId: this.projectId(), page: this.page() }));

  private readonly page$: Observable<BuildsPage | null> = toObservable(this.request).pipe(
    switchMap(({ projectId, page }) => {
      this.loading.set(true);
      this.error.set(false);
      return this.api.builds(projectId, { page }).pipe(
        catchError(() => {
          this.error.set(true);
          return of<BuildsPage | null>(null);
        }),
      );
    }),
  );

  protected readonly data = toSignal(this.page$, { initialValue: null });

  constructor() {
    // Whenever a fresh response (or an error) lands, drop the loading gate.
    effect(() => {
      this.data();
      this.error();
      this.loading.set(false);
    });
  }

  protected readonly items = computed(() => this.data()?.items ?? []);
  protected readonly totalPages = computed(() => this.data()?.totalPages ?? 1);
  protected readonly isEmpty = computed(
    () => !this.loading() && !this.error() && this.items().length === 0,
  );

  /** Stable skeleton row placeholders for the loading state. */
  protected readonly skeletonRows = Array.from({ length: 7 }, (_, i) => i);

  /** Page numbers to render in the pager (1..totalPages). */
  protected readonly pages = computed(() => {
    const total = this.totalPages();
    return Array.from({ length: total }, (_, i) => i + 1);
  });

  protected goTo(p: number): void {
    if (p < 1 || p > this.totalPages() || p === this.page()) return;
    this.page.set(p);
  }

  // ---- click-to-copy commit hash ----
  protected readonly copied = signal<string | null>(null);

  protected copyHash(event: Event, sha: string): void {
    event.stopPropagation();
    event.preventDefault();
    void navigator.clipboard?.writeText(sha);
    this.copied.set(sha);
    setTimeout(() => {
      if (this.copied() === sha) this.copied.set(null);
    }, 900);
  }

  protected shortSha(sha: string): string {
    return sha.slice(0, 7);
  }

  /** Real run duration (finalizedAt − createdAt) as "2м 14с"; null while the build is still running. */
  protected duration(b: BuildListItem): string | null {
    if (!b.finalizedAt) return null;
    const ms = new Date(b.finalizedAt).getTime() - new Date(b.createdAt).getTime();
    if (!Number.isFinite(ms) || ms < 0) return null;
    const sec = Math.round(ms / 1000);
    const min = Math.floor(sec / 60);
    return min > 0 ? `${min}м ${sec % 60}с` : `${sec}с`;
  }

  /** Russian relative time ("8 мин", "1 ч", "вчера", "2 дн", "сейчас"). */
  protected relTime(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return '';
    const diff = Date.now() - then;
    const sec = Math.round(diff / 1000);
    if (sec < 45) return 'сейчас';
    const min = Math.round(sec / 60);
    if (min < 60) return `${min} мин`;
    const hr = Math.round(min / 60);
    if (hr < 24) return `${hr} ч`;
    const day = Math.round(hr / 24);
    if (day === 1) return 'вчера';
    if (day < 30) return `${day} дн`;
    const mon = Math.round(day / 30);
    if (mon < 12) return `${mon} мес`;
    return `${Math.round(mon / 12)} г`;
  }
}
