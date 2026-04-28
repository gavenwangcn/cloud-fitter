# 构建上下文：cloud-fitter 仓库根目录（与 docker-compose 中 web.build.context 一致）
# 前端源码目录由 WEB_SUBDIR 指定（默认 cloud-fitter-web 子目录）

ARG WEB_SUBDIR=cloud-fitter-web

FROM swr.cn-east-3.myhuaweicloud.com/asdwjl/node:22-bookworm-slim AS builder
ARG WEB_SUBDIR

WORKDIR /app

RUN apt-get update \
	&& apt-get install -y --no-install-recommends python3 make g++ \
	&& rm -rf /var/lib/apt/lists/*

COPY ${WEB_SUBDIR}/package.json ./
RUN npm install --legacy-peer-deps

COPY ${WEB_SUBDIR}/ ./
# Node 17+ / OpenSSL 3 下 Webpack 4（Umi 2）会报 ERR_OSSL_EVP_UNSUPPORTED
ENV NODE_OPTIONS=--openssl-legacy-provider
RUN npm run build

FROM swr.cn-east-3.myhuaweicloud.com/asdwjl/nginx:1.26-alpine

COPY --from=builder /app/dist /usr/share/nginx/html/
# 文件在本仓库 deploy/web/，不依赖前端仓库内是否有 default.conf
COPY deploy/web/default.conf /etc/nginx/conf.d/default.conf

EXPOSE 8089

CMD ["nginx", "-g", "daemon off;"]
