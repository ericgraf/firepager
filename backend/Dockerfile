FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o api ./cmd/api
RUN go build -o processor ./cmd/processor

FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/api .
COPY --from=builder /app/processor .
# Default to API for compatibility
CMD ["./api"]
