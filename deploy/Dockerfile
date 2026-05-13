# 第一阶段：编译 Go 服务
FROM golang:1.25 AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY authd/go.mod authd/go.sum ./
RUN go mod download
COPY authd/ .
RUN CGO_ENABLED=0 go build -o authd .

# 第二阶段：运行环境（Ubuntu，包含 Samba + NFS + ACL 工具）
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y \
    samba \
    nfs-kernel-server \
    libnss-ldap libpam-ldap \
    acl \
    curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/authd /usr/local/bin/authd
COPY deploy/smb.conf /etc/samba/smb.conf
COPY deploy/ldap.conf /etc/ldap.conf
COPY deploy/nsswitch.conf /etc/nsswitch.conf
COPY deploy/start.sh /start.sh
RUN chmod +x /start.sh

CMD ["/start.sh"]
