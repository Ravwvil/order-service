FROM golang:1.24-alpine as builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /migrator ./cmd/migrator/main.go

# final stage
FROM alpine:latest

COPY --from=builder /migrator /migrator
COPY --from=builder /app/migrations /migrations

ENTRYPOINT ["/migrator"] 