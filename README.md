# Lurkspire — 高机动 FPS 对战 服务端

高机动 FPS 对战游戏《Lurkspire》的服务端（Unity 客户端见 [Lurkspire_unity](https://github.com/chenzhou0071/Lurkspire_unity)）。Go 单进程架构：裸 TCP 长连接 + 自研二进制帧协议 + MySQL 持久化，支持 4-8 人同房实时对战。项目已完成"注册登录 → 大厅（好友/房间/背包）→ 对局 → 断线重连"完整闭环。

## 架构

```
Unity 客户端 ──裸 TCP 帧协议（:7777）──> gateway
                                          │  ├─ 连接层（Session/心跳/登录态）
                                          │  ├─ 大厅业务（账号/好友/房间列表/背包）
                                          │  └─ 房间 Manager（内嵌——单进程合一）
                                          │        └─ Room ×N（30Hz 单写者对局循环）
                                          └──────────── MySQL ────────────
                                                （账号/好友/装备）
```

| 组件 | 职责 |
|---|---|
| **gateway** | TCP 连接生命周期（心跳/超时踢出）、登录注册（bcrypt/HMAC Token）、好友/房间列表/背包装备、房间编排与广播路由 |
| **room** | 30Hz 单写者对局循环（goroutine + channel 串行化免锁）、移动验证、权威命中、断线宽限、状态广播 |
| **MySQL** | 账号/好友关系/装备目录与装备选择 |

## 协议设计

帧格式（12 字节头，大端序，与客户端 C# 镜像逐字节一致——跨语言 golden bytes 对拍）：

```
| magic 0x5344 (2B) | msgID (2B) | seq (4B) | bodyLen (4B) | body ... (≤64KB) |
```

消息号分区：

| 段 | 范围 | 内容 |
|---|---|---|
| 通用 | 1-99 | 心跳（9） |
| 大厅 | 200-299 | 登录/注册/重连（200-204）、好友（210-218）、房间列表（220-223）、背包（230-231） |
| 战斗 | 300-349 | 入房/入房应答（300/301）、输入上报（310）、状态广播（320）、命中/死亡（330/331）、玩家昵称（335） |

## 功能特性

- 实时对战：4-8 人同房 30Hz 状态广播；服务端权威裁决（伤害/击杀/重生/计分）
- 同步模型：客户端上报本地位置（含跳跃/跑墙等机动）+ 服务端验证采纳（位移上限/值域/浮点防御）——高机动玩法不产生预测漂移
- 权威命中：基于地图墙体包围盒（AABB）的射线求交遮挡判定，覆盖枪械射线/近战扇形/冲刺路径/锁定必中/格挡减伤
- 断线重连：30 秒宽限期保留玩家状态（离线标记），HMAC Token 免密重连续接位置/血量/击杀数；心跳保活 + 僵尸连接踢出
- 账号系统：注册/登录（bcrypt）、昵称唯一、HMAC Token（24h）
- 社交与大厅：好友申请栏（双向/在线实时推送/离线不丢）、房间列表（创建/备注/人数/散房广播）、背包装备（equipment 表驱动，改表即生效）
- 地图碰撞数据：由 Unity Editor 工具导出（墙体包围盒列表），服务端加载用于命中判定
- CF 式无限对局：无结算、房间打到人走光；空房自动销毁

## 快速开始

前置：Go 1.26+、MySQL 8。

```bash
# 1. 建库（一次）
mysql -u root -p < db/schema.sql

# 2. 配置（含数据库凭据——不提交）
cp config.json.example config.json   # 填 mysql_dsn / token_secret

# 3. 启动网关
go run ./cmd/gateway
# 输出：存储: MySQL ✓ / 服务已启动，监听 :7777
```

### Docker 部署

```bash
cp .env.example .env                              # 填数据库密码
cp config.docker.json.example config.docker.json  # 填同样密码 + token 密钥
docker compose up -d --build
docker compose logs -f gateway                    # 看启动日志
```

拉取镜像困难时（如国内网络），可改用预编译方案：本地交叉编译后打包（见 `Dockerfile.prebuilt` 注释，`.env` 中设置 `GATEWAY_DOCKERFILE=Dockerfile.prebuilt`）。

## 配置

| 文件 | 内容 | 是否提交 |
|---|---|---|
| `config.json` | 监听地址 / MySQL DSN / Token 密钥 | 否（模板 `config.json.example`） |
| `config.docker.json` | 容器部署用（DSN 指向 compose 服务名 `mysql:3306`） | 否（模板 `config.docker.json.example`） |
| `.env` | compose 数据库密码 / Dockerfile 选择 | 否（模板 `.env.example`） |

## 测试

```bash
go test ./... -race        # 全部单元测试与集成测试（-race 全绿是合入门槛）
```

覆盖：帧编解码 golden bytes、粘包拆包、房间容量/移动验证/权威命中/断线宽限与淘汰、网关集成（真 TCP 双客户端入房/好友流/背包/重连恢复/心跳超时）。

## 目录结构

```
├── cmd/gateway          网关进程入口（配置加载 + 组装）
├── db/schema.sql        数据库结构 + 装备种子（utf8mb4）
├── Dockerfile           多阶段构建（源码 → 最小运行镜像）
├── Dockerfile.prebuilt  预编译打包（无构建网络依赖）
├── docker-compose.yml   MySQL + gateway 一键编排
└── internal/
    ├── protocol         帧编解码 / 消息号 / 对局与大厅协议（两端契约）
    ├── gateway          连接层（Session/心跳/watchdog）+ 大厅路由 + 房间编排（Hub）
    ├── room             房间核心（30Hz 循环/移动验证/命中/宽限/广播）+ 地图墙盒
    ├── lobby            账号服务（注册/登录/Token/装备/好友透传）
    └── store            存储抽象（Mem 测试 / MySQL 生产，语义一致）
```

## 路线图

- M1 单机原型 ✅（客户端侧：机动链/武器/靶子）
- M2 联机对战 ✅（帧协议/房间循环/权威命中/广播）
- M3 账号与大厅 ✅（MySQL/好友/房间列表/背包/断线重连）
- M4 表现层打磨（进行中：地图美术/音效等，纯客户端）

设计文档与实施计划：`docs/superpowers/`
