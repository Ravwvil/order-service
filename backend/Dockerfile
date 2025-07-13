FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/main.go

FROM alpine:latest

RUN apk --no-cache add curl ca-certificates

WORKDIR /app
COPY --from=builder /app/server .

EXPOSE 8081

# Healthcheck to ensure the server is ready before accepting traffic
HEALTHCHECK --interval=5s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:8081/healthz || exit 1

ENTRYPOINT ["/app/server"] 