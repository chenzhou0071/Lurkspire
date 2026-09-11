# ---- 构建阶段（Go 编译——版本与 go.mod 对齐；国内代理加速）----
FROM golang:1.26-alpine AS build
ENV GOPROXY=https://goproxy.cn,direct
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/gateway ./cmd/gateway

# ---- 运行阶段（最小镜像——纯静态二进制，无需运行时）----
FROM alpine:latest
WORKDIR /app
COPY --from=build /out/gateway /app/gateway
EXPOSE 7777
ENTRYPOINT ["/app/gateway", "--config", "/app/config.json"]
