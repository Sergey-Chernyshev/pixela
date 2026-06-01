import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
import { toObservable, toSignal } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { catchError, firstValueFrom, map, of, startWith, switchMap } from 'rxjs';
import type { BuildDetail as BuildDetailDto, SnapshotBrief } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';
import { StatusPill } from '../../shared/status-pill';

/** Snapshot statuses that represent a visual difference (what "только изменённые" keeps). */
const DIFF_STATUSES = new Set(['CHANGED', 'NEW', 'REMOVED']);

interface LoadState {
  loading: boolean;
  error: boolean;
  data: BuildDetailDto | null;
}

/**
 * BuildDetail — the snapshot grid for one build. Header shows branch, commit hash, status and createdAt;
 * an aggregate count summary (computed from the snapshots, since BuildDetail itself carries no Counts);
 * a "только изменённые" toggle (default ON) filtering to CHANGED/NEW/REMOVED; and a grid of status-forward
 * snapshot cards, each linking to its review.
 *
 * Cards render real presigned thumbnails (new/diff/baseline) from the API. Batch approve/reject (Phase 5)
 * acts on every reviewable snapshot and refetches the build. The mock's author/duration/CI-link header
 * meta have no backing API field and are omitted.
 */
@Component({
  selector: 'px-build-detail',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, AppShell, StatusPill],
  templateUrl: './build-detail.html',
  styleUrl: './build-detail.scss',
})
export class BuildDetail {
  private readonly api = inject(ApiService);

  readonly buildId = input.required<string>();

  /** Bumped after a batch approve/reject to refetch the build (statuses + counts change). */
  private readonly reload = signal(0);
  private readonly trigger = computed(() => ({ id: this.buildId(), nonce: this.reload() }));

  /** Build load: loading/error/data trio, default data = null until resolved (loading-state modelling). */
  private readonly state = toSignal(
    toObservable(this.trigger).pipe(
      switchMap(({ id }) =>
        this.api.build(id).pipe(
          map((data): LoadState => ({ loading: false, error: false, data })),
          startWith({ loading: true, error: false, data: null } as LoadState),
          catchError(() => of({ loading: false, error: true, data: null } as LoadState)),
        ),
      ),
    ),
    { initialValue: { loading: true, error: false, data: null } as LoadState },
  );

  protected readonly loading = computed(() => this.state().loading);
  protected readonly error = computed(() => this.state().error);
  protected readonly build = computed(() => this.state().data);

  /** "Только изменённые" — ON by default (mock default). */
  protected readonly onlyChanged = signal(true);

  private readonly snapshots = computed<SnapshotBrief[]>(() => this.build()?.snapshots ?? []);

  /** Aggregate per-status counts, derived from the snapshots (BuildDetail has no Counts of its own). */
  protected readonly counts = computed(() => {
    const acc = { unchanged: 0, changed: 0, new: 0, removed: 0 };
    for (const s of this.snapshots()) {
      switch (s.status) {
        case 'UNCHANGED':
        case 'APPROVED':
          acc.unchanged++;
          break;
        case 'CHANGED':
          acc.changed++;
          break;
        case 'NEW':
          acc.new++;
          break;
        case 'REMOVED':
          acc.removed++;
          break;
        default:
          break;
      }
    }
    return acc;
  });

  /** Whether any count bucket is non-zero (drives the em-dash fallback in the summary). */
  protected readonly anyCounts = computed(() => {
    const c = this.counts();
    return !!(c.unchanged || c.changed || c.new || c.removed);
  });

  /** Visible cards: when the toggle is on, only diffing statuses; otherwise everything. */
  protected readonly visible = computed<SnapshotBrief[]>(() => {
    const all = this.snapshots();
    return this.onlyChanged() ? all.filter((s) => DIFF_STATUSES.has(s.status)) : all;
  });

  protected readonly isEmpty = computed(() => this.visible().length === 0);

  /** Snapshots a reviewer can act on (drives the batch approve/reject buttons). */
  protected readonly reviewableCount = computed(
    () => this.snapshots().filter((s) => DIFF_STATUSES.has(s.status) || s.status === 'ERROR').length,
  );

  // ---- batch review ----
  protected readonly submitting = signal(false);
  protected readonly note = signal<string | null>(null);
  private noteTimer: ReturnType<typeof setTimeout> | null = null;

  protected approveAll(): void {
    void this.reviewAll('approve');
  }
  protected rejectAll(): void {
    void this.reviewAll('reject');
  }

  /** Pessimistic batch action: call the API, then refetch the build so statuses/counts update. */
  private async reviewAll(action: 'approve' | 'reject'): Promise<void> {
    if (this.submitting() || this.reviewableCount() === 0) return;
    this.submitting.set(true);
    this.note.set(null);
    try {
      const call =
        action === 'approve'
          ? this.api.approveBuild(this.buildId())
          : this.api.rejectBuild(this.buildId());
      const res = await firstValueFrom(call);
      this.flash(
        action === 'approve'
          ? `Принято снимков: ${res.affected}`
          : `Отклонено снимков: ${res.affected}`,
      );
      this.reload.update((v) => v + 1); // refetch the build
    } catch {
      this.flash('Не удалось применить действие');
    } finally {
      this.submitting.set(false);
    }
  }

  private flash(msg: string): void {
    this.note.set(msg);
    if (this.noteTimer) clearTimeout(this.noteTimer);
    this.noteTimer = setTimeout(() => this.note.set(null), 2600);
  }

  protected toggleOnlyChanged(): void {
    this.onlyChanged.update((v) => !v);
  }

  /** The thumbnail to show: the new capture when present (changed/new/unchanged), else the baseline
   *  (removed). Returns null only when neither blob exists. Presigned by the server. */
  protected thumbUrl(snap: SnapshotBrief): string | null {
    return snap.images?.new ?? snap.images?.baseline ?? null;
  }

  /** The diff overlay PNG, shown only on CHANGED cards (aligns over the new capture). */
  protected diffOverlay(snap: SnapshotBrief): string | null {
    return snap.status === 'CHANGED' ? (snap.images?.diff ?? null) : null;
  }

  /** Status modifier class for the thumbnail (grayscale removed, dim unchanged — mirrors the mock). */
  protected thumbClass(snap: SnapshotBrief): string {
    return `thumb--${snap.status.toLowerCase()}`;
  }

  /** diffRatio (0..1) → percentage string, only when present. */
  protected pct(snap: SnapshotBrief): string | null {
    if (snap.diffRatio == null) return null;
    return `${(snap.diffRatio * 100).toFixed(2)}%`;
  }

  /** Maps a snapshot status to the diff-colour class for its diff%, matching the mock. */
  protected pctClass(snap: SnapshotBrief): string {
    if (snap.status === 'NEW') return 'pct--new';
    if (snap.status === 'REMOVED') return 'pct--removed';
    return 'pct--changed';
  }

  protected when(iso: string | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleString('ru-RU', {
      day: 'numeric',
      month: 'short',
      hour: '2-digit',
      minute: '2-digit',
    });
  }
}
