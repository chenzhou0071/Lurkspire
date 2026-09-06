-- Lurkspire M3 数据库结构（MySQL 8）
CREATE DATABASE IF NOT EXISTS lurkspire DEFAULT CHARACTER SET utf8mb4;
USE lurkspire;

-- 用户：账号密码登录 + 昵称显示（独立）；装备持久化
CREATE TABLE IF NOT EXISTS users (
    id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    account       VARCHAR(64)  NOT NULL UNIQUE,
    password_hash VARCHAR(100) NOT NULL,          -- bcrypt
    nickname      VARCHAR(32)  NOT NULL,
    equip_id      INT          NOT NULL DEFAULT 0, -- 背包装备（0=无 1..4）
    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- 好友关系（双向：A-B 一条关系存两行，friend_a < friend_b 恒成立 + 各自方向视图查询）
CREATE TABLE IF NOT EXISTS friends (
    id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_a     BIGINT UNSIGNED NOT NULL,           -- 小 id
    user_b     BIGINT UNSIGNED NOT NULL,           -- 大 id
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_pair (user_a, user_b)
) ENGINE=InnoDB;
