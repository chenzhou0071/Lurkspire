-- Lurkspire M3 数据库结构（MySQL 8）
CREATE DATABASE IF NOT EXISTS lurkspire DEFAULT CHARACTER SET utf8mb4;
USE lurkspire;

-- 用户：账号密码登录 + 昵称显示（独立且唯一——好友按昵称搜索添加）；装备持久化
CREATE TABLE IF NOT EXISTS users (
    id            INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    account       VARCHAR(64)  NOT NULL UNIQUE,
    password_hash VARCHAR(100) NOT NULL,          -- bcrypt
    nickname      VARCHAR(32)  NOT NULL UNIQUE,   -- 昵称唯一（好友搜索凭据）
    equip_id      INT          NOT NULL DEFAULT 0, -- 背包装备（0=无 1..4）
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- 好友关系（单向申请 + 双向生效：user_a 申请加 user_b；status 0=申请中 1=好友）
CREATE TABLE IF NOT EXISTS friends (
    id         INT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_a     INT UNSIGNED NOT NULL,            -- 申请人
    user_b     INT UNSIGNED NOT NULL,            -- 被申请人
    status     TINYINT      NOT NULL DEFAULT 0,  -- 0=申请中 1=好友
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_pair (user_a, user_b)
) ENGINE=InnoDB;
