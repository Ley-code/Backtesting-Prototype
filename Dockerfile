FROM golang:1.26.1-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/bybit-backtester ./cmd/api

FROM alpine:3.22

RUN apk add --no-cache ca-certificates && \
    addgroup -S app && \
    adduser -S -G app app

WORKDIR /app

COPY --from=builder /out/bybit-backtester ./bybit-backtester
COPY web ./web

RUN mkdir -p /app/.cache && chown -R app:app /app

ENV ADDR=:8080
ENV CACHE_DIR=/app/.cache
ENV WEB_DIR=/app/web

EXPOSE 8080

VOLUME ["/app/.cache"]

USER app

ENTRYPOINT ["./bybit-backtester"]
