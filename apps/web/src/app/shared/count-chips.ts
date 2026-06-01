import { ChangeDetectionStrategy, Component, computed, input } from '@angular/core';
import type { Counts } from '@pixela/shared';

/** Per-status snapshot count chips for a build (only non-zero buckets render; em-dash when all zero). */
@Component({
  selector: 'px-count-chips',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="counts">
      @if (any()) {
        @if (c().unchanged) {
          <span class="chip chip--ok"
            ><span class="dot"></span><b>{{ c().unchanged }}</b></span
          >
        }
        @if (c().changed) {
          <span class="chip chip--changed"
            ><span class="dot"></span><b>{{ c().changed }}</b> изм.</span
          >
        }
        @if (c().new) {
          <span class="chip chip--new"
            ><span class="dot"></span><b>{{ c().new }}</b> нов.</span
          >
        }
        @if (c().removed) {
          <span class="chip chip--removed"
            ><span class="dot"></span><b>{{ c().removed }}</b> уд.</span
          >
        }
      } @else {
        <span class="muted">—</span>
      }
    </div>
  `,
  styles: [
    `
      .counts {
        display: flex;
        align-items: center;
        gap: 6px;
        flex-wrap: nowrap;
        overflow: hidden;
      }
      .muted {
        font-size: 12px;
        color: var(--text-3);
      }
    `,
  ],
})
export class CountChips {
  readonly counts = input.required<Counts>();
  protected readonly c = computed(() => this.counts());
  protected readonly any = computed(() => {
    const v = this.counts();
    return !!(v.unchanged || v.changed || v.new || v.removed);
  });
}
