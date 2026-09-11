-- Lurkspire M3 数据库结构（MySQL 8）
-- 强制客户端 UTF-8——防导入中文种子乱码（Windows 客户端默认非 UTF-8）
SET NAMES utf8mb4;
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

-- 装备目录（背包物品定义——名称/描述/效果。改表即改装备，重启生效）
-- 注意：效果数值需与客户端 GameConfig.ApplyEquip 保持一致（M4 可迁移为下发驱动）
CREATE TABLE IF NOT EXISTS equipments (
    id           INT UNSIGNED PRIMARY KEY,           -- 0=无 1..n
    name         VARCHAR(32)  NOT NULL,
    description  VARCHAR(128) NOT NULL,
    effect_type  VARCHAR(32)  NOT NULL DEFAULT '',   -- none/air_jump/run_speed/magazine/block_max
    effect_value FLOAT        NOT NULL DEFAULT 0,    -- 效果数值（+0.3/+1/+4/+10）
    sort_order   INT          NOT NULL DEFAULT 0,    -- 展示顺序
    enabled      TINYINT      NOT NULL DEFAULT 1     -- 1=上架（0=隐藏，便于运营下架）
) ENGINE=InnoDB;

-- 装备种子（4 件全解锁；INSERT IGNORE 便于重复执行 schema）
INSERT IGNORE INTO equipments (id, name, description, effect_type, effect_value, sort_order) VALUES
    (0, '无',      '不装备任何物品',      '',          0,  0),
    (1, '轻盈之靴', '二段跳高度 +0.3m',   'air_jump',  0.3, 1),
    (2, '疾风护腕', '跑步速度 +1',        'run_speed', 1,   2),
    (3, '扩容弹匣', '弹夹子弹 +4',        'magazine',  4,   3),
    (4, '守护徽章', '格挡条上限 +10',     'block_max', 10,  4);
