import assert from 'node:assert/strict';
import { test } from 'node:test';

import { isGitlabCi, resolveBuildContext, type EnvLike } from './ci';

const noGit = (): string | undefined => undefined;

test('resolveBuildContext reads GitLab CI env vars', () => {
  const env: EnvLike = {
    GITLAB_CI: 'true',
    CI_COMMIT_REF_NAME: 'feature/seatmap',
    CI_COMMIT_SHA: 'c3a1f9e0',
    CI_PIPELINE_ID: '12345',
    CI_JOB_URL: 'https://gitlab.example.com/jobs/678',
    CI_MERGE_REQUEST_IID: '42',
    CI_NODE_TOTAL: '4',
  };
  const ctx = resolveBuildContext({}, env, noGit);
  assert.equal(ctx.branch, 'feature/seatmap');
  assert.equal(ctx.commitSha, 'c3a1f9e0');
  assert.equal(ctx.ciBuildId, '12345');
  assert.equal(ctx.ciJobUrl, 'https://gitlab.example.com/jobs/678');
  assert.equal(ctx.mrIid, '42');
  assert.equal(ctx.parallelTotal, 4);
});

test('explicit overrides win over CI env', () => {
  const env: EnvLike = { CI_COMMIT_REF_NAME: 'ci-branch', CI_PIPELINE_ID: '999' };
  const ctx = resolveBuildContext(
    { branch: 'override-branch', ciBuildId: 'override-build', parallelTotal: 2 },
    env,
    noGit,
  );
  assert.equal(ctx.branch, 'override-branch');
  assert.equal(ctx.ciBuildId, 'override-build');
  assert.equal(ctx.parallelTotal, 2);
});

test('local run falls back to git then to a unique build id', () => {
  const env: EnvLike = {};
  const gitRunner = (args: readonly string[]): string | undefined => {
    if (args.includes('--abbrev-ref')) return 'local-branch';
    if (args[0] === 'rev-parse') return 'deadbeefcafe';
    return undefined;
  };
  const ctx = resolveBuildContext({}, env, gitRunner);
  assert.equal(ctx.branch, 'local-branch');
  assert.equal(ctx.commitSha, 'deadbeefcafe');
  assert.equal(ctx.parallelTotal, 1);
  // Unique local id so local screenshots never aggregate into a foreign CI build.
  assert.match(ctx.ciBuildId, /^local-/);
});

test('two local runs get distinct build ids', () => {
  const a = resolveBuildContext({}, {}, noGit);
  const b = resolveBuildContext({}, {}, noGit);
  // The timestamp component makes collisions effectively impossible across calls.
  assert.notEqual(a.ciBuildId, b.ciBuildId);
});

test('parallelTotal ignores non-positive CI_NODE_TOTAL', () => {
  assert.equal(resolveBuildContext({}, { CI_NODE_TOTAL: '0' }, noGit).parallelTotal, 1);
  assert.equal(resolveBuildContext({}, { CI_NODE_TOTAL: 'x' }, noGit).parallelTotal, 1);
});

test('isGitlabCi reflects the GITLAB_CI marker', () => {
  assert.equal(isGitlabCi({ GITLAB_CI: 'true' }), true);
  assert.equal(isGitlabCi({}), false);
  assert.equal(isGitlabCi({ GITLAB_CI: '' }), false);
});
