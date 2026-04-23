# Docker 与 Compose 说明

## 后端（本仓库根目录 `Dockerfile`）

多阶段构建：**先**用 `bufbuild/buf` 根据 `idl/` 生成 `gen/`（构建机需能访问 `buf.build`），**再**编译 Go。`.dockerignore` 会排除宿主机上的 `gen/`，避免与镜像内生成结果不一致。运行时需挂载 `config.yaml`：

```bash
cp config_template.yaml config.yaml
# 编辑 config.yaml 后
docker build -t cloud-fitter-app:local .
docker run --rm -p 9090:9090 -p 9091:9091 -v "$PWD/config.yaml:/app/config/config.yaml:ro" cloud-fitter-app:local
```

排查首页 ECS 无数据：看后端日志（已输出到 stderr，Compose 下可 `docker logs -f cloud-fitter-app`）。关注 `loaded cloud`、`ecs List` / `ListAll`、`no cloud accounts loaded`、`ListDetail error` 等行。

## 前端（cloud-fitter-web）

**前端以源码形式放在本仓库 `cloud-fitter-web/` 目录，与后端一并提交**（勿只提交 submodule 指针或外链而不含实际文件；`node_modules/`、`dist/` 等见根目录 `.gitignore`）。

上游独立仓库的 `Dockerfile` 多为 `COPY ./dist`。本仓库使用 **`deploy/web/Dockerfile`**：构建上下文为 **仓库根目录**，在镜像内对 `cloud-fitter-web` 执行 `npm install` 与 `umi build`；Nginx 配置 **`deploy/web/default.conf`** / **`nginx.compose.conf`** 在 `deploy/web/`。

单独构建前端（在 **cloud-fitter** 根目录执行）：

```bash
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local .
# 若前端目录名不是 cloud-fitter-web：
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local --build-arg WEB_SUBDIR=你的目录名 .
```

## Compose

仓库根目录 **`.env`** 默认包含 **`COMPOSE_PROFILES=full`**，因此 **`docker compose up -d --build` 会同时构建/启动 app（容器名 `cloud-fitter-app`）与 web**。

须保证 **`cloud-fitter-web/package.json` 等源码已纳入本仓库并拉取到本地**（否则构建 web 会因 `COPY .../package.json` 失败）：

```bash
docker compose up -d --build
```

- 前端：<http://localhost:8089>（映射 8089:8089）
- 后端 HTTP（grpc-gateway）：<http://localhost:9090>，gRPC：`9091`

**仅后端**：清空或删除 `.env` 中的 `COMPOSE_PROFILES`，或执行 `docker compose up -d --build app`。

## Nginx 说明

- Compose 使用 **`deploy/web/nginx.compose.conf`**，将 `/apis` 代理到 Docker 网络中的服务名 **`app:9090`**，无需再配置 `localnode` hosts。
- 镜像内默认 **`deploy/web/default.conf`**（代理 `localnode:9090`），便于脱离 Compose 单独运行（需自行 `--add-host localnode:<宿主机IP>` 或改配置）。
