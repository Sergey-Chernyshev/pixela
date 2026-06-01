// Ready Workflow orchestration for verify-go: fan out one reviewer per dimension over the changed Go
// files, adversarially verify each finding, return confirmed issues by severity.
//
// Run via the Workflow tool: Workflow({ scriptPath: "<this file>", args: { repo, files, dimensions } })
//   args.repo       — absolute repo root (default: /Users/<you>/ticksly/pixela)
//   args.files      — array of changed .go file paths (relative to repo); required
//   args.dimensions — subset of dimension keys to run; default = all that look relevant
// Prereq: run scripts/gate.sh FIRST (gofmt/vet/golangci-lint/build/test -race) — don't spend agents on
// what tooling already catches.

export const meta = {
  name: 'verify-go-review',
  description: 'Independent multi-agent review of changed Pixela Go files against the conventions, with adversarial verification',
  phases: [{ title: 'Review' }, { title: 'Verify' }],
}

const repo = (args && args.repo) || '/Users/se.chernyshev/ticksly/pixela'
const files = (args && args.files) || []
const RULES = `${repo}/.claude/skills/verify-go/references/review-dimensions.md`
const BOOK = `${repo}/docs/architecture/go-backend.md`
const ALL = ['idioms', 'errors', 'concurrency', 'data', 'security', 'http', 'determinism']
const dims = (args && args.dimensions && args.dimensions.length ? args.dimensions : ALL).filter((d) => ALL.includes(d))

if (!files.length) return { error: 'no files provided in args.files' }

const FINDINGS = {
  type: 'object', additionalProperties: false,
  properties: {
    dimension: { type: 'string' },
    findings: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          title: { type: 'string' }, severity: { type: 'string', enum: ['blocker', 'high', 'medium', 'low', 'nit'] },
          file: { type: 'string' }, line: { type: 'string' }, rule: { type: 'string' },
          evidence: { type: 'string' }, fix: { type: 'string' },
        },
        required: ['title', 'severity', 'file', 'line', 'rule', 'evidence', 'fix'],
      },
    },
  },
  required: ['dimension', 'findings'],
}
const VERDICT = {
  type: 'object', additionalProperties: false,
  properties: { isReal: { type: 'boolean' }, severity: { type: 'string', enum: ['blocker', 'high', 'medium', 'low', 'nit'] }, reasoning: { type: 'string' } },
  required: ['isReal', 'severity', 'reasoning'],
}

const base =
  `Review Pixela Go code. Rulebook: ${BOOK}. Per-dimension checklist: ${RULES} (read YOUR dimension's section). ` +
  `Changed files to review (read them):\n${files.map((f) => `- ${repo}/${f}`).join('\n')}\n` +
  `Flag ONLY genuine violations of a stated rule — not taste, not later-phase stubs. Cite file:line and the ` +
  `go-backend.md section. An empty findings list is a good result. Do not invent issues.`

phase('Review')
const reviewed = await pipeline(
  dims,
  (d) => agent(`${base}\n\nDIMENSION: ${d}. Apply only the "${d}" checklist section.`, { label: `review:${d}`, phase: 'Review', schema: FINDINGS }),
  (r) =>
    parallel(
      (r?.findings ?? []).map((f) => () =>
        agent(
          `Adversarially VERIFY this Go review finding (default isReal=false). Read ${repo}/${f.file} and confirm only ` +
            `if the evidence holds and it truly violates the cited rule in ${BOOK}.\n` +
            `title:${f.title}\nsev:${f.severity}\nfile:${f.file}\nline:${f.line}\nrule:${f.rule}\nevidence:${f.evidence}\nfix:${f.fix}`,
          { label: `verify:${(f.file || 'x').split('/').pop()}`, phase: 'Verify', schema: VERDICT },
        ).then((v) => ({ ...f, dimension: r.dimension, verdict: v })),
      ),
  ),
)

const all = reviewed.flat().filter(Boolean)
const order = { blocker: 0, high: 1, medium: 2, low: 3, nit: 4 }
const confirmed = all
  .filter((f) => f.verdict && f.verdict.isReal)
  .map((f) => ({ severity: f.verdict.severity || f.severity, dimension: f.dimension, title: f.title, file: f.file, line: f.line, rule: f.rule, fix: f.fix }))
  .sort((a, b) => (order[a.severity] ?? 9) - (order[b.severity] ?? 9))

return { dimensions: dims, reviewed: all.length, confirmedCount: confirmed.length, confirmed }
