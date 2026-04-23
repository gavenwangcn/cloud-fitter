# 构建上下文：cloud-fitter-web 仓库根目录（与 docker-compose 中 context 一致）
# 多阶段：在镜像内 npm install + umi build，避免依赖宿主机 dist；运行阶段仅 nginx + 静态资源

FROM node:18-bookworm-slim AS builder

WORKDIR /app

# umi / node-sass 等偶发需要编译工具链
RUN apt-get update \
	&& apt-get install -y --no-install-recommends python3 make g++ \
	&& rm -rf /var/lib/apt/lists/*

COPY package.json ./
# 上游仓库若未提交 lock 文件，使用 npm install；有 package-lock.json 时可改为 npm ci
RUN npm install --legacy-peer-deps

COPY . .
RUN npm run build

# -------- runtime --------
FROM nginx:1.26-alpine

COPY --from=builder /app/dist /usr/share/nginx/html/
# 默认与上游仓库一致，便于单独 docker run；Compose 下可用卷覆盖为 nginx.compose.conf
COPY default.conf /etc/nginx/conf.d/default.conf

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
