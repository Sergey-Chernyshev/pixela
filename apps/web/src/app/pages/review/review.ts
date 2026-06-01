import {
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  HostListener,
  computed,
  effect,
  inject,
  input,
  signal,
  viewChild,
} from '@angular/core';
import { DecimalPipe, Location } from '@angular/common';
import { firstValueFrom } from 'rxjs';
import type { SnapshotReview } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { StatusPill } from '../../shared/status-pill';

/**
 * Review — the visual-diff workspace and the face of the product. A focused full-screen view (no
 * app-shell chrome): topbar (breadcrumb back, snapshot identity, mode segmented control, zoom controls,
 * approve/reject) + comparison stage + metadata side panel with the approval history.
 *
 * Four comparison modes (signal `mode`): side-by-side, overlay (diff PNG composited over the new shot),
 * onion (opacity cross-fade baseline↔new), curtain (draggable reveal). A single `zoom` + synced scroll
 * applies identically to both panes — synchronous zoom is the non-negotiable F-26 requirement.
 *
 * The design mock carried rich data the API does not return (author full names + avatar colours, flaky
 * flag, threshold, image pixel dimensions, change-progress nav, the descriptive history text). Those are
 * honestly omitted; only API-backed fields render. Approve/Reject have no endpoint yet (Phase 5) — the
 * buttons stay present and fire a transient inline note instead. See apiGapsHandled.
 *
 * State is the loading/error/data trio with `data` defaulting to null (not {}), so the template tells
 * "not loaded" (skeleton) apart from "loaded" — loading-state modelling.
 */
type Mode = 'side' | 'overlay' | 'onion' | 'curtain';

@Component({
  selector: 'px-review',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [StatusPill, DecimalPipe],
  templateUrl: './review.html',
  styleUrl: './review.scss',
})
export class Review {
  private readonly api = inject(ApiService);
  private readonly location = inject(Location);

  /** Route param `snapshots/:snapshotId`, auto-bound via withComponentInputBinding. */
  readonly snapshotId = input.required<string>();

  protected readonly loading = signal(true);
  protected readonly error = signal<string | null>(null);
  protected readonly data = signal<SnapshotReview | null>(null);

  /** Comparison mode + the optional diff-overlay toggle (independent of mode, like the mock). */
  protected readonly mode = signal<Mode>('side');
  protected readonly showDiff = signal(false);

  /** Synced zoom: 0 = "fit" sentinel; >0 = explicit scale applied identically to both panes (F-26). */
  protected readonly zoom = signal(0);
  /** Onion opacity (0 = baseline, 1 = new) and curtain reveal position (0..1). */
  protected readonly onion = signal(0.5);
  protected readonly curtain = signal(0.5);

  /** Transient confirmation/error note shown after a review action. */
  protected readonly note = signal<string | null>(null);
  private noteTimer: ReturnType<typeof setTimeout> | null = null;

  /** In-flight review action (disables the controls; pessimistic — no optimistic status flip). */
  protected readonly submitting = signal(false);
  /** Whether the snapshot is still actionable (resolved snapshots disable approve/reject). */
  protected readonly reviewable = computed(() => {
    const s = this.data()?.status;
    return s === 'CHANGED' || s === 'NEW' || s === 'REMOVED';
  });

  private readonly stage = viewChild<ElementRef<HTMLElement>>('stage');
  private readonly curtainStack = viewChild<ElementRef<HTMLElement>>('curtainStack');

  // ---- derived view-model ----
  protected readonly images = computed(() => this.data()?.images ?? null);
  protected readonly hasBaseline = computed(() => !!this.images()?.baseline);
  protected readonly hasNew = computed(() => !!this.images()?.new);
  protected readonly hasDiff = computed(() => !!this.images()?.diff);

  /** Diff ratio as a percentage string (API gives a 0..1 double); null when absent. */
  protected readonly ratioPct = computed(() => {
    const r = this.data()?.diffRatio;
    return typeof r === 'number' ? (r * 100).toFixed(2) : null;
  });
  /** Bar fill width clamped to 0..100 (%). */
  protected readonly ratioBar = computed(() => {
    const r = this.data()?.diffRatio;
    return typeof r === 'number' ? Math.min(100, Math.max(0, r * 100)) : 0;
  });
  protected readonly history = computed(() => this.data()?.history ?? []);

