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
本仓库提供 **`deploy/web/Dockerfile`**：构建上下文为 **cloud-fitter 仓库根目录**，在镜像内从子目录（默认 `cloud-fitter-web`）完成依赖安装与 `umi build`；Nginx 配置 **`deploy/web/default.conf`** 在本仓库内，不依赖前端仓库是否自带该文件。

单独构建前端（在 **cloud-fitter** 根目录执行）：

```bash
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local .
# 若前端目录名不是 cloud-fitter-web：
docker build -f deploy/web/Dockerfile -t cloud-fitter-web:local --build-arg WEB_SUBDIR=你的目录名 .
```

## Compose（默认前后端一起）

1. 准备前端源码：在 **cloud-fitter 仓库根目录** 下克隆  
   `git clone https://github.com/cloud-fitter/cloud-fitter-web.git cloud-fitter-web`  
   或与后端同级时：`ln -snf ../cloud-fitter-web cloud-fitter-web`（也可设 `CLOUD_FITTER_WEB_SUBDIR`）。
2. 在本仓库根目录准备好 `config.yaml`。
3. 在 **cloud-fitter** 根目录执行：

```bash
docker compose up -d --build
```

- 前端：<http://localhost:8089>（静态页 + `/apis` 反代到后端；映射为 8089:8089）
- 后端 HTTP（grpc-gateway）：<http://localhost:9090>，gRPC：`9091`

## Nginx 说明

- Compose 使用 **`deploy/web/nginx.compose.conf`**，将 `/apis` 代理到 Docker 网络中的服务名 **`api:9090`**，无需再配置 `localnode` hosts。
- 镜像内默认 **`deploy/web/default.conf`**（代理 `localnode:9090`），便于脱离 Compose 单独运行（需自行 `--add-host localnode:<宿主机IP>` 或改配置）。
