import { NgTemplateOutlet } from '@angular/common';
import { ChangeDetectionStrategy, Component, computed, inject, input } from '@angular/core';
import { Router, RouterLink } from '@angular/router';
import { SessionService } from '../core/session';

interface NavItem {
  key: string;
  label: string;
  link?: string[];
  count?: number | null;
}

/**
 * AppShell is the shared dashboard chrome (sidebar + topbar), ported from app-shell.css. Two modes:
 * `org` (Проекты / Участники / Активность / Настройки) and `project` (Сборки / Снимки / Базовые линии /
 * Настройки). Only routes backed by a real endpoint carry a routerLink today; the rest are visible but
 * inert until their phase lands. The page body is projected via <ng-content>; topbar actions via
 * <ng-content select="[actions]">.
 */
@Component({
  selector: 'px-app-shell',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [RouterLink, NgTemplateOutlet],
  templateUrl: './app-shell.html',
})
export class AppShell {
  private readonly session = inject(SessionService);
  private readonly router = inject(Router);

  readonly mode = input<'org' | 'project'>('org');
  readonly title = input.required<string>();
  readonly subtitle = input<string>('');
  readonly active = input<string>('');
  readonly projectName = input<string>('');
  readonly projectId = input<string>('');

  protected readonly user = this.session.user;

  protected readonly nav = computed<NavItem[]>(() => {
    if (this.mode() === 'project') {
      const pid = this.projectId();
      return [
        { key: 'builds', label: 'Сборки', link: pid ? ['/projects', pid, 'builds'] : undefined },
        { key: 'snapshots', label: 'Снимки' },
        { key: 'baselines', label: 'Базовые линии' },
        { key: 'settings', label: 'Настройки' },
      ];
    }
    return [
      { key: 'projects', label: 'Проекты', link: ['/'] },
      { key: 'members', label: 'Участники' },
      { key: 'activity', label: 'Активность' },
      { key: 'settings', label: 'Настройки' },
    ];
  });

  protected initials(seed: string | null | undefined): string {
    const s = (seed ?? '').trim();
    if (!s) return '??';
    const parts = s.split(/[\s@.]+/).filter(Boolean);
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
    return s.slice(0, 2).toUpperCase();
  }

  protected async logout(): Promise<void> {
    await this.session.logout();
    await this.router.navigate(['/login']);
  }
}
