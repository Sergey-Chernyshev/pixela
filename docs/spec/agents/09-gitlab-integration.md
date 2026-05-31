# Agent 09 — GitLab Integration

**Фаза:** 5 · **Фичи:** F-41, F-42, F-43 · **Зависит от:** Agent 04 (статусы), Agent 06 (auth)

## Задача
Интеграция с GitLab: commit status в MR по результату билда, идемпотентный комментарий-сводка в MR, и
GitLab OAuth для входа в дашборд.

## Контекст (прочитать)
- `specs/09-integrations.md` (GitLab — детально), `specs/10-security-and-auth.md` (токен, OAuth).

## Что сделать
1. **GitLab API клиент** в `notifications/channels/`: обёртка над REST (commit statuses, MR notes), токен из env.
2. **Commit status** (F-41): на смену статуса билда ставить status на коммит:
   - COMPARING → `pending`, PASSED → `success`, REVIEW_REQUIRED → `failed` или `pending` (конфигурируемо per-project, B-13),
     REJECTED → `failed`. `target_url` → ссылка на билд в дашборде. `name = "pixela/visual"`.
3. **MR-комментарий** (F-42): постить/ОБНОВЛЯТЬ (не плодить) note со сводкой (counts + ссылка). Хранить note id
   для идемпотентного апдейта.
4. **GitLab OAuth** (F-43): `GET /auth/gitlab` (redirect) + `/auth/gitlab/callback` (обмен code, апсерт User
   по gitlabId, сессия). Конфиг `GITLAB_OAUTH_CLIENT_ID/SECRET`, `GITLAB_BASE_URL`.
5. Триггерится из notifications-оркестратора (Agent 10) на события билда; fire-and-forget, ошибки логируются.

## Acceptance criteria
- [ ] Commit status появляется в MR и обновляется при смене статуса билда; ведёт на билд в дашборде.
- [ ] Политика блокировки (failed vs pending при REVIEW_REQUIRED) конфигурируема per-project.
- [ ] MR-комментарий создаётся один раз и ОБНОВЛЯЕТСЯ при изменениях (не дублируется).
- [ ] GitLab OAuth-вход работает: новый пользователь апсертится, выдаётся сессия.
- [ ] Падение GitLab-запроса не ломает обработку билда (логируется).
- [ ] Тесты (с моками GitLab API): маппинг статусов, идемпотентность комментария, OAuth-callback.

## Не делать
- Не хранить GitLab-токен в БД проектов в открытом виде (env/секрет).
- Не завязывать обработку билда на успех нотификации.
