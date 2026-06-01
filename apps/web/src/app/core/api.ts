import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, InjectionToken, inject } from '@angular/core';
import { Observable } from 'rxjs';
import type {
  ActivityList,
  BaselineList,
  BuildDetail,
  BuildsPage,
  LoginResponse,
  MemberList,
  ProjectList,
  ReviewResult,
  SnapshotReview,
  User,
} from '@pixela/shared';

/**
 * API_BASE is the dashboard API origin/prefix. It defaults to the relative `/api`, so the SPA talks to
 * the same origin it is served from (a reverse proxy / `ng serve` proxy forwards `/api` to the Go
 * backend). Relative + same-origin keeps the session cookie first-party — no CORS, no hardcoded host.
 */
export const API_BASE = new InjectionToken<string>('API_BASE', {
  providedIn: 'root',
  factory: () => '/api',
});

/**
 * ApiService is the single typed gateway to the Go dashboard API. Every DTO is the OpenAPI-generated
 * type from `@pixela/shared` (the contract is the source of truth). `withCredentials` is added globally
 * by the credentials interceptor, so the `pixela_session` cookie rides along automatically.
 */
@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly http = inject(HttpClient);
  private readonly base = inject(API_BASE);

  private url(path: string): string {
    return `${this.base}/v1${path}`;
  }

  // ---- auth ----
  login(email: string, password: string): Observable<LoginResponse> {
    return this.http.post<LoginResponse>(this.url('/auth/login'), { email, password });
  }

  logout(): Observable<unknown> {
    return this.http.post(this.url('/auth/logout'), {});
  }

  me(): Observable<User> {
    return this.http.get<User>(this.url('/auth/me'));
  }

  // ---- reads ----
  projects(): Observable<ProjectList> {
    return this.http.get<ProjectList>(this.url('/projects'));
  }

  builds(
    projectId: string,
    opts: { branch?: string; status?: string; page?: number } = {},
  ): Observable<BuildsPage> {
    let params = new HttpParams();
    if (opts.branch) params = params.set('branch', opts.branch);
    if (opts.status) params = params.set('status', opts.status);
    if (opts.page) params = params.set('page', String(opts.page));
    return this.http.get<BuildsPage>(
      this.url(`/projects/${encodeURIComponent(projectId)}/builds`),
      { params },
    );
  }

  build(buildId: string): Observable<BuildDetail> {
    return this.http.get<BuildDetail>(this.url(`/builds/${encodeURIComponent(buildId)}`));
  }

  snapshot(snapshotId: string): Observable<SnapshotReview> {
    return this.http.get<SnapshotReview>(this.url(`/snapshots/${encodeURIComponent(snapshotId)}`));
  }

  members(projectId: string): Observable<MemberList> {
    return this.http.get<MemberList>(
      this.url(`/projects/${encodeURIComponent(projectId)}/members`),
    );
  }

  baselines(projectId: string): Observable<BaselineList> {
    return this.http.get<BaselineList>(
      this.url(`/projects/${encodeURIComponent(projectId)}/baselines`),
    );
  }

  activity(): Observable<ActivityList> {
    return this.http.get<ActivityList>(this.url('/activity'));
  }

  // ---- review actions (Phase 5) ----
  approveSnapshot(snapshotId: string): Observable<ReviewResult> {
    return this.http.post<ReviewResult>(this.url(`/snapshots/${encodeURIComponent(snapshotId)}/approve`), {});
  }

  rejectSnapshot(snapshotId: string): Observable<ReviewResult> {
    return this.http.post<ReviewResult>(this.url(`/snapshots/${encodeURIComponent(snapshotId)}/reject`), {});
  }

  approveBuild(buildId: string): Observable<ReviewResult> {
    return this.http.post<ReviewResult>(this.url(`/builds/${encodeURIComponent(buildId)}/approve`), {});
  }

  rejectBuild(buildId: string): Observable<ReviewResult> {
    return this.http.post<ReviewResult>(this.url(`/builds/${encodeURIComponent(buildId)}/reject`), {});
  }
}
