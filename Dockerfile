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

# 复制 go.mod / go.sum 预热模块缓存；新增依赖若未写入 go.sum，构建前 tidy 会补全
COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=codegen /workspace/gen ./gen
# 显式输出名，避免依赖默认命名规则；strip 符号表减小体积
RUN go mod tidy && go build -ldflags="-s -w" -o /cloud-fitter . \
	&& go build -ldflags="-s -w" -o /init-mysql ./cmd/init-mysql

# 运行阶段：仅可执行文件 + 证书（访问各公有云 HTTPS API）
FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /cloud-fitter ./cloud-fitter
COPY --from=builder /init-mysql ./init-mysql

RUN mkdir -p log config data

EXPOSE 9090 9091

# 配置需挂载：docker run -v /your/config_dir:/app/config/ ...（目录内放 config.yaml）
# SQLite 账号配置默认 data/cloud-fitter.db，可挂载 -v /your/data:/app/data
# -logtostderr 便于 docker logs 查看；与 glog.Infof 等配合排查 API/云同步问题
ENTRYPOINT ["./cloud-fitter", "-conf=config/config.yaml", "-sqlitedb=data/cloud-fitter.db", "-log_dir=log/", "-logtostderr=true", "-stderrthreshold=INFO"]
