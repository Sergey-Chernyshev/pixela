# ─────────────────────────────────────────────────────────────────────────────
# Pixela web dashboard — Angular build → static files served by nginx.
# Build context is the REPO ROOT (the app depends on the @pixela/shared workspace
# package), so build with:  docker build -f apps/web/Dockerfile .
# ─────────────────────────────────────────────────────────────────────────────

# ── Build stage ──────────────────────────────────────────────────────────────
FROM node:22-alpine AS build
RUN corepack enable
WORKDIR /repo

# Pre-cache the install: copy only the manifests pnpm needs to resolve the workspace.
COPY pnpm-workspace.yaml package.json pnpm-lock.yaml tsconfig.base.json ./
COPY packages/shared/package.json packages/shared/
COPY packages/sdk/package.json packages/sdk/
COPY apps/web/package.json apps/web/
RUN pnpm install --frozen-lockfile

# Sources for the web app + its only workspace dependency (@pixela/shared).
COPY packages/shared packages/shared
COPY apps/web apps/web

# Build the contract types first (the web build imports @pixela/shared/dist), then the app.
RUN pnpm --filter @pixela/shared build \
 && pnpm --filter @pixela/web build

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM nginx:alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /repo/apps/web/dist/pixela-ui/browser /usr/share/nginx/html
EXPOSE 80
