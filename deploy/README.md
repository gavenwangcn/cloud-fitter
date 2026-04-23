# Docker 与 Compose 说明

## 后端（本仓库根目录 `Dockerfile`）

多阶段构建：**先**用 `bufbuild/buf` 根据 `idl/` 生成 `gen/`（构建机需能访问 `buf.build`），**再**编译 Go。`.dockerignore` 会排除宿主机上的 `gen/`，避免与镜像内生成结果不一致。运行时需挂载 `config.yaml`：

```bash
cp config_template.yaml config.yaml
# 编辑 config.yaml 后
docker build -t cloud-fitter-api:local .
docker run --rm -p 9090:9090 -p 9091:9091 -v "$PWD/config.yaml:/app/config/config.yaml:ro" cloud-fitter-api:local
```

## 前端（cloud-fitter-web）

上游仓库原有 `Dockerfile` 仅 `COPY ./dist`，要求在宿主机先执行 `npm run build`。  
本仓库提供 **`deploy/web/Dockerfile`**：在镜像内完成依赖安装与 `umi build`，更利于 CI 与版本一致。

单独构建前端（将路径改为你的 `cloud-fitter-web` 克隆目录）：

```bash
# 前端在子目录 ./cloud-fitter-web 时：
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local ./cloud-fitter-web
# 或与后端同级目录时：
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local ../cloud-fitter-web
```

## 前后端一起启动

1. 准备前端源码，任选一种布局：
   - **推荐（与 compose 默认一致）**：在 **cloud-fitter 仓库根目录** 下克隆  
     `git clone https://github.com/cloud-fitter/cloud-fitter-web.git cloud-fitter-web`
   - **同级目录**：`../your-path/cloud-fitter` 与 `../your-path/cloud-fitter-web` 并列时，启动前执行  
     `export CLOUD_FITTER_WEB_DIR=../cloud-fitter-web`（Windows 用 `set CLOUD_FITTER_WEB_DIR=...`）
2. 在本仓库根目录准备好 `config.yaml`。
3. 在 **cloud-fitter** 根目录执行：

```bash
docker compose up -d --build
```

- 前端：<http://localhost:8080>（静态页 + `/apis` 反代到后端）
- 后端 HTTP（grpc-gateway）：<http://localhost:9090>，gRPC：`9091`

## Nginx 说明

- Compose 使用 **`deploy/web/nginx.compose.conf`**，将 `/apis` 代理到 Docker 网络中的服务名 **`api:9090`**，无需再配置 `localnode` hosts。
- 镜像内仍保留上游的 `default.conf`，便于脱离 Compose 单独运行（需自行 `--add-host` 或改配置）。
