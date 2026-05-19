# 第一阶段：编译 Go 服务
FROM golang:1.25 AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY authd/go.mod authd/go.sum ./
RUN go mod download
COPY authd/ .
RUN go install github.com/swaggo/swag/cmd/swag@v1.8.12 && \
    swag init -g main.go --parseDependency --parseInternal && \
    CGO_ENABLED=0 go build -o authd .

# 第二阶段：运行环境（Ubuntu，包含 Samba + NFS + ACL 工具）
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y \
    samba \
    nfs-kernel-server \
    nginx \
    libnginx-mod-http-dav-ext \
    libnss-ldap libpam-ldap \
    acl \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/authd /usr/local/bin/authd
COPY deploy/smb.conf /etc/samba/smb.conf
COPY deploy/ldap.conf /etc/ldap.conf
COPY deploy/nsswitch.conf /etc/nsswitch.conf
COPY deploy/nginx-webdav.conf /etc/nginx/sites-enabled/webdav
COPY deploy/start.sh /start.sh
RUN rm -f /etc/nginx/sites-enabled/default
RUN chmod +x /start.sh

CMD ["/start.sh"]
