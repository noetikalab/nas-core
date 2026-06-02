-- 存证操作记录表：合并原 operation_logs，增加文件元信息和内容指纹
CREATE TABLE IF NOT EXISTS certified_operations (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp     INTEGER NOT NULL,              -- Unix 纳秒
    type          TEXT NOT NULL,                 -- "file" | "auth" | "system"
    user_name     TEXT NOT NULL,                 -- 操作者用户名
    user_uid      INTEGER NOT NULL DEFAULT 0,    -- 操作者 UID
    action        TEXT NOT NULL,                 -- upload | download | delete | mkdir | move
    path          TEXT NOT NULL,                 -- 操作目标路径
    dest_path     TEXT NOT NULL DEFAULT '',      -- move/rename 目标路径
    detail        TEXT NOT NULL DEFAULT '',      -- 补充说明
    file_name     TEXT NOT NULL DEFAULT '',      -- 文件名
    is_dir        INTEGER NOT NULL DEFAULT 0,    -- 是否目录
    file_size     INTEGER NOT NULL DEFAULT 0,    -- 文件大小（字节）
    mime_type     TEXT NOT NULL DEFAULT '',      -- MIME 类型
    owner_uid     INTEGER NOT NULL DEFAULT 0,    -- 文件所有者 UID
    owner_name    TEXT NOT NULL DEFAULT '',      -- 文件所有者用户名
    group_name    TEXT NOT NULL DEFAULT '',      -- 文件所属组
    file_perm     TEXT NOT NULL DEFAULT '',      -- POSIX 权限位
    mod_time      INTEGER NOT NULL DEFAULT 0,    -- 文件修改时间（Unix 纳秒）
    file_hash     BLOB,                          -- SM3/SHA-256(文件内容)
    hash_algo     TEXT NOT NULL DEFAULT ''       -- "SM3" | "SHA-256"
);

-- 存证哈希链表：每条记录通过 prev_hash 链接前一条
CREATE TABLE IF NOT EXISTS proof_records (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    cert_id       INTEGER NOT NULL UNIQUE,       -- FK → certified_operations.id
    chain_index   INTEGER NOT NULL UNIQUE,       -- 链序号（自增）
    prev_hash     BLOB,                          -- 前一条 data_hash，首条为 NULL
    data_hash     BLOB NOT NULL,                 -- SHA-256(operation + prev_hash)
    signature     BLOB,                          -- PUF SM2 签名，就绪前为 NULL
    device_uid    BLOB,                          -- PUF 硬件 UID
    sig_timestamp INTEGER NOT NULL DEFAULT 0,    -- 签名时间（Unix 纳秒）
    hash_algo     TEXT NOT NULL DEFAULT '',      -- "SM3" | "SHA-256"
    FOREIGN KEY (cert_id) REFERENCES certified_operations(id)
);

CREATE INDEX IF NOT EXISTS idx_certified_ops_timestamp ON certified_operations(timestamp);
CREATE INDEX IF NOT EXISTS idx_certified_ops_type ON certified_operations(type);
CREATE INDEX IF NOT EXISTS idx_certified_ops_user ON certified_operations(user_name);
CREATE INDEX IF NOT EXISTS idx_proof_chain_index ON proof_records(chain_index);
CREATE INDEX IF NOT EXISTS idx_proof_cert_id ON proof_records(cert_id);
