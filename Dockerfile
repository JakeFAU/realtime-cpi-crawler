# syntax=docker/dockerfile:1.7

##############################
# Build
##############################
ARG GO_VERSION=1.25
ARG BUILD_TARGET="."          # <-- set to "./cmd/crawler" if you move later

FROM golang:${GO_VERSION}-bookworm AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION="dev"
ARG COMMIT="dirty"
ARG BUILD_DATE
ARG BUILD_TARGET

WORKDIR /workspace

# deps first for caching
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

# copy the rest
COPY . .

# build
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
    go build -buildvcs=false -trimpath \
    -ldflags="-s -w \
    -X main.version=${VERSION} \
    -X main.commit=${COMMIT} \
    -X main.buildDate=${BUILD_DATE}" \
    -o /out/webcrawler ${BUILD_TARGET}

##############################
# Runtime (distroless)
##############################
FROM gcr.io/distroless/static-debian12:nonroot AS runtime
WORKDIR /app
COPY --from=builder /out/webcrawler /usr/local/bin/webcrawler
COPY config.yaml /app/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/webcrawler"]
CMD ["crawl"]

##############################
# Runtime (Chromium) — optional
##############################
FROM debian:bookworm-slim AS runtime-chrome
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    chromium ca-certificates fonts-liberation tzdata && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /app
RUN useradd -u 65532 -r -s /usr/sbin/nologin app
USER app
ENV CHROME_PATH=/usr/bin/chromium
COPY --from=builder /out/webcrawler /usr/local/bin/webcrawler
COPY --chown=app:app config.yaml /app/config.yaml
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/webcrawler"]
CMD ["crawl"]
