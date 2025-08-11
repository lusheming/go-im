-- Groups
CREATE TABLE IF NOT EXISTS `groups` (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  owner_id VARCHAR(64) NOT NULL,
  muted TINYINT(1) NOT NULL DEFAULT 0,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  KEY idx_owner (owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Group members
CREATE TABLE IF NOT EXISTS group_members (
  group_id VARCHAR(64) NOT NULL,
  user_id VARCHAR(64) NOT NULL,
  role VARCHAR(16) NOT NULL,
  remark VARCHAR(128) DEFAULT '',
  muted_until DATETIME DEFAULT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY(group_id, user_id),
  KEY idx_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Group notices
CREATE TABLE IF NOT EXISTS group_notices (
  id VARCHAR(64) PRIMARY KEY,
  group_id VARCHAR(64) NOT NULL,
  title VARCHAR(255) NOT NULL,
  content TEXT NOT NULL,
  created_by VARCHAR(64) NOT NULL,
  created_at DATETIME NOT NULL,
  KEY idx_group (group_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Messages (TiDB/MySQL compatible)
CREATE TABLE IF NOT EXISTS messages (
  server_msg_id VARCHAR(64) PRIMARY KEY,
  client_msg_id VARCHAR(128) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  conv_type VARCHAR(16) NOT NULL,
  from_user_id VARCHAR(64) NOT NULL,
  to_user_id VARCHAR(64) DEFAULT NULL,
  group_id VARCHAR(64) DEFAULT NULL,
  seq BIGINT NOT NULL,
  timestamp DATETIME NOT NULL,
  type VARCHAR(32) NOT NULL,
  payload BLOB,
  recalled TINYINT(1) NOT NULL DEFAULT 0,
  expire_at DATETIME NULL,
  burn_after_read TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY uniq_conv_client (conv_id, client_msg_id),
  KEY idx_conv_seq (conv_id, seq),
  KEY idx_expire_at (expire_at),
  KEY idx_from_time (from_user_id, timestamp)
) /*T! SHARD_ROW_ID_BITS=4 PRE_SPLIT_REGIONS=8 */ DEFAULT CHARSET=utf8mb4;

-- Per-user conversation delete watermark
CREATE TABLE IF NOT EXISTS conv_deletes (
  owner_id VARCHAR(64) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  deleted_at DATETIME NOT NULL,
  PRIMARY KEY(owner_id, conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 收藏表
CREATE TABLE IF NOT EXISTS favorites (
    id VARCHAR(32) NOT NULL PRIMARY KEY COMMENT '收藏ID',
    user_id VARCHAR(32) NOT NULL COMMENT '用户ID',
    message_id VARCHAR(32) DEFAULT NULL COMMENT '消息ID（收藏消息时）',
    conv_id VARCHAR(64) NOT NULL COMMENT '会话ID',
    type ENUM('message', 'custom') NOT NULL DEFAULT 'message' COMMENT '收藏类型',
    title VARCHAR(255) NOT NULL COMMENT '收藏标题',
    content LONGBLOB NOT NULL COMMENT '收藏内容JSON',
    tags VARCHAR(500) DEFAULT '' COMMENT '标签（逗号分隔）',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_user_id (user_id),
    INDEX idx_message_id (message_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='收藏表';

-- 文件上传表
CREATE TABLE IF NOT EXISTS file_uploads (
    id VARCHAR(32) NOT NULL PRIMARY KEY COMMENT '文件ID',
    user_id VARCHAR(32) NOT NULL COMMENT '上传者ID',
    file_name VARCHAR(255) NOT NULL COMMENT '原始文件名',
    file_size BIGINT NOT NULL DEFAULT 0 COMMENT '文件大小（字节）',
    mime_type VARCHAR(100) NOT NULL COMMENT 'MIME类型',
    store_path VARCHAR(500) NOT NULL COMMENT '存储路径',
    url VARCHAR(500) NOT NULL COMMENT '访问URL',
    status ENUM('uploading', 'success', 'failed') NOT NULL DEFAULT 'uploading' COMMENT '状态',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '上传时间',
    expires_at TIMESTAMP NULL DEFAULT NULL COMMENT '过期时间',
    INDEX idx_user_id (user_id),
    INDEX idx_status (status),
    INDEX idx_created_at (created_at),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='文件上传表';

-- Read receipts (store in MySQL typically; keep here if you prefer TiDB)
CREATE TABLE IF NOT EXISTS read_receipts (
  user_id VARCHAR(64) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  seq BIGINT NOT NULL,
  PRIMARY KEY(user_id, conv_id)
) DEFAULT CHARSET=utf8mb4; 