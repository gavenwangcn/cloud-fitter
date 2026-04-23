# 构建阶段：使用当前主流 Go（与 go.mod / toolchain 对齐；可按需改为固定补丁版本如 1.23.4-alpine）
FROM golang:1.23-alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOPROXY=https://goproxy.cn,direct

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# 显式输出名，避免依赖默认命名规则；strip 符号表减小体积
RUN go build -ldflags="-s -w" -o /cloud-fitter .

# 运行阶段：仅可执行文件 + 证书（访问各公有云 HTTPS API）
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /cloud-fitter ./cloud-fitter

RUN mkdir -p log config

EXPOSE 9090 9091

# 配置需挂载：docker run -v /your/config_dir:/app/config/ ...（目录内放 config.yaml）
ENTRYPOINT ["./cloud-fitter", "-conf=config/config.yaml", "-log_dir=log/"]
