# Agent 04 — Diff Pipeline

**Фаза:** 2 · **Фичи:** F-09..F-14, F-46 (opt) · **Зависит от:** Agent 02, Agent 03

## Задача
Асинхронное сравнение скриншотов: очередь BullMQ, воркер-процессор, pixelmatch+sharp, классификация
статусов, генерация diff-PNG, агрегатный статус билда. Это «движок», но он сознательно простой.

## Контекст (прочитать)
- `specs/07-diff-engine.md` (алгоритм, пороги, детерминизм — главное), `specs/03-data-model.md` (статусы),
  `specs/06-baseline-strategy.md` (сравнение в пределах ветки, НЕ merge-base).

## Что сделать
1. **Очередь**: BullMQ на Redis. Producer (в ingestion при финализации) кладёт job `{ snapshotId }`.
2. **Воркер** (`API_MODE=worker`): consumer с конфигурируемой concurrency (дефолт = CPU).
3. **Baseline-резолв**: найти Baseline по (project, branch, name, browser, viewport). Branch берётся из
   билда снапшота. НИКАКОГО merge-base — строго эта ветка.
4. **Diff-алгоритм** по `07-diff-engine.md`:
   - baseline отсутствует → NEW (без diff).
   - decode обеих через sharp → RGBA.
   - разные размеры → CHANGED (diffRatio=1), без ресайза (MVP).
   - применить ignore-маски, если заданы.
   - pixelmatch → diffPixels; diffRatio = diffPixels/(w*h).
   - <= порога → UNCHANGED; иначе → закодировать diff-PNG, sha256, put в S3, CHANGED.
5. **Агрегатный статус билда**: когда все снапшоты обработаны → PASSED (все unchanged/approved) или
   REVIEW_REQUIRED (есть changed/new/removed/error). Пересчёт атомарно/идемпотентно.
6. **Детерминизм**: зафиксировать версии pixelmatch/sharp, параметры (includeAA, threshold, кодирование).
7. **Изоляция ошибок**: битый PNG / ошибка → Snapshot.ERROR + errorMsg, ретрай (BullMQ backoff N раз), билд не падает.
8. (opt F-46) odiff за фиче-флагом per-project как альтернативный движок.

## Acceptance criteria
- [ ] Diff выполняется в воркере, не в HTTP-запросе (инвариант #3).
- [ ] Каждый статус корректен: UNCHANGED (идентичные), CHANGED (различие + diff-PNG), NEW (нет baseline),
      REMOVED (вычислен на финализации), ERROR (битый PNG).
- [ ] **Детерминизм**: одни и те же два PNG → идентичный diffRatio и diff-sha при повторных прогонах.
- [ ] Разные размеры baseline/new → CHANGED, без падения и без «умного» ресайза.
- [ ] Порог diffRatioThreshold и pixelThreshold применяются (per-project + override).
- [ ] Падение одного job не валит билд; job ретраится; после исчерпания → ERROR.
- [ ] Агрегатный статус билда выставляется корректно и идемпотентно.
- [ ] Тесты: все статусы, детерминизм, разные размеры, порог, ошибка декода+ретрай, агрегатный статус.

## Не делать
- Не изобретать свой алгоритм сравнения/ML (инвариант #5: diff дешёвый, pixelmatch достаточно).
- Не делать merge-base baseline (инвариант #1).
