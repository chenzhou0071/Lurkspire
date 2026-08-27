# Lurkspire M4（作品集打磨）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 求职作品集最终形态——地图精修（跑墙路线/交锋点）、美术完整（人物建模/12 个动画/双枪入场景）、音效特效、演示视频。验收：录一段 60-90 秒流畅演示，能直接放进简历/面试展示。

**Architecture:** 表现层全面升级：美术资产替换占位、地图按设计精修、特效/音效补全、演示模式（靶子房/单人演练）。玩法与同步逻辑不动（M1-M3 已定），只做表现与内容。

**Tech Stack:** Unity 6（URP 3D）、Blender（低多边形建模）、动画（Humanoid 或 Legacy 手 K）

## Global Constraints

- 玩法数值/同步逻辑不改（M1-M3 已验收）——M4 只做表现与内容
- 美术风格：低多边形 + 平色材质（风格统一，与双枪建模一致）
- 动画清单（12 个）：跑/跳/滑铲/跑墙/出刀/冲刺斩/格挡/换弹/蓄力/受击/死亡/重生
- 地图：一张精图（灰墙 + 大空地 + 跑墙路线 + 高低差 + 3-4 个交锋点）
- 中文字体仍未接入——UI 英文
- 演示视频：60-90 秒，1080p

---

### Task 1: 地图精修

**Files:**
- Modify（Unity）: `Assets/Scenes/TestArena.unity` → 正式地图 `Assets/Scenes/Arena.unity`
- Create: `Assets/Art/Environment/`（灰墙/地面/台子低模材质）

**验收：**
```text
□ 跑墙路线：至少 2 条连续墙段（3 秒可跑通 + 可跳转）
□ 高低差：2-3 层（滑铲下坡/跑墙上墙）
□ 掩体：交错分布（对枪有掩护可绕）
□ 交锋点：3-4 个（出生点附近/中央/高台）
□ 视觉：灰墙 + 大空地，低多边形风格统一
```

### Task 2: 人物建模与绑定

**Files:**
- Create: `Assets/Art/Character/`（低多边形人形 + 材质）

**验收：** 角色模型替换占位胶囊体；低多边形风格与双枪一致；绑定可驱动动画。

### Task 3: 动画集（12 个）

**Files:**
- Create: `Assets/Art/Animations/`（12 个动画剪辑 + Animator Controller）

**验收：** 动画状态机覆盖：移动/机动链/武器切换/攻击/受击/死亡——每个动画与手感匹配（滑铲/跑墙/冲刺斩要踩点）。

### Task 4: 双枪入场景 + 武器表现

**Files:**
- Create: `Assets/Art/Weapons/`（双枪建模导入——黑枪/白枪）
- Modify: `Assets/Scripts/Weapons/GunController.cs`（枪口火光/交替后坐/换弹表现）、`SwordController.cs`（刀光/格挡特效）

**验收：** 黑枪白枪挂在角色左右手；开火交替火光清晰；换弹动画；蓄力锁头有枪口充能表现（设计文档要求）。

### Task 5: 音效与特效

**Files:**
- Create: `Assets/Art/Audio/`（枪声×2/刀挥/格挡/命中/死亡/跳/滑铲/跑墙/蓄力）
- Create: `Assets/Scripts/FX/`（命中伤害数字/击杀提示/受击红闪）

**验收：** 全部动作有反馈（声音 + 视觉），手感闭环。

### Task 6: 演示模式

**Files:**
- Create: `Assets/Scripts/UI/MainMenu.cs`（主菜单：开始对局/单人演练/设置）
- Modify: `GameManager`（演示模式：单人 + 靶子 + 自由机动）

**验收：** 打开游戏 10 秒内能开始玩；单人演练可展示全机制（跑墙链/双枪/刀/靶子）。

### Task 7: 演示视频与收尾

- [ ] 用户录制 60-90 秒演示（1080p）

```text
演示脚本建议：
0-10s  主菜单 → 单人演练（机动链展示：跑墙/滑铲）
10-30s 双枪连射 + 锁头秒杀靶子
30-50s 切刀：冲刺斩 + 格挡反击
50-70s 联机片段（4 人局击杀榜）
```

- [ ] 项目收尾：README（架构/运行说明）、最终提交

---

## Self-Review 备注

- 规格覆盖：地图（T1）、美术完整（T2-T5）、演示（T6/T7）——设计文档"作品集打磨"全覆盖
- 依赖：T2/T3 建模动画工作量大（用户主导），代码侧 T4/T5 并行
- M4 结束后可选：写 Lurkspire 开发日志（第七期起）、简历项目描述更新
