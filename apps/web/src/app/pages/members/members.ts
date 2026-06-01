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
import type { Member, MemberList, ProjectList } from '@pixela/shared';
import { ApiService } from '../../core/api';
import { AppShell } from '../../layout/app-shell';

/**
 * Members — the project-level team roster (design: project/members.html). A table of everyone with
 * access to the project: avatar (initials) + name/email, a role badge (Владелец/Участник) and the
 * member's total review count. Loads from the API as a loading/error/data trio (loading-state
 * modelling: `data` defaults to null so the skeleton holds the layout until the first response
 * resolves; an empty array renders the empty-state, not the skeleton).
 *
 * The mock also shows per-project access tags, an approval-rate bar, an activity sparkline, a
 * last-active timestamp and four team-metric summary cards — none of those have a backing field on
 * the Member DTO (only id/email/name/role/totalReviews exist), so they are honestly omitted rather
 * than fabricated.
 */
@Component({
  selector: 'px-members',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [AppShell],
  templateUrl: './members.html',
  styleUrl: './members.scss',
})
export class Members {
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
    const found = list.find((p) => p.id === id);
    return found?.name ?? id;
  });

  // ---- members roster (loading / error / data trio) ----
  protected readonly loading = signal(true);
  protected readonly error = signal(false);

  private readonly list$: Observable<MemberList | null> = toObservable(this.projectId).pipe(
    switchMap((projectId) => {
      this.loading.set(true);
      this.error.set(false);
      return this.api.members(projectId).pipe(
        catchError(() => {
          this.error.set(true);
          return of<MemberList | null>(null);
        }),
      );
    }),
  );

  protected readonly data = toSignal(this.list$, { initialValue: null });

  constructor() {
    // Whenever a fresh response (or an error) lands, drop the loading gate.
    effect(() => {
      this.data();
      this.error();
      this.loading.set(false);
    });
  }

  protected readonly members = computed<Member[]>(() => this.data()?.members ?? []);
  protected readonly count = computed(() => this.members().length);
  protected readonly isEmpty = computed(
    () => !this.loading() && !this.error() && this.count() === 0,
  );

  /** Stable skeleton row placeholders for the loading state. */
  protected readonly skeletonRows = Array.from({ length: 6 }, (_, i) => i);

  /** Display name: the member's name when present, otherwise the email (no fabricated label). */
  protected displayName(m: Member): string {
    return m.name?.trim() || m.email;
  }

  /** Initials from name ("Мира Кан" → "МК") or email ("ci@acme.dev" → "CI") — never colour alone. */
  protected initials(m: Member): string {
    const seed = (m.name?.trim() || m.email).trim();
    if (!seed) return '??';
    const parts = seed.split(/[\s@._-]+/).filter(Boolean);
    if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase();
    return seed.slice(0, 2).toUpperCase();
  }

  /**
   * Role → role-badge classes (returned together so a single `[class]` binding keeps `.role`).
   * Only OWNER/MEMBER are issued by the API; any other value falls back to the neutral viewer style.
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

  protected isOwner(role: string): boolean {
    return role === 'OWNER';
  }

  /** Plural "участник/участника/участников" for the section count. */
  protected membersWord(n: number): string {
    const mod10 = n % 10;
    const mod100 = n % 100;
    if (mod10 === 1 && mod100 !== 11) return 'участник';
    if (mod10 >= 2 && mod10 <= 4 && (mod100 < 10 || mod100 >= 20)) return 'участника';
    return 'участников';
  }
}
