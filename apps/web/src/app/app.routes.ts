import { Routes } from '@angular/router';
import { authGuard, guestGuard } from './core/auth.guard';

export const routes: Routes = [
  {
    path: 'login',
    canActivate: [guestGuard],
    loadComponent: () => import('./pages/login/login').then((m) => m.Login),
  },
  {
    path: '',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/projects/projects').then((m) => m.Projects),
  },
  {
    path: 'projects/:projectId/builds',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/builds/builds').then((m) => m.Builds),
  },
  {
    path: 'projects/:projectId/members',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/members/members').then((m) => m.Members),
  },
  {
    path: 'projects/:projectId/baselines',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/baselines/baselines').then((m) => m.Baselines),
  },
  {
    path: 'activity',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/activity/activity').then((m) => m.Activity),
  },
  {
    path: 'builds/:buildId',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/build-detail/build-detail').then((m) => m.BuildDetail),
  },
  {
    path: 'snapshots/:snapshotId',
    canActivate: [authGuard],
    loadComponent: () => import('./pages/review/review').then((m) => m.Review),
  },
  { path: '**', redirectTo: '' },
];
