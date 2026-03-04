# Stage 1: Builder
FROM docker.io/library/golang:1.26 AS builder

WORKDIR /app

ENV GOPROXY=direct

# ติดตั้ง swag สำหรับ generate docs ก่อน build
RUN go install github.com/swaggo/swag/cmd/swag@latest

COPY go.mod go.sum ./
RUN go mod tidy

COPY . .

# Generate swagger docs (ถ้าไม่ใช้ลบบรรทัดนี้ออก)
RUN swag init

# Build แบบ Production
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o app .

# Stage 2: Runtime (image เล็กสุด)
FROM docker.io/library/alpine:3.19

WORKDIR /app

# ติดตั้ง ca-certificates (ถ้าเรียก HTTPS ภายนอก)
RUN apk --no-cache add ca-certificates tzdata

# Copy แค่ binary จาก builder
COPY --from=builder /app/app .

# Copy ไฟล์ที่จำเป็น (เช่น .env, config, static files)
# COPY --from=builder /app/.env .
# COPY --from=builder /app/static ./static

EXPOSE 3001

CMD ["./app"]