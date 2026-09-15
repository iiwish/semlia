FROM node:24.15.0-bookworm-slim AS web-builder

ENV PNPM_HOME=/pnpm
ENV PATH=$PNPM_HOME:$PATH
WORKDIR /src

RUN corepack enable && corepack prepare pnpm@11.1.3 --activate
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY sdk/typescript/package.json sdk/typescript/package.json
COPY web/package.json web/package.json
RUN pnpm install --frozen-lockfile --filter @semlia/web...

COPY sdk/typescript sdk/typescript
COPY web web
RUN pnpm --filter @semlia/web build

FROM golang:1.26.6-alpine AS go-builder

WORKDIR /src
RUN apk add --no-cache build-base
COPY go.mod go.sum ./
COPY . .
RUN chmod -R a+rX /src/migrations
RUN rm -rf internal/platform/web/static
COPY --from=web-builder /src/web/dist internal/platform/web/static
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=1 GOOS=linux go build -trimpath -tags netgo \
    -ldflags="-s -w -linkmode external -extldflags -static" -o /out/semlia ./cmd/semlia
RUN mkdir -p /out/git-content/sql /out/artifacts

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app
COPY --from=go-builder /out/semlia /app/semlia
COPY --from=go-builder /src/migrations /app/migrations
COPY --from=go-builder --chown=nonroot:nonroot /out/git-content /var/lib/semlia/content
COPY --from=go-builder --chown=nonroot:nonroot /out/artifacts /var/lib/semlia/artifacts
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/semlia"]
CMD ["server"]
