# Agent 11 — Deploy & CI (самого Pixela)

**Фаза:** 7 · **Фичи:** F-37, F-39, F-40 · **Зависит от:** все предыдущие

## Задача
Продакшен-деплой Pixela через docker-compose за reverse-proxy с TLS, healthchecks, бэкап БД и бакета,
процедура восстановления, и CI-пайплайн самого Pixela (линт, тесты, сборка образов).

## Контекст (прочитать)
- `infra/docker-compose.md`, `infra/deployment.md`, `specs/11-nonfunctional.md` (бэкап/восстановление, надёжность),
  `specs/10-security-and-auth.md` (TLS, закрытие MinIO-консоли).

## Что сделать
1. **Прод docker-compose** (`docker-compose.prod.yml`): сервисы api, worker, web, postgres, redis, minio, traefik.
   Отдельные образы для api/worker (один код, разный `API_MODE`) и web (статика за nginx или Traefik).
2. **Dockerfiles**: многоступенчатые сборки для api (node), web (build Angular → отдать статику), sdk (если публикуется).
3. **Traefik**: маршрутизация (web на `/`, api на `/api`), TLS (Let's Encrypt ACME или свой серт), редирект http→https.
4. **Healthchecks** в compose для всех сервисов; `depends_on` с условиями healthy.
5. **MinIO-консоль (9001)** — НЕ публиковать наружу (или basic-auth/закрыть фаерволом). API MinIO (9000) — только внутри сети.
6. **Бэкап-скрипт** (`scripts/backup.sh`): `pg_dump` Postgres + `mc mirror`/реплика бакета MinIO; ротация; вывод вовне.
7. **Восстановление** (`infra/deployment.md`): пошагово — поднять compose, восстановить БД из дампа, восстановить бакет;
   проверить хотя бы раз на стейджинге.
8. **CI Pixela** (`.gitlab-ci.yml`): стадии install → lint → typecheck → test (unit api/sdk + e2e) → build образов
   → (опц.) push в registry. Кэш зависимостей.
9. **Конфиг прод-env**: `.env.prod.example`; чёткое разделение dev/prod переменных; секреты не в репо.

## Acceptance criteria
- [ ] `docker-compose -f docker-compose.prod.yml up -d` поднимает весь стек за Traefik с рабочим TLS.
- [ ] Healthchecks зелёные; сервисы стартуют в правильном порядке (depends_on healthy).
- [ ] Дашборд и API доступны по https; http редиректит на https; CORS корректен.
- [ ] MinIO-консоль недоступна публично; S3-API только внутри сети.
- [ ] `scripts/backup.sh` создаёт восстановимый дамп БД и копию бакета; ротация работает.
- [ ] Процедура восстановления задокументирована и проверена (БД+бакет → рабочая система).
- [ ] CI самого Pixela: линт/типы/тесты/сборка зелёные; образы собираются.
- [ ] README объясняет деплой, обновление (rolling: миграции → новые образы), и бэкап.

## Не делать
- Не публиковать MinIO-консоль и внутренние порты наружу.
- Не хранить секреты в образах/репо.
- Не делать сложный оркестратор (k8s) в v1 — docker-compose достаточно для целевого масштаба.
