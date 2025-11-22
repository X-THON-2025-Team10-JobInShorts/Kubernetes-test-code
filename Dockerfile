# 1. 빌드 단계
FROM golang:1.20-alpine AS builder
WORKDIR /app

# go.mod 와 go.sum을 먼저 복사
COPY go.mod go.sum ./

# 의존성 다운로드
RUN go mod tidy

# 소스 코드 복사
COPY . .

# 빌드 실행
RUN go build -o worker main.go

# 2. 실행 단계
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/worker .
RUN apk add --no-cache ca-certificates
CMD ["./worker"]