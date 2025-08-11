# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache build-base git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# 静态编译，便于精简运行镜像
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/server ./cmd/server

FROM alpine:3.19 AS runtime
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/server /usr/local/bin/server
# 复制静态资源，供 /ui 与 /app 使用
COPY web /app/web
# 可选复制 config.yml（如存在）
COPY config.yml /app/config.yml
EXPOSE 8080
ENV IM_LISTEN_ADDR=:8080
CMD ["server"] 