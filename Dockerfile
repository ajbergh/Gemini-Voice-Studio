# syntax=docker/dockerfile:1.7
# Multi-stage Dockerfile for Gemini Voice Studio.

FROM node:22-alpine AS frontend
WORKDIR /app
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci
COPY . .
RUN npm run build

FROM golang:1.25-alpine AS backend
RUN apk add --no-cache ca-certificates git tzdata
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY backend/ .
COPY --from=frontend /app/dist ./internal/embed/dist/

ARG VERSION=dev
ARG COMMIT_SHA=unknown
ARG BUILD_DATE=unknown
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w \
        -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Version=${VERSION} \
        -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Commit=${COMMIT_SHA} \
        -X github.com/ajbergh/gemini-voice-gen-tts/backend/internal/buildinfo.Date=${BUILD_DATE}" \
      -o /server ./cmd/server

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -h /home/app app && \
    install -d -o app -g app -m 0700 /home/app/data
USER app
WORKDIR /home/app
COPY --from=backend /server /usr/local/bin/gemini-voice-studio

VOLUME ["/home/app/data"]
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/health >/dev/null || exit 1

ENTRYPOINT ["gemini-voice-studio"]
CMD ["--port", "8080", "--data-dir", "/home/app/data", "--open=false"]
