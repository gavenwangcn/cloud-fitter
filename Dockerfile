# 阶段 1：仅根据 idl/ 生成 gen/（改 proto 后无需在宿主机先跑 gen.sh 也可构建）
FROM bufbuild/buf:1.28.1 AS codegen
WORKDIR /workspace
COPY buf.yaml buf.gen.yaml ./
COPY idl ./idl/
# 根据 buf.yaml 拉取 googleapis / grpc-gateway 等到 buf.lock（仓库若未提交 buf.lock 时也必须执行）
# 构建期需能访问 buf.build（下载依赖与 buf.gen.yaml 中的远程插件）
RUN buf mod update && buf generate

# 阶段 2：Go 编译（用生成结果覆盖上下文中的 gen/）
FROM golang:1.23-alpine AS builder

ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOPROXY=https://goproxy.cn,direct

WORKDIR /src

# 仅复制 go.mod 即可下载依赖；部分 fork 未提交 go.sum 时避免 COPY 失败
COPY go.mod ./
RUN go mod download

COPY . .
COPY --from=codegen /workspace/gen ./gen
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
# -logtostderr 便于 docker logs 查看；与 glog.Infof 等配合排查 API/云同步问题
ENTRYPOINT ["./cloud-fitter", "-conf=config/config.yaml", "-log_dir=log/", "-logtostderr=true", "-stderrthreshold=INFO"]
