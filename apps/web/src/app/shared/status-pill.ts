import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';

/**
 * Status is always icon + label + colour (never colour alone — accessibility, design rule). Covers both
 * build statuses (RUNNING/COMPARING/PASSED/REVIEW_REQUIRED/REJECTED/ERROR) and snapshot statuses
 * (UNCHANGED/CHANGED/NEW/REMOVED/APPROVED/REJECTED/ERROR), since the design renders them identically.
 *
 * SVGs are inlined in the template (Angular renders SVG natively); going through [innerHTML] would let
 * the HTML sanitizer strip the <svg>.
 */
@Component({
  selector: 'px-status',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <span class="status status--{{ mod() }}">
      @switch (status()) {
        @case ('REVIEW_REQUIRED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7z" />
            <circle cx="12" cy="12" r="3" />
          </svg>
        }
        @case ('PASSED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
            <path d="M20 6 9 17l-5-5" />
          </svg>
        }
        @case ('APPROVED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
            <path d="M20 6 9 17l-5-5" />
          </svg>
        }
        @case ('REJECTED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
            <path d="M18 6 6 18M6 6l12 12" />
          </svg>
        }
        @case ('ERROR') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M12 9v4M12 17h.01" />
            <path
              d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z"
            />
          </svg>
        }
        @case ('COMPARING') {
          <svg
            class="spin"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
          >
            <path d="M21 12a9 9 0 1 1-6.2-8.5" />
          </svg>
        }
        @case ('RUNNING') {
          <svg
            class="spin"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.2"
          >
            <path d="M21 12a9 9 0 1 1-6.2-8.5" />
          </svg>
        }
        @case ('CHANGED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2">
            <path d="M12 3v18M3 7.5h7M3 16.5h7M21 7.5h-7M21 16.5h-7" />
          </svg>
        }
        @case ('NEW') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
            <path d="M12 5v14M5 12h14" />
          </svg>
        }
        @case ('REMOVED') {
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4">
            <path d="M5 12h14" />
          </svg>
        }
        @default {
          <svg viewBox="0 0 24 24" fill="currentColor"><circle cx="12" cy="12" r="4" /></svg>
        }
      }
      {{ label() }}
    </span>
  `,
})
export class StatusPill {
  readonly status = input.required<string>();
  protected readonly mod = computed(() => MODS[this.status()] ?? 'unchanged');
  protected readonly label = computed(() => LABELS[this.status()] ?? '—');
}

const MODS: Record<string, string> = {
  REVIEW_REQUIRED: 'review',
  PASSED: 'passed',
  REJECTED: 'rejected',
  ERROR: 'error',
  COMPARING: 'comparing',
  RUNNING: 'running',
  UNCHANGED: 'unchanged',
  CHANGED: 'changed',
  NEW: 'new',
  REMOVED: 'removed',
  APPROVED: 'approved',
};

const LABELS: Record<string, string> = {
  REVIEW_REQUIRED: 'Нужна проверка',
  PASSED: 'Пройдено',
  REJECTED: 'Отклонено',
  ERROR: 'Ошибка',
  COMPARING: 'Сравнение',
  RUNNING: 'Выполняется',
  UNCHANGED: 'Без изменений',
  CHANGED: 'Изменено',
  NEW: 'Новый',
  REMOVED: 'Удалён',
  APPROVED: 'Одобрено',
};
