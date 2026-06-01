import {
  ChangeDetectionStrategy,
  Component,
  computed,
  effect,
  inject,
  input,
  signal,
} from '@angular/core';
import { type Observable, catchError, of, switchMap } from 'rxjs';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import type { BaselineList, BaselineView, ProjectList } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';

/** Baselines older than this are flagged "устарела" (computed straight from updatedAt). */
const STALE_DAYS = 90;
const DAY_MS = 86_400_000;

/**
 * Baselines — the project's accepted reference snapshots per branch (design: project/baselines.html).
 * A responsive grid of cards: a thumbnail (presigned imageUrl, placeholder when absent), the snapshot
 * name, its branch chip, browser·viewport, who approved it, and a relative "обновлён" age. Staleness
 * (>90 days since the last accept) is a real derivation from updatedAt, so the mock's amber "устарела"
 * highlight is kept. The summary strip likewise carries only real aggregates computed from the response
 * (total, distinct branches, updated-this-week, stale). Loads via ApiService with the loading/error/data
 * trio (data defaults to null; the skeleton holds the layout until the first response resolves).
 *
 * The mock's interactive branch tabs, search box, "сравнить ветки" action, the segmented Все/Недавние/
 * Устаревшие filter, and the per-card history/reset buttons have no backing endpoint and are omitted
 * rather than faked. Avatar colours/initials in the mock are decorative — only the real approver email
 * is rendered (mono), or "—" when the API omits approvedBy.
 */
@Component({
  selector: 'px-baselines',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [AppShell],
  templateUrl: './baselines.html',
  styleUrl: './baselines.scss',
})
export class Baselines {
  private readonly api = inject(ApiService);

  /** Route param (auto-bound by withComponentInputBinding). */
  readonly projectId = input.required<string>();

  /** Project name for the shell header — resolved from the projects list, falls back to the id. */
  private readonly projects$: Observable<ProjectList | null> = this.api
    .projects()
    .pipe(catchError(() => of<ProjectList | null>(null)));
  private readonly projects = toSignal(this.projects$, { initialValue: null });
  protected readonly projectName = computed(() => {
    const id = this.projectId();
    const list = this.projects()?.projects ?? [];
    return list.find((p) => p.id === id)?.name ?? id;
  });

  // ---- baselines (loading / error / data trio) ----
  protected readonly loading = signal(true);
  protected readonly error = signal(false);

  private readonly data$: Observable<BaselineList | null> = toObservable(this.projectId).pipe(
    switchMap((projectId) => {
      this.loading.set(true);
      this.error.set(false);
      return this.api.baselines(projectId).pipe(
        catchError(() => {
          this.error.set(true);
          return of<BaselineList | null>(null);
        }),
      );
    }),
  );

  protected readonly data = toSignal(this.data$, { initialValue: null });

  constructor() {
    // Drop the loading gate as soon as a response (or an error) lands.
    effect(() => {
      this.data();
      this.error();
      this.loading.set(false);
    });
  }

  protected readonly items = computed<BaselineView[]>(() => this.data()?.baselines ?? []);
  protected readonly count = computed(() => this.items().length);
  protected readonly isEmpty = computed(
    () => !this.loading() && !this.error() && this.count() === 0,
  );

  /** Distinct branches present in the resolved baselines (real aggregate). */
  protected readonly branchCount = computed(() => new Set(this.items().map((b) => b.branch)).size);

  /** Baselines re-accepted in the last 7 days (real aggregate from updatedAt). */
  protected readonly updatedThisWeek = computed(() => {
    const cutoff = Date.now() - 7 * DAY_MS;
    return this.items().filter((b) => {
      const t = new Date(b.updatedAt).getTime();
      return Number.isFinite(t) && t >= cutoff;
    }).length;
  });

  /** Baselines older than STALE_DAYS since their last accept (real aggregate). */
  protected readonly staleCount = computed(
    () => this.items().filter((b) => this.isStale(b)).length,
  );

  /** Stable skeleton placeholders for the loading grid. */
  protected readonly skeletons = Array.from({ length: 8 }, (_, i) => i);

  /** True when the baseline hasn't been re-accepted in over STALE_DAYS. */
  protected isStale(b: BaselineView): boolean {
    const t = new Date(b.updatedAt).getTime();
    if (!Number.isFinite(t)) return false;
    return Date.now() - t > STALE_DAYS * DAY_MS;
  }

  /** Russian relative time ("сейчас", "8 мин", "1 ч", "вчера", "2 дн", "3 мес", "1 г"). */
  protected relTime(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return '';
    const sec = Math.round((Date.now() - then) / 1000);
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
