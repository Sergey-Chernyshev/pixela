import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { SessionService } from '../../core/session';

/**
 * Login — the unauthenticated entry. The design mocks a magic-link + SSO card; the Go backend uses
 * email + password (the SSO buttons are shown for parity but not yet wired to an OAuth backend). On
 * success the server sets the session cookie and we navigate to the originally-requested URL.
 */
@Component({
  selector: 'px-login',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [FormsModule],
  templateUrl: './login.html',
  styleUrl: './login.scss',
})
export class Login {
  private readonly session = inject(SessionService);
  private readonly router = inject(Router);
  private readonly route = inject(ActivatedRoute);

  protected email = '';
  protected password = '';
  protected readonly loading = signal(false);
  protected readonly error = signal<string | null>(null);

  protected async submit(): Promise<void> {
    if (this.loading() || !this.email || !this.password) return;
    this.loading.set(true);
    this.error.set(null);
    try {
      await this.session.login(this.email, this.password);
      const returnUrl = this.route.snapshot.queryParamMap.get('returnUrl') || '/';
      await this.router.navigateByUrl(returnUrl);
    } catch {
      this.error.set('Неверная почта или пароль');
    } finally {
      this.loading.set(false);
    }
  }
}
