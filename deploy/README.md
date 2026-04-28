# Docker 与 Compose 说明

## 后端（本仓库根目录 `Dockerfile`）

多阶段构建：**先**用 `bufbuild/buf` 根据 `idl/` 生成 `gen/`（构建机需能访问 `buf.build`），**再**编译 Go。`.dockerignore` 会排除宿主机上的 `gen/`，避免与镜像内生成结果不一致。运行时需挂载 `config.yaml`：

```bash
cp config_template.yaml config.yaml
# 编辑 config.yaml 后
# 本地运行：普通 docker build 即可
docker build -t cloud-fitter-app:local .
# 若需 push 到华为云 SWR 等仅接受传统清单的仓库，请用 buildx 并关闭 provenance/SBOM（见下文「镜像仓库」）
docker run --rm -p 9090:9090 -p 9091:9091 -v "$PWD/config.yaml:/app/config/config.yaml:ro" cloud-fitter-app:local
```

### 镜像仓库（华为云 SWR 基础版等）

BuildKit/`docker buildx` 默认会附加 provenance（及可能 SBOM），镜像清单呈现 OCI 形态，部分仓库（如华为云 **SWR 基础版**）会报 `Invalid image, fail to parse 'manifest.json'`。

- **推荐 `docker compose build`**：Compose 里已为各服务的 `build` 设置 `provenance: false`、`sbom: false`，构建完成后对 `cloud-fitter-app:local` / `cloud-fitter-web:local` 执行 `docker tag` 再打 `docker push` 即可，与单独脚本等价。
- **不用 Compose、仅命令行一键 build 并推送**时可用：

```bash
docker buildx build --provenance=false --sbom=false -f Dockerfile \
  -t swr.cn-east-3.myhuaweicloud.com/<组织>/cloud-fitter-app:<标签> \
  --push .
```

前端镜像同理：`deploy/web/Dockerfile` 加 `-f`，并关闭 provenance/SBOM。

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

推送到不接受 OCI attestations 的仓库时：

```bash
docker buildx build --provenance=false --sbom=false -f deploy/web/Dockerfile \
  -t <仓库地址>/<组织>/cloud-fitter-web:<标签> --push .
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
