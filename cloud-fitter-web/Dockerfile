FROM nginx:latest

ENV TZ=Asia/Shanghai

COPY ./dist /usr/share/nginx/html/
COPY ./default.conf /etc/nginx/conf.d/
EXPOSE 8080
