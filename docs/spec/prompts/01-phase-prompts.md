# 01 — Phase Prompts

> Скармливай Claude Code по одному, когда предыдущая фаза доведена до зелёного и закоммичена.
> Каждый промт самодостаточен и ссылается на агент-файл с детальным ТЗ.

---

## Фаза 1 — Ingestion API + Storage

> Цель: принимать билды и скриншоты из CI с дедупликацией; хранить блобы в MinIO, метаданные в Postgres.

Реализуй Фазу 1 по `docs/spec/agents/02-ingestion-api.md` и `docs/spec/agents/03-storage-layer.md`.
Покрывает фичи F-01..F-08, F-34. Сверься с `specs/04-api-contract.md` (endpoints приёма) и
`specs/10-security-and-auth.md` (API-ключи).

Сначала план + вопросы. Особое внимание: двухшаговый протокол заливки с проверкой sha256, идемпотентность
(upsert по уникальному ключу снапшота), content-addressable дедупликация. Напиши тесты на: дедупликацию
(один блоб на одинаковый sha), idempotent retry, mismatch хэша → 400.

Definition of Done: можно через curl/SDK создать билд, залить N скриншотов (повторная заливка не плодит
дубли), финализировать; блобы лежат в MinIO по sha, метаданные в Postgres; API-ключ обязателен и
изолирует проект; тесты зелёные.

---

## Фаза 2 — Diff Pipeline

> Цель: асинхронное сравнение через очередь; классификация снапшотов; агрегатный статус билда.

Реализуй Фазу 2 по `docs/spec/agents/04-diff-pipeline.md`. Покрывает F-09..F-14 (odiff F-46 — опционально,
за флагом). Сверься с `specs/07-diff-engine.md`.

Сначала план + вопросы. Критично: детерминизм diff (фикс. версии, фикс. параметры pixelmatch/sharp);
изоляция ошибок (битый PNG не валит билд); корректная обработка РАЗНЫХ размеров baseline/new (MVP: разные
размеры = CHANGED, не «умный» ресайз); вычисление REMOVED на финализации. Тесты на: каждый статус
(unchanged/changed/new/removed), порог diffRatio, разные размеры, ошибку декода → ERROR с ретраем.

Definition of Done: после финализации билда воркер сравнивает каждый снапшот, проставляет статус и
diffRatio, генерирует diff-PNG для CHANGED, выставляет агрегатный статус билда; всё детерминировано и
покрыто тестами; падение одного job не ломает билд.

---

## Фаза 3 — Playwright Reporter + SDK

> Цель: тестовый проект на Playwright заливает скриншоты в Pixela одной строкой подключения.

Реализуй Фазу 3 по `docs/spec/agents/05-playwright-reporter.md` и `docs/spec/playwright/*`. Покрывает
F-15..F-19. Сверься с `specs/04-api-contract.md` и `playwright/reporter-design.md`.

Сначала план + вопросы. Критично: автоопределение GitLab CI env (F-17); поддержка шардинга (агрегация в
один билд, F-18); soft mode (F-19); вычисление sha256 PNG на клиенте для дедупликации; режим A baseline
(reporter дочитывает baseline-файлы Playwright и заливает их для ревью). Сделай пример тестового проекта
в `examples/` и проверь сквозной прогон против локального Pixela.

Definition of Done: пример Playwright-проекта с `@pixela/playwright-reporter` прогоняет тесты, скриншоты с
корректными метаданными приезжают в Pixela, шарды агрегируются в один билд, diff считается; документация
подключения готова (`playwright/example-usage.md` отражает реальность).

---

## Фаза 4 — Dashboard Shell + Review UI

> Цель: Angular-дашборд со списком билдов и качественным review UI (3 режима + синхронный зум).

Реализуй Фазу 4 по `docs/spec/agents/06-frontend-shell.md` и `docs/spec/agents/07-review-ui.md`.
Покрывает F-20..F-28, F-35, F-36, F-44. Сверься с `specs/08-frontend-spec.md` и `frontend/angular-decisions.md`.
**Применяй frontend-design skill** (см. `skills/how-to-use-your-skills.md`) — это самая UX-чувствительная часть.

Сначала план + вопросы. Критично: компонент `image-compare` с 3 режимами (side-by-side / onion / slider) и
СИНХРОННЫМ зумом/паном; ленивые thumbnails и виртуализация списка (F-27); хоткеи навигации/approve (F-28);
типы фронта зеркалят API-контракт; signals + OnPush; без NgModules и тяжёлых китов.

Definition of Done: вход в дашборд, список проектов и билдов с фильтрами, детали билда с фильтром «только
изменившиеся», открытие снапшота показывает 3 режима сравнения с синхронным зумом; навигация хоткеями;
список превью не тормозит на сотнях изображений.

---

## Фаза 5 — Approve Workflow + GitLab

> Цель: замкнуть цикл — approve обновляет baseline (git-native) и репортит статус в GitLab MR.

Реализуй Фазу 5 по `docs/spec/agents/08-approve-workflow.md` и `docs/spec/agents/09-gitlab-integration.md`.
Покрывает F-29..F-33, F-41, F-42, F-43 (маски F-45 — если успеваем). Сверься с `specs/06-baseline-strategy.md`
(раздел Approve→Git) и `specs/09-integrations.md`.

Сначала план + вопросы. Критично: approve обновляет Baseline + ApprovalEvent + пересчитывает статус билда;
git-native действие (MVP: CLI `pixela pull-baseline`; если есть токен — commit через GitLab API);
commit status в MR (F-41); идемпотентный комментарий в MR (F-42, обновлять, не плодить). НЕ скатывайся в
merge-base resolution.

Definition of Done: approve/reject поштучно и пачкой работают, baseline обновляется, история ведётся;
GitLab MR получает статус и (опц.) комментарий со сводкой и ссылкой; git-native обновление baseline
выполнимо хотя бы через CLI.

---

## Фаза 6 — Notifications + полировка

> Цель: уведомления в Telegram/Slack; ignore-области в UI; шлифовка.

Реализуй Фазу 6 по `docs/spec/agents/10-notifications.md`. Покрывает F-47, F-48 (и F-45 если не сделана).
Сверься с `specs/09-integrations.md` (Telegram/Slack).

Сначала план. Критично: каналы за общим интерфейсом; отправка fire-and-forget (падение нотификации не
ломает билд); дебаунс (одно сообщение на смену статуса билда, не на каждый снапшот).

Definition of Done: при REVIEW_REQUIRED/PASSED приходит сообщение в настроенный Telegram/Slack со сводкой
и ссылкой; конфиг per-project; ошибки уведомлений логируются, но не влияют на обработку.

---

## Фаза 7 — Deploy + CI самого Pixela

> Цель: продакшен-деплой через docker-compose за reverse-proxy с TLS; бэкапы; CI самого Pixela.

Реализуй Фазу 7 по `docs/spec/agents/11-deploy-and-ci.md` и `docs/spec/infra/*`. Покрывает F-37, F-39, F-40.
Сверься с `infra/docker-compose.md` и `infra/deployment.md`.

Сначала план. Критично: прод docker-compose (api, worker, web, postgres, redis, minio, traefik); TLS;
healthchecks; скрипт бэкапа Postgres + бакета MinIO; документ восстановления; CI-пайплайн самого Pixela
(линт, тесты, сборка образов).

Definition of Done: `docker-compose -f docker-compose.prod.yml up` поднимает весь стек за Traefik с TLS;
healthchecks зелёные; бэкап-скрипт работает и задокументирован; восстановление проверено; README
объясняет деплой и обновление.