  /** Zoom label: "Fit" while in the fit sentinel, else a rounded percentage. */
  protected readonly zoomLabel = computed(() => {
    const z = this.zoom();
    return z > 0 ? `${Math.round(z * 100)}%` : 'Fit';
  });

  /** Explicit scale to apply, resolving the fit sentinel against the stage width. */
  protected readonly scale = computed(() => {
    const z = this.zoom();
    if (z > 0) return z;
    return this.fitScale();
  });

  /** Re-evaluate fit when the stage resizes; a signal bumped from the resize listener. */
  private readonly stageW = signal(0);

  protected readonly skeletonHist = [0, 1, 2];

  constructor() {
    void this.load();
    // Reset transient view state whenever a different snapshot is opened.
    effect(() => {
      this.snapshotId();
      this.zoom.set(0);
      this.onion.set(0.5);
      this.curtain.set(0.5);
      this.showDiff.set(false);
    });
  }

  private async load(): Promise<void> {
    this.loading.set(true);
    this.error.set(null);
    try {
      const res = await firstValueFrom(this.api.snapshot(this.snapshotId()));
      this.data.set(res);
      // Default to a sensible mode: if there is no baseline (e.g. a NEW snapshot) overlay/curtain are
      // meaningless, so stay on side-by-side which renders the placeholder gracefully.
      this.mode.set('side');
    } catch {
      this.error.set('Не удалось загрузить снимок');
      this.data.set(null);
    } finally {
      this.loading.set(false);
    }
  }

  // ---- mode / diff ----
  protected setMode(m: Mode): void {
    this.mode.set(m);
  }
  protected toggleDiff(): void {
    this.showDiff.update((v) => !v);
  }

  // ---- zoom ----
  private fitScale(): number {
    const stageEl = this.stage()?.nativeElement;
    const w = stageEl?.clientWidth ?? this.stageW();
    if (!w) return 1;
    // Side-by-side splits the stage in two; other modes use the full width.
    const paneW = this.mode() === 'side' ? w / 2 : w;
    // 56px padding budget mirrors the mock's .pane-pad / .onion-pad gutters.
    const natural = this.naturalWidth();
    return Math.max(0.1, (paneW - 56) / natural);
  }

  /** A reference natural width for fit math. Real pixel dims are not in the API, so use a stable base. */
  private naturalWidth(): number {
    return 960;
  }

  protected zoomIn(): void {
    const cur = this.scale();
    this.zoom.set(Math.min(3, cur * 1.25));
  }
  protected zoomOut(): void {
    const cur = this.scale();
    this.zoom.set(Math.max(0.15, cur / 1.25));
  }
  protected fit(): void {
    this.zoom.set(0);
  }
  protected oneToOne(): void {
    this.zoom.set(1);
  }

  /**
   * Wheel over the stage zooms the synced view (F-26: one zoom level for both panes). Shift+wheel is
   * left to the browser so the user can still pan horizontally inside an overflowing pane.
   */
  protected onWheel(e: WheelEvent): void {
    if (e.shiftKey) return;
    e.preventDefault();
    const cur = this.scale();
    const factor = e.deltaY < 0 ? 1.12 : 1 / 1.12;
    this.zoom.set(Math.min(3, Math.max(0.15, cur * factor)));
  }

  // ---- onion / curtain inputs ----
  protected onOnion(e: Event): void {
    const v = Number((e.target as HTMLInputElement).value) / 100;
    this.onion.set(v);
  }

  private curtainDrag = false;
  protected startCurtain(e: PointerEvent): void {
    this.curtainDrag = true;
    (e.target as HTMLElement).setPointerCapture?.(e.pointerId);
    e.preventDefault();
    this.updateCurtain(e);
  }
  @HostListener('window:pointermove', ['$event'])
  protected moveCurtain(e: PointerEvent): void {
    if (!this.curtainDrag) return;
    this.updateCurtain(e);
  }
  @HostListener('window:pointerup')
  protected endCurtain(): void {
    this.curtainDrag = false;
  }
  private updateCurtain(e: PointerEvent): void {
    const el = this.curtainStack()?.nativeElement;
    if (!el) return;
    const r = el.getBoundingClientRect();
    this.curtain.set(Math.max(0, Math.min(1, (e.clientX - r.left) / r.width)));
  }

  // ---- review actions ----
  protected approve(): void {
    void this.act('approve');
  }
  protected reject(): void {
    void this.act('reject');
  }

