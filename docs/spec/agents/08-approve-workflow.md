# Agent 08 — Approve Workflow (git-native)

**Фаза:** 5 · **Фичи:** F-29..F-33, F-45 (opt) · **Зависит от:** Agent 04, Agent 07

## Задача
Реализовать approve/reject: обновление Baseline, аудит ApprovalEvent, пересчёт статуса билда и git-native
обновление baseline-файлов в репозитории тестируемого проекта. Это замыкает цикл visual regression.

## Контекст (прочитать)
- `specs/06-baseline-strategy.md` (раздел «Approve → Git», ОБЯЗАТЕЛЬНО), `specs/04-api-contract.md`
  (approve/reject endpoints), `specs/03-data-model.md` (Baseline, ApprovalEvent), brainstorm B-12.

## Что сделать
1. `POST /snapshots/:id/approve`:
   - создать `ApprovalEvent(APPROVE)`;
   - upsert `Baseline` для (project, branch, name, browser, viewport) → новый `imageSha` (= newImageSha снапшота);
   - Snapshot.status = APPROVED;
   - пересчитать агрегатный статус билда (нет нерешённых CHANGED/NEW → PASSED).
2. `POST /snapshots/:id/reject`: ApprovalEvent(REJECT); Snapshot.status = REJECTED; статус билда → REJECTED
   (или остаётся REVIEW_REQUIRED по политике — реши и зафиксируй).
3. `POST /builds/:id/approve-all` и `/reject-all`: пакетно по всем CHANGED+NEW; вернуть счётчики; атомарно.
4. **Git-native обновление baseline** (F-32) — реализовать по выбранному в B-12 уровню:
   - **MVP-минимум (обязательно)**: CLI `pixela pull-baseline --build <id>` (в `packages/sdk` или отдельный bin):
     скачивает approved-картинки и кладёт в пути снапшотов Playwright; разработчик коммитит сам.
   - **Если есть write-токен (желательно)**: commit обновлённых PNG в ветку через GitLab Commits API
     (несколько файлов в одном коммите).
   - Отдельный MR — пост-MVP (B-12 вариант 3).
5. **История** (F-33): endpoint отдаёт ленту ApprovalEvent по снапшоту (кто/когда/что); UI в Agent 07.
6. (opt F-45) **Ignore-области**: сохранение масок per-snapshot/по name из дашборда; применяются в diff (Agent 04).

## Acceptance criteria
- [ ] Approve обновляет Baseline и пишет ApprovalEvent; повторный approve идемпотентен.
- [ ] Reject фиксируется в аудите; статус билда меняется по заданной политике.
- [ ] Approve-all/reject-all обрабатывают все нерешённые снапшоты атомарно, возвращают счётчики.
- [ ] После решения всех изменений статус билда корректно становится PASSED (или REJECTED).
- [ ] CLI `pixela pull-baseline` реально приводит baseline-файлы проекта к approved-состоянию.
- [ ] (если включён токен) commit через GitLab API создаёт корректный коммит с обновлёнными PNG.
- [ ] История approve доступна по API и отражается в UI.
- [ ] Тесты: approve обновляет baseline, идемпотентность, пересчёт статуса, approve-all, аудит.

## Не делать
- НЕ реализовывать merge-base resolution (инвариант #1) — baseline всегда в пределах ветки.
- Не делать необратимых git-операций без явного действия пользователя.
