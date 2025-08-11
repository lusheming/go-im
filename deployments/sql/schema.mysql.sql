-- Users
CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(64) PRIMARY KEY,
  username VARCHAR(64) NOT NULL UNIQUE,
  password VARCHAR(255) NOT NULL,
  nickname VARCHAR(128) DEFAULT '',
  avatar_url VARCHAR(512) DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Friends
CREATE TABLE IF NOT EXISTS friends (
  user_id VARCHAR(64) NOT NULL,
  friend_id VARCHAR(64) NOT NULL,
  remark VARCHAR(128) DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY(user_id, friend_id),
  KEY idx_friend (friend_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Groups
CREATE TABLE IF NOT EXISTS `groups` (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  owner_id VARCHAR(64) NOT NULL,
  muted TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否全员禁言',
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
  muted_until DATETIME DEFAULT NULL COMMENT '成员禁言截止时间',
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

-- Read receipts (per user per conversation)
CREATE TABLE IF NOT EXISTS read_receipts (
  user_id VARCHAR(64) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  seq BIGINT NOT NULL,
  PRIMARY KEY(user_id, conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Conversations（会话元信息，last_seq 用于计算未读）
CREATE TABLE IF NOT EXISTS conversations (
  id VARCHAR(128) PRIMARY KEY,
  conv_type VARCHAR(16) NOT NULL,
  peer_id VARCHAR(64) DEFAULT NULL,
  group_id VARCHAR(64) DEFAULT NULL,
  last_seq BIGINT NOT NULL DEFAULT 0,
  updated_at DATETIME NOT NULL,
  KEY idx_group (group_id),
  KEY idx_peer (peer_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- User conversations（用户与会话的关系，用于列表）
CREATE TABLE IF NOT EXISTS user_conversations (
  user_id VARCHAR(64) NOT NULL,
  conv_id VARCHAR(128) NOT NULL,
  conv_type VARCHAR(16) NOT NULL,
  peer_id VARCHAR(64) DEFAULT NULL,
  group_id VARCHAR(64) DEFAULT NULL,
  pinned TINYINT(1) NOT NULL DEFAULT 0,
  muted TINYINT(1) NOT NULL DEFAULT 0,
  draft TEXT NULL,
  updated_at DATETIME NOT NULL,
  PRIMARY KEY(user_id, conv_id),
  KEY idx_user (user_id),
  KEY idx_conv (conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Messages（适配 MySQL，含幂等与检索索引）
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
  payload LONGBLOB,
  recalled TINYINT(1) NOT NULL DEFAULT 0,
  expire_at DATETIME NULL,
  burn_after_read TINYINT(1) NOT NULL DEFAULT 0,
  UNIQUE KEY uniq_conv_client (conv_id, client_msg_id),
  KEY idx_conv_seq (conv_id, seq),
  KEY idx_expire_at (expire_at),
  KEY idx_from_time (from_user_id, timestamp)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 会话删除水位表
CREATE TABLE IF NOT EXISTS conv_deletes (
    owner_id VARCHAR(32) NOT NULL COMMENT '用户ID',
    conv_id VARCHAR(64) NOT NULL COMMENT '会话ID',
    deleted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '删除时间',
    PRIMARY KEY (owner_id, conv_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='会话删除水位表';

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