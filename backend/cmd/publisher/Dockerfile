FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -o /publisher ./cmd/publisher

FROM alpine:latest

RUN apk --no-cache add ca-certificates

COPY --from=builder /publisher /publisher

ENTRYPOINT ["/publisher"] 