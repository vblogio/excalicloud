# syntax=docker/dockerfile:1

# --- 1. Frontend : React + @excalidraw/excalidraw, build Vite -> /src/web/dist
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
# Version d'Excalidraw imposée au build (vide : celle de package-lock.json).
# Ex. : EXCALIDRAW_VERSION=0.18.2 docker compose up -d --build
ARG EXCALIDRAW_VERSION=
RUN --mount=type=cache,target=/root/.npm \
    if [ -n "$EXCALIDRAW_VERSION" ]; then \
      npm install --no-audit --no-fund --save-exact "@excalidraw/excalidraw@$EXCALIDRAW_VERSION"; \
    fi
COPY frontend/ ./
RUN npm run build

# --- 2. Backend : binaire Go statique embarquant le frontend
FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/
COPY --from=frontend /src/web/dist ./web/dist
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go test ./internal/... && \
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server && \
    mkdir -p /out/data

# --- 3. Image finale : distroless, non-root (uid 65532)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=backend /out/server /server
COPY --from=backend --chown=65532:65532 /out/data /data
ENV LISTEN_ADDR=:8080 \
    DATA_DIR=/data \
    MAX_BODY_MB=50
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]
