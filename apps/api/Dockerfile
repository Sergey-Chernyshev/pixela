# ─────────────────────────────────────────────────────────────────────────────
# Pixela Go backend — one image, two modes (serve | worker).
#
#   serve   : chi + Huma HTTP API (insert-only River client)   ← default CMD
#   worker  : River diff/finalize workers                      docker `command: [worker]`
#   migrate : Atlas migrations (one-shot)                       docker `command: [migrate]`
#
# Per the rulebook (§13): CGO_ENABLED=0 static binary, -trimpath for reproducible
# image/png bytes, distroless/static (NOT alpine/musl, NOT debian-slim by default),
# nonroot. libvips never enters this image.
# ─────────────────────────────────────────────────────────────────────────────

# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26 AS builder

WORKDIR /src

# Pre-cache modules: copy manifests first so `go mod download` is cached unless
# go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

# Source needed to build the binary. (.dockerignore prunes everything else.)
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db

# Static, stripped, reproducible build of the single `pixela` binary.
#   CGO_ENABLED=0  → no libc dependency, runs on distroless/static & scratch
#   -trimpath      → strip local FS paths (also pins reproducible image/png bytes)
#   -ldflags -s -w → drop symbol table + DWARF (smaller binary)
RUN CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /pixela \
      ./cmd/pixela

# ── Runtime stage ────────────────────────────────────────────────────────────
# distroless/static: no shell, no package manager, glibc-free. `:nonroot` ships
# a non-root user (uid/gid 65532) and CA certs + tzdata.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /pixela /pixela

USER nonroot:nonroot

# ENTRYPOINT is the binary; the subcommand is the (overridable) CMD.
#   default                → pixela serve
#   command: ["worker"]    → pixela worker
#   command: ["migrate"]   → pixela migrate
ENTRYPOINT ["/pixela"]
CMD ["serve"]