  /** Pessimistic approve/reject: call the API, and only on success reflect the new snapshot status. */
  private async act(action: 'approve' | 'reject'): Promise<void> {
    if (this.submitting() || !this.reviewable()) return;
    this.submitting.set(true);
    this.note.set(null);
    try {
      const call =
        action === 'approve'
          ? this.api.approveSnapshot(this.snapshotId())
          : this.api.rejectSnapshot(this.snapshotId());
      const res = await firstValueFrom(call);
      const current = this.data();
      if (current) {
        this.data.set({ ...current, status: action === 'approve' ? 'APPROVED' : 'REJECTED' });
      }
      this.flash(
        action === 'approve'
          ? `Принято — эталон обновлён · сборка ${this.buildStatusRu(res.buildStatus)}`
          : `Отклонено · сборка ${this.buildStatusRu(res.buildStatus)}`,
      );
    } catch {
      this.flash(action === 'approve' ? 'Не удалось принять снимок' : 'Не удалось отклонить снимок');
    } finally {
      this.submitting.set(false);
    }
  }

  private buildStatusRu(s: string): string {
    const map: Record<string, string> = {
      PASSED: 'пройдена',
      REVIEW_REQUIRED: 'на проверке',
      REJECTED: 'отклонена',
      COMPARING: 'сравнение',
      RUNNING: 'выполняется',
      ERROR: 'ошибка',
    };
    return map[s] ?? s;
  }

  protected back(): void {
    this.location.back();
  }

  private flash(msg: string): void {
    this.note.set(msg);
    if (this.noteTimer) clearTimeout(this.noteTimer);
    this.noteTimer = setTimeout(() => this.note.set(null), 2600);
  }

  // ---- keyboard: A approve, R reject, 1..4 modes ----
  @HostListener('window:keydown', ['$event'])
  protected onKey(e: KeyboardEvent): void {
    const target = e.target as HTMLElement | null;
    if (target && (target.tagName === 'INPUT' || target.tagName === 'TEXTAREA')) return;
    switch (e.key) {
      case 'a':
      case 'A':
      case 'ф':
      case 'Ф':
        this.approve();
        break;
      case 'r':
      case 'R':
      case 'к':
      case 'К':
        this.reject();
        break;
      case '1':
        this.setMode('side');
        break;
      case '2':
        this.setMode('overlay');
        break;
      case '3':
        this.setMode('onion');
        break;
      case '4':
        this.setMode('curtain');
        break;
      case 'Escape':
        this.back();
        break;
      default:
        return;
    }
  }

  @HostListener('window:resize')
  protected onResize(): void {
    const w = this.stage()?.nativeElement.clientWidth ?? 0;
    this.stageW.set(w);
  }

  // ---- history rendering helpers (only API fields: action, user email, at) ----
  protected actionLabel(action: string): string {
    if (action === 'APPROVE' || action === 'APPROVED') return 'одобрил снимок';
    if (action === 'REJECT' || action === 'REJECTED') return 'отклонил снимок';
    return action.toLowerCase();
  }

  protected actionMod(action: string): string {
    if (action === 'APPROVE' || action === 'APPROVED') return 'ok';
    if (action === 'REJECT' || action === 'REJECTED') return 'rej';
    return 'neutral';
  }

  /** Avatar initials derived from the real email (no fabricated display name). */
  protected initials(email: string): string {
    const handle = email.split('@')[0] ?? email;
    const parts = handle.split(/[._-]+/).filter(Boolean);
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
    return handle.slice(0, 2).toUpperCase();
  }

  /** Relative time in Russian from the ISO `at` (real field) via Intl.RelativeTimeFormat. */
  protected relative(iso: string): string {
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return '';
    const diff = Date.now() - then;
    const sec = Math.round(diff / 1000);
    const rtf = new Intl.RelativeTimeFormat('ru', { numeric: 'auto', style: 'short' });
    if (sec < 60) return rtf.format(-sec, 'second');
    const min = Math.round(sec / 60);
    if (min < 60) return rtf.format(-min, 'minute');
    const hr = Math.round(min / 60);
    if (hr < 24) return rtf.format(-hr, 'hour');
    const day = Math.round(hr / 24);
    if (day < 30) return rtf.format(-day, 'day');
    const mon = Math.round(day / 30);
    if (mon < 12) return rtf.format(-mon, 'month');
    return rtf.format(-Math.round(mon / 12), 'year');
  }
}
