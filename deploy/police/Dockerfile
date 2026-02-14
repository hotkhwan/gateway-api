# ----- Build Stage -----
FROM golang:1.24-alpine AS builder

WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o main .

# ----- Runtime Stage -----
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/main .
COPY .env.prod .env.prod

EXPOSE 3000

# Run with default env flag (can be overridden at runtime)
CMD ["./main", "--env=prod"]
