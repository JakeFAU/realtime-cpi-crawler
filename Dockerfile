# ---- Build stage ----
FROM golang:1.25-bookworm AS builder

ENV CGO_ENABLED=0 \
    GO111MODULE=on

WORKDIR /app

# Cache deps
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy and build
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/realtime-cpi-crawler ./cmd/...

# ---- Runtime stage with headless Chromium ----
FROM debian:bookworm-slim AS runtime

# Install Chromium + fonts + CA roots + wget for healthcheck
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    chromium \
    ca-certificates \
    fonts-liberation \
    fonts-noto \
    fonts-noto-color-emoji \
    tzdata \
    wget \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Non-root user
RUN useradd -r -u 10001 -m appuser
WORKDIR /app

# Binary
COPY --from=builder /out/realtime-cpi-crawler /app/realtime-cpi-crawler
COPY config.yaml /app/config.yaml
RUN chown -R appuser:appuser /app

# App & Chrome defaults
ENV APP_PORT=8080
ENV PORT=8080
ENV CHROME_PATH=/usr/bin/chromium
# Sensible defaults for Chrome in containers; override if you need to debug:
ENV CHROME_ARGS="--headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --remote-allow-origins=*"

# If your app reads these, it can pass to chromedp.ExecAllocator:
#   execPath := os.Getenv("CHROME_PATH")
#   args := strings.Fields(os.Getenv("CHROME_ARGS"))
#   // allocate with chromedp

EXPOSE 8080
USER appuser:appuser
ENTRYPOINT ["/app/realtime-cpi-crawler", "-config", "/app/config.yaml"]
