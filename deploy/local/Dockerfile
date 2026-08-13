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

FROM golang:1.26.5-alpine AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
COPY . .
RUN chmod -R a+rX /src/migrations
RUN rm -rf internal/platform/web/static
COPY --from=web-builder /src/web/dist internal/platform/web/static
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/semlia ./cmd/semlia

FROM gcr.io/distroless/base-debian12:nonroot

WORKDIR /app
COPY --from=go-builder /out/semlia /app/semlia
COPY --from=go-builder /src/migrations /app/migrations
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/semlia"]
CMD ["server"]
