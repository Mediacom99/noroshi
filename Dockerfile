FROM golang:1.26.1-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /monitor ./cmd/monitor/

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata curl && \
    adduser -D -h /app appuser

WORKDIR /app

COPY --from=builder /monitor .

RUN mkdir -p /app/data && chown appuser:appuser /app/data

USER appuser

VOLUME ["/app/data"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8080/healthz || exit 1

ENTRYPOINT ["./monitor"]
