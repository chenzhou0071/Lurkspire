# Lurkspire M1（单机原型）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lurkspire 单机原型——3D 高机动角色（快跑/跑墙/滑铲）+ 双枪（交替开火/充能锁头）+ 武士刀（两刀/冲刺斩/格挡条）全部机制可玩，配靶子假人与测试地图，验收标准：自己玩 100 局手感满意。

**Architecture:** 纯 Unity 单机（无服务器，M2 才接入）。脚本按职责分包：`Player`（角色控制）/ `Weapons`（双枪+刀）/ `Combat`（伤害/靶子）/ `UI`（HUD）/ `Core`（配置/对局管理）。数值全部集中在 `GameConfig`（ScriptableObject 或静态类），手感调参只改一处。命中用射线即时判定（无投射物）。逻辑类（交替序列/弹匣/格挡条数学/伤害）走 EditMode TDD；手感类（移动/跑墙/滑铲）手动 Play 调参。

**Tech Stack:** Unity 6（URP 3D）、新 Input System（Keyboard/Mouse.current）、C#、EditMode 测试（NUnit）

## Global Constraints

- 项目路径：新建 Unity 项目 `E:\pro\Lurkspire`（Unity 6 / URP 3D 模板，3D Core），**用户手动创建**
- 使用新 Input System API（`Keyboard.current` / `Mouse.current`）——旧 `Input.GetKey` 在 Unity 6 不可用
- 所有数值来自 `GameConfig`（唯一事实源，调手感只改它）：跑墙 3s、滑铲略快、双枪交替 4 发击杀、充能 10s 存 3 发、弹匣换弹、刀两刀、格挡条 100/挡-10/回 5/s/减伤 50%/砍人+20、血量 100、重生无敌 1s
- 测试：逻辑类 EditMode 测试（`-race` 不适用，Unity Test Runner）；手感类手动验收
- 每 Task 提交（`git add` + commit），提交前 `git status` 核对范围
- 中文字体未接入——**UI 全部英文**

---

### Task 1: 项目骨架与 GameConfig

**Files:**
- Create: `Assets/Scripts/Core/GameConfig.cs`
- Create: `Assets/Scripts/Core/GameManager.cs`
- Test: `Assets/Tests/EditMode/GameConfigTests.cs`
- Create: `Assets/Scripts/Core/GameConfig.cs.meta`（Unity 自动生成）

**Interfaces:**
- Consumes: 无（首个任务）
- Produces: `GameConfig`（静态类，全部武器/机动/对局数值）、`GameManager`（MonoBehaviour 单例：当前对局状态——击杀数/重生管理，后续任务消费）

- [ ] **Step 1: 用户创建 Unity 项目**

用户操作：Unity Hub → 新建项目 → Unity 6 → 3D (URP) 模板 → 路径 `E:\pro\Lurkspire`。创建后初始化 git：`git init` + 首次提交（`.gitignore` 用 Unity 官方模板）。

- [ ] **Step 2: 写失败测试**

`Assets/Tests/EditMode/GameConfigTests.cs`：
```csharp
using NUnit.Framework;

public class GameConfigTests
{
    [Test]
    public void WeaponNumbers_AsDesigned()
    {
        Assert.AreEqual(4, GameConfig.HitsToKill);          // 双枪 4 发
        Assert.AreEqual(25, GameConfig.GunDamage);          // 每发 25% = 100/4
        Assert.AreEqual(3, GameConfig.ChargeMax);           // 锁头存 3 发
        Assert.AreEqual(10f, GameConfig.ChargeSeconds);     // 10s 充能
        Assert.AreEqual(50, GameConfig.SwordDamage);        // 刀 50% = 两刀
        Assert.AreEqual(100, GameConfig.BlockMax);          // 格挡条
        Assert.AreEqual(10, GameConfig.BlockCostPerShot);   // 挡一枪 -10
        Assert.AreEqual(5f, GameConfig.BlockRegenPerSec);   // 回 5/s
        Assert.AreEqual(0.5f, GameConfig.BlockDamageMult);  // 减伤 50%
        Assert.AreEqual(20, GameConfig.BlockGainOnHit);     // 砍人 +20
        Assert.AreEqual(3f, GameConfig.WallRunSeconds);     // 跑墙 3s
        Assert.AreEqual(100, GameConfig.MaxHealth);         // 血量
        Assert.AreEqual(1f, GameConfig.SpawnInvulnSeconds); // 重生无敌
    }
}
```

- [ ] **Step 3: 跑测试确认失败**

Unity Test Runner → EditMode → Run All。预期：FAIL（GameConfig 未定义）。

- [ ] **Step 4: 实现 GameConfig**

`Assets/Scripts/Core/GameConfig.cs`：
```csharp
// GameConfig — 所有玩法数值的唯一事实源（调手感只改这里）
public static class GameConfig
{
    // 双枪
    public const int HitsToKill = 4;
    public const int GunDamage = 25;
    public const int ChargeMax = 3;
    public const float ChargeSeconds = 10f;
    public const int MagazineSize = 24;   // 弹匣 24 发
    public const float ReloadSeconds = 1.2f;
    public const float FireInterval = 0.09f; // 交替射速
    // 武士刀
    public const int SwordDamage = 50;
    public const float SwordRange = 3f;
    public const float SwordArcSeconds = 0.25f;
    public const float DashAttackRange = 6f;
    // 格挡
    public const int BlockMax = 100;
    public const int BlockCostPerShot = 10;
    public const float BlockRegenPerSec = 5f;
    public const float BlockDamageMult = 0.5f;
    public const int BlockGainOnHit = 20;
    // 机动
    public const float RunSpeed = 12f;
    public const float WallRunSeconds = 3f;
    public const float WallRunSpeedMult = 0.85f;
    public const float SlideSpeedMult = 1.1f;
    public const float JumpHeight = 2f;
    // 对局
    public const int MaxHealth = 100;
    public const float SpawnInvulnSeconds = 1f;
    public const float RespawnSeconds = 2f;
}
```

`Assets/Scripts/Core/GameManager.cs`（骨架，后续任务填充）：
```csharp
using UnityEngine;

// GameManager — 单机对局管理：重生/计分（骨架，M1 后续任务填充）
public class GameManager : MonoBehaviour
{
    public static GameManager I { get; private set; }
    public int Kills { get; private set; }

    private void Awake()
    {
        if (I != null) { Destroy(gameObject); return; }
        I = this;
    }

    public void RegisterKill() => Kills++;
}
```

- [ ] **Step 5: 跑测试确认通过**

Test Runner → EditMode → Run All。预期：GameConfigTests 全 PASS。

- [ ] **Step 6: 提交**

```bash
cd E:\pro\Lurkspire
git add Assets/Scripts Assets/Tests
git commit -m "feat: 项目骨架与 GameConfig 数值配置"
```

---

### Task 2: PlayerMotor——快跑/跳跃/滑铲

**Files:**
- Create: `Assets/Scripts/Player/PlayerMotor.cs`
- Create: `Assets/Scripts/Player/PlayerInput.cs`
- Test: `Assets/Tests/EditMode/PlayerMotorTests.cs`

**Interfaces:**
- Consumes: `GameConfig.RunSpeed/JumpHeight/SlideSpeedMult`
- Produces: `PlayerMotor`（MonoBehaviour：`Move(Vector2 input, bool jump, bool slide)`——纯逻辑可测；速度计算/滑铲状态）、`PlayerInput`（读取 Keyboard.current，驱动 PlayerMotor）

- [ ] **Step 1: 写失败测试**

`PlayerMotorTests.cs`（测纯逻辑部分——速度计算与滑铲状态机）：
```csharp
using NUnit.Framework;

public class PlayerMotorTests
{
    [Test]
    public void RunSpeed_MatchesConfig()
    {
        Assert.AreEqual(GameConfig.RunSpeed, PlayerMotor.ComputeSpeed(false, false));
    }

    [Test]
    public void SlideSpeed_SlightlyFaster()
    {
        float run = PlayerMotor.ComputeSpeed(false, false);
        float slide = PlayerMotor.ComputeSpeed(false, true);
        Assert.Greater(slide, run); // 滑铲比跑步略快
        Assert.Less(slide, run * 1.5f); // 但不多
    }

    [Test]
    public void Slide_EndsAfterTimeout()
    {
        var motor = new PlayerMotor(); // 纯逻辑状态（无 MonoBehaviour）
        motor.SetSlide(true);
        motor.TickSlide(0.5f);
        Assert.IsTrue(motor.IsSliding);
        motor.TickSlide(1.0f); // 超过滑铲时长
        Assert.IsFalse(motor.IsSliding);
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

预期：FAIL（PlayerMotor 未定义）。

- [ ] **Step 3: 实现**

`PlayerMotor.cs`——静态速度计算 + 滑铲状态（纯逻辑部分）：
```csharp
using UnityEngine;

// PlayerMotor — 角色移动核心：速度计算/滑铲状态（纯逻辑可测）
public class PlayerMotor
{
    public const float SlideDuration = 0.8f;
    private float _slideTimer;
    public bool IsSliding { get; private set; }

    public static float ComputeSpeed(bool wallRunning, bool sliding)
    {
        float speed = GameConfig.RunSpeed;
        if (wallRunning) speed *= GameConfig.WallRunSpeedMult;
        if (sliding) speed *= GameConfig.SlideSpeedMult;
        return speed;
    }

    public void SetSlide(bool slide)
    {
        if (slide && !IsSliding) _slideTimer = SlideDuration;
        IsSliding = slide;
    }

    public void TickSlide(float dt)
    {
        if (!IsSliding) return;
        _slideTimer -= dt;
        if (_slideTimer <= 0f) IsSliding = false;
    }
}
```

`PlayerInput.cs`（MonoBehaviour，驱动 PlayerMotor + CharacterController，手感部分手动调）：
```csharp
using UnityEngine;
using UnityEngine.InputSystem;

// PlayerInput — 输入采集 + 角色驱动（新 Input System）
[RequireComponent(typeof(CharacterController))]
public class PlayerInput : MonoBehaviour
{
    [SerializeField] private float gravity = -20f;
    private CharacterController _cc;
    private PlayerMotor _motor;
    private Vector3 _velocity;

    private void Awake()
    {
        _cc = GetComponent<CharacterController>();
        _motor = new PlayerMotor();
    }

    private void Update()
    {
        var kb = Keyboard.current;
        Vector2 move = Vector2.zero;
        if (kb != null)
        {
            if (kb.wKey.isPressed) move.y += 1;
            if (kb.sKey.isPressed) move.y -= 1;
            if (kb.aKey.isPressed) move.x -= 1;
            if (kb.dKey.isPressed) move.x += 1;
        }
        bool slide = kb != null && kb.leftShiftKey.isPressed;
        _motor.SetSlide(slide);
        _motor.TickSlide(Time.deltaTime);

        float speed = PlayerMotor.ComputeSpeed(false, _motor.IsSliding);
        Vector3 dir = (transform.right * move.x + transform.forward * move.y).normalized;
        _velocity.x = dir.x * speed;
        _velocity.z = dir.z * speed;
        _velocity.y += gravity * Time.deltaTime;
        if (kb != null && kb.spaceKey.wasPressedThisFrame && _cc.isGrounded)
            _velocity.y = Mathf.Sqrt(GameConfig.JumpHeight * -2f * gravity);
        _cc.Move(_velocity * Time.deltaTime);
    }
}
```

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 场景验证（用户）**

TestArena 场景（Task 10 建，或先建最小场景）：放 Player（胶囊体 + CharacterController + PlayerInput）、地面、摄像机跟随。Play 验证：WASD 移动、空格跳、Shift 滑铲略加速。

- [ ] **Step 6: 提交**

```bash
git add Assets/Scripts/Player Assets/Tests
git commit -m "feat(player): 快跑/跳跃/滑铲机动基础"
```

---

### Task 3: 跑墙（WallRun）

**Files:**
- Create: `Assets/Scripts/Player/WallRun.cs`
- Test: `Assets/Tests/EditMode/WallRunTests.cs`

**Interfaces:**
- Consumes: `GameConfig.WallRunSeconds/WallRunSpeedMult`、`PlayerMotor.ComputeSpeed`
- Produces: `WallRun`（`bool TryEnterWallRun(Vector3 normal)` / `void Tick(float dt, out bool refreshable)`——跑墙状态机：3 秒计时、换墙/落地刷新）

- [ ] **Step 1: 写失败测试**

`WallRunTests.cs`：
```csharp
using NUnit.Framework;

public class WallRunTests
{
    [Test]
    public void WallRun_Duration_ThreeSeconds()
    {
        var wr = new WallRun();
        wr.Enter(Vector3.right);
        Assert.IsTrue(wr.IsWallRunning);
        wr.Tick(2f);
        Assert.IsTrue(wr.IsWallRunning);
        wr.Tick(1.5f); // 累计超过 3s
        Assert.IsFalse(wr.IsWallRunning);
    }

    [Test]
    public void WallRun_ReEnter_Refreshes()
    {
        var wr = new WallRun();
        wr.Enter(Vector3.right);
        wr.Tick(2.5f);
        wr.Enter(Vector3.forward); // 换墙 → 刷新计时
        Assert.IsTrue(wr.IsWallRunning);
        wr.Tick(2f);
        Assert.IsTrue(wr.IsWallRunning); // 3s 从换墙重新算
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现**

`WallRun.cs`：
```csharp
using UnityEngine;

// WallRun — 跑墙状态机：3 秒计时，换墙/落地刷新
public class WallRun
{
    private float _timer;
    private Vector3 _wallNormal;
    public bool IsWallRunning { get; private set; }

    public void Enter(Vector3 wallNormal)
    {
        _wallNormal = wallNormal;
        _timer = GameConfig.WallRunSeconds; // 换墙/上墙刷新
        IsWallRunning = true;
    }

    public void Exit() => IsWallRunning = false;

    public void Tick(float dt)
    {
        if (!IsWallRunning) return;
        _timer -= dt;
        if (_timer <= 0f) IsWallRunning = false;
    }

    // 跑墙方向：沿墙面切线（法线叉积上方向）
    public Vector3 RunDirection(Vector3 inputDir)
    {
        Vector3 tangent = Vector3.Cross(Vector3.up, _wallNormal).normalized;
        return Vector3.Dot(inputDir, tangent) >= 0 ? tangent : -tangent;
    }
}
```

`PlayerInput.cs` 接入（修改）：Update 里射线检测右侧墙面 → `wr.Enter(hitNormal)`；`wr.Tick`；跑墙中速度用 `RunDirection`。

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 场景验证（用户）**

测试地图放一面高墙，Play：靠近墙自动跑墙 3 秒，跳开再上墙刷新。

- [ ] **Step 6: 提交**

```bash
git add Assets/Scripts/Player Assets/Tests
git commit -m "feat(player): 跑墙机动（3秒/换墙刷新）"
```

---

### Task 4: GunController——交替开火与弹匣

**Files:**
- Create: `Assets/Scripts/Weapons/GunController.cs`
- Create: `Assets/Scripts/Weapons/WeaponSwitch.cs`
- Test: `Assets/Tests/EditMode/GunControllerTests.cs`

**Interfaces:**
- Consumes: `GameConfig.HitsToKill/GunDamage/MagazineSize/ReloadSeconds/FireInterval`
- Produces: `GunController`（`bool TryFire(out int shooter)`——交替序列与弹匣逻辑；`void Reload()`；`int Ammo` / `int Charge`）、`WeaponSwitch`（1=枪 2=刀，滚轮切换）

- [ ] **Step 1: 写失败测试**

`GunControllerTests.cs`：
```csharp
using NUnit.Framework;

public class GunControllerTests
{
    [Test]
    public void Fire_AlternatesBlackWhite()
    {
        var gun = new GunController();
        gun.TryFire(out int first);
        gun.TryFire(out int second);
        Assert.AreEqual(0, first);   // 黑枪
        Assert.AreEqual(1, second);  // 白枪
    }

    [Test]
    public void Magazine_Empty_BlocksFire()
    {
        var gun = new GunController();
        for (int i = 0; i < GameConfig.MagazineSize; i++) gun.TryFire(out _);
        Assert.IsFalse(gun.TryFire(out _)); // 弹匣空
        gun.Reload();
        Assert.IsTrue(gun.TryFire(out _));
    }

    [Test]
    public void Cooldown_BlocksInstantFire()
    {
        var gun = new GunController();
        gun.TryFire(out _);
        Assert.IsFalse(gun.TryFire(out _)); // 冷却未到
        gun.Tick(GameConfig.FireInterval + 0.01f);
        Assert.IsTrue(gun.TryFire(out _));
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现**

`GunController.cs`（逻辑部分）：
```csharp
// GunController — 双枪逻辑：交替序列/弹匣/冷却（纯逻辑可测）
public class GunController
{
    private int _alternate;
    private float _cooldown;
    public int Ammo { get; private set; } = GameConfig.MagazineSize;

    public bool TryFire(out int shooter)
    {
        shooter = _alternate;
        if (Ammo <= 0 || _cooldown > 0f) return false;
        _alternate = 1 - _alternate; // 黑白交替
        Ammo--;
        _cooldown = GameConfig.FireInterval;
        return true;
    }

    public void Reload() => Ammo = GameConfig.MagazineSize;

    public void Tick(float dt)
    {
        if (_cooldown > 0f) _cooldown -= dt;
    }
}
```

`GunController` MonoBehaviour 部分（射线命中 → DamageSystem，Task 7 接）+ `WeaponSwitch.cs`（滚轮/按键切换枪刀，启用对应 GameObject）。

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add Assets/Scripts/Weapons Assets/Tests
git commit -m "feat(weapons): 双枪交替开火/弹匣/冷却"
```

---

### Task 5: 充能锁头

**Files:**
- Test: `Assets/Tests/EditMode/ChargeTests.cs`
- Modify: `Assets/Scripts/Weapons/GunController.cs`

**Interfaces:**
- Consumes: `GameConfig.ChargeMax/ChargeSeconds`
- Produces: `GunController.TryLockOn(out int shooter)`（消耗一发充能，返回黑白交替枪号；`Charge` 属性；`Tick` 中充能累计）

- [ ] **Step 1: 写失败测试**

`ChargeTests.cs`：
```csharp
using NUnit.Framework;

public class ChargeTests
{
    [Test]
    public void Charge_Accumulates_TenSeconds()
    {
        var gun = new GunController();
        gun.Tick(10f);
        Assert.AreEqual(1, gun.Charge);
    }

    [Test]
    public void Charge_Max_Three()
    {
        var gun = new GunController();
        for (int i = 0; i < 5; i++) gun.Tick(10f);
        Assert.AreEqual(GameConfig.ChargeMax, gun.Charge); // 存 3 发封顶
    }

    [Test]
    public void LockOn_Consumes_AndAlternates()
    {
        var gun = new GunController();
        gun.Tick(10f);
        gun.TryLockOn(out int first);
        gun.Tick(10f);
        gun.TryLockOn(out int second);
        Assert.AreEqual(0, gun.Charge);
        Assert.AreEqual(0, first);  // 第 1 次右键白枪（交替序列从白开始）
        Assert.AreEqual(1, second); // 第 2 次黑枪
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现（修改 GunController）**

```csharp
private float _chargeTimer;
public int Charge { get; private set; }

public void Tick(float dt)
{
    if (_cooldown > 0f) _cooldown -= dt;
    if (Charge < GameConfig.ChargeMax)
    {
        _chargeTimer += dt;
        if (_chargeTimer >= GameConfig.ChargeSeconds)
        {
            _chargeTimer = 0f;
            Charge++;
        }
    }
}

public bool TryLockOn(out int shooter)
{
    shooter = _alternate;
    if (Charge <= 0) return false;
    _alternate = 1 - _alternate;
    Charge--;
    return true;
}
```

MonoBehaviour 部分：右键 → `TryLockOn` → 射线"必中"（自瞄：向最近玩家/靶子方向发射，命中保证——单机阶段直接向视线最近目标判定）。

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add Assets/Scripts/Weapons Assets/Tests
git commit -m "feat(weapons): 充能锁头（10s/存3发/黑白交替）"
```

---

### Task 6: SwordController——两刀/冲刺斩/格挡条

**Files:**
- Create: `Assets/Scripts/Weapons/SwordController.cs`
- Test: `Assets/Tests/EditMode/SwordTests.cs`

**Interfaces:**
- Consumes: `GameConfig.SwordDamage/SwordRange/SwordArcSeconds/DashAttackRange/BlockMax/BlockCostPerShot/BlockRegenPerSec/BlockDamageMult/BlockGainOnHit`
- Produces: `SwordController`（`bool TrySwing()`——近战攻击冷却；`bool TryDashAttack()`；格挡状态机 `bool IsBlocking` / `float Block` / `bool BlockDamage(ref int damage)` / `void GainBlock(int amount)` / `Tick` 回条）

- [ ] **Step 1: 写失败测试**

`SwordTests.cs`：
```csharp
using NUnit.Framework;

public class SwordTests
{
    [Test]
    public void Block_DamageReduced_FiftyPercent()
    {
        var sword = new SwordController();
        sword.Block = 100;
        sword.IsBlocking = true;
        int dmg = 25;
        sword.BlockDamage(ref dmg);
        Assert.AreEqual(12, dmg); // 50% 减伤（25 → 12，整数向下）
        Assert.AreEqual(90, sword.Block); // 挡一枪 -10
    }

    [Test]
    public void Block_Regen_FivePerSec()
    {
        var sword = new SwordController();
        sword.Block = 50;
        sword.Tick(2f);
        Assert.AreEqual(60, sword.Block);
    }

    [Test]
    public void Hit_Enemy_GainsTwenty()
    {
        var sword = new SwordController();
        sword.Block = 10;
        sword.GainBlock(GameConfig.BlockGainOnHit);
        Assert.AreEqual(30, sword.Block);
    }

    [Test]
    public void Block_Empty_NoReduction()
    {
        var sword = new SwordController();
        sword.Block = 5;
        sword.IsBlocking = true;
        int dmg = 25;
        sword.BlockDamage(ref dmg);
        Assert.AreEqual(25, dmg); // 条不够 10：不触发减伤（或扣光后全额）
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现**

`SwordController.cs`（逻辑部分）：
```csharp
// SwordController — 武士刀逻辑：攻击冷却/格挡条（纯逻辑可测）
public class SwordController
{
    private float _swingCooldown;
    private float _dashCooldown;
    public bool IsBlocking { get; set; }
    public float Block { get; set; } = GameConfig.BlockMax;

    public bool TrySwing()
    {
        if (_swingCooldown > 0f) return false;
        _swingCooldown = GameConfig.SwordArcSeconds;
        return true;
    }

    public bool TryDashAttack()
    {
        if (_dashCooldown > 0f) return false;
        _dashCooldown = 1.5f;
        return true;
    }

    // 格挡伤害：条够扣 10 则减伤 50%，返回是否成功格挡
    public bool BlockDamage(ref int damage)
    {
        if (!IsBlocking || Block < GameConfig.BlockCostPerShot) return false;
        Block -= GameConfig.BlockCostPerShot;
        damage = Mathf.FloorToInt(damage * GameConfig.BlockDamageMult);
        return true;
    }

    public void GainBlock(int amount) =>
        Block = Mathf.Min(GameConfig.BlockMax, Block + amount);

    public void Tick(float dt)
    {
        if (_swingCooldown > 0f) _swingCooldown -= dt;
        if (_dashCooldown > 0f) _dashCooldown -= dt;
        if (Block < GameConfig.BlockMax)
            Block = Mathf.Min(GameConfig.BlockMax, Block + GameConfig.BlockRegenPerSec * dt);
    }
}
```

MonoBehaviour 部分：左键近战扇形射线（命中敌人 → DamageSystem 50 + `GainBlock(20)`）、Shift 冲刺斩（位移 + 前方 6m 范围）、右键格挡（IsBlocking）。需要 `using UnityEngine;`（Mathf）。

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add Assets/Scripts/Weapons Assets/Tests
git commit -m "feat(weapons): 武士刀两刀/冲刺斩/格挡条（减伤50/回5/砍人+20）"
```

---

### Task 7: DamageSystem 与血量

**Files:**
- Create: `Assets/Scripts/Combat/DamageSystem.cs`
- Create: `Assets/Scripts/Combat/Health.cs`
- Test: `Assets/Tests/EditMode/DamageSystemTests.cs`

**Interfaces:**
- Consumes: `GameConfig.MaxHealth`、`SwordController.BlockDamage`、`GunController`（伤害来源）
- Produces: `Health`（`void TakeDamage(int amount, SwordController blocker)`——格挡介入后扣血；`int HP`；`bool IsDead`；死亡回调）、`DamageSystem`（静态：`ApplyHit(Health target, int damage, SwordController blocker)`）

- [ ] **Step 1: 写失败测试**

`DamageSystemTests.cs`：
```csharp
using NUnit.Framework;

public class DamageSystemTests
{
    [Test]
    public void FourGunShots_Kill()
    {
        var health = new Health();
        for (int i = 0; i < 4; i++) health.TakeDamage(GameConfig.GunDamage, null);
        Assert.IsTrue(health.IsDead);
    }

    [Test]
    public void TwoSwordHits_Kill()
    {
        var health = new Health();
        health.TakeDamage(GameConfig.SwordDamage, null);
        health.TakeDamage(GameConfig.SwordDamage, null);
        Assert.IsTrue(health.IsDead);
    }

    [Test]
    public void Blocking_ReducesDamage()
    {
        var health = new Health();
        var sword = new SwordController();
        sword.IsBlocking = true;
        health.TakeDamage(25, sword);
        Assert.AreEqual(100 - 12, health.HP); // 25 → 12
        Assert.AreEqual(90, sword.Block);     // 条 -10
    }
}
```

- [ ] **Step 2: 跑测试确认失败**

- [ ] **Step 3: 实现**

`Health.cs` + `DamageSystem.cs`：
```csharp
using System;

// Health — 血量：伤害经格挡介入后扣血
public class Health
{
    public int HP { get; private set; } = GameConfig.MaxHealth;
    public bool IsDead => HP <= 0;
    public event Action OnDeath;

    public void TakeDamage(int damage, SwordController blocker)
    {
        if (IsDead) return;
        if (blocker != null) blocker.BlockDamage(ref damage);
        HP -= damage;
        if (HP <= 0)
        {
            HP = 0;
            OnDeath?.Invoke();
        }
    }

    public void Reset() => HP = GameConfig.MaxHealth;
}

// DamageSystem — 命中入口（静态，射线命中后调用）
public static class DamageSystem
{
    public static void ApplyHit(Health target, int damage, SwordController blocker)
        => target.TakeDamage(damage, blocker);
}
```

- [ ] **Step 4: 跑测试确认通过**

- [ ] **Step 5: 提交**

```bash
git add Assets/Scripts/Combat Assets/Tests
git commit -m "feat(combat): 伤害系统与血量（格挡介入/4发击杀/两刀击杀）"
```

---

### Task 8: TargetDummy 靶子

**Files:**
- Create: `Assets/Scripts/Combat/TargetDummy.cs`

**Interfaces:**
- Consumes: `Health`、`DamageSystem`
- Produces: `TargetDummy`（MonoBehaviour：挂 Health；`OnDeath` → 倒下动画/重置计时 3 秒后恢复；`HitsTaken` 计数供 HUD）

- [ ] **Step 1: 实现**

`TargetDummy.cs`：
```csharp
using UnityEngine;

// TargetDummy — 靶子：可击倒、3 秒后恢复、计数（单人练枪/伤害验证）
public class TargetDummy : MonoBehaviour
{
    public int HitsTaken { get; private set; }
    private Health _health;
    private float _resetTimer;
    private Vector3 _up;

    private void Awake()
    {
        _health = new Health();
        _health.OnDeath += OnDummyDeath;
        _up = transform.up;
    }

    private void Update()
    {
        if (_health.IsDead)
        {
            _resetTimer -= Time.deltaTime;
            if (_resetTimer <= 0f) _health.Reset();
        }
        // 倒下/恢复表现：绕 X 轴旋转
        float target = _health.IsDead ? 90f : 0f;
        transform.rotation = Quaternion.RotateTowards(transform.rotation,
            Quaternion.Euler(target, 0, 0), 120f * Time.deltaTime);
    }

    private void OnDummyDeath()
    {
        HitsTaken++;
        _resetTimer = 3f;
    }

    // 射线命中入口（GunController 调用）
    public void Hit(int damage) => DamageSystem.ApplyHit(_health, damage, null);
}
```

- [ ] **Step 2: 场景放置（用户）**

测试地图放 3-4 个 TargetDummy（胶囊体 + 组件），验证：开枪倒下、3 秒恢复、计数增加。

- [ ] **Step 3: 提交**

```bash
git add Assets/Scripts/Combat
git commit -m "feat(combat): 靶子假人（击倒/恢复/计数）"
```

---

### Task 9: HUD

**Files:**
- Create: `Assets/Scripts/UI/HUD.cs`
- Modify: `Assets/Scripts/Weapons/GunController.cs`、`Assets/Scripts/Weapons/SwordController.cs`（暴露状态属性——已暴露）

**Interfaces:**
- Consumes: `GunController.Ammo/Charge`、`SwordController.Block/IsBlocking`、`Health.HP`、`GameManager.Kills`
- Produces: `HUD`（MonoBehaviour：TMP 文本显示 弹药/充能/格挡条/血量/击杀数）

- [ ] **Step 1: 实现**

`HUD.cs`（英文 UI，中文字体未接入）：
```csharp
using TMPro;
using UnityEngine;

// HUD — 状态显示：弹药/充能/格挡/血量/击杀（英文，中文字体未接入）
public class HUD : MonoBehaviour
{
    [SerializeField] private TMP_Text ammoText;
    [SerializeField] private TMP_Text chargeText;
    [SerializeField] private TMP_Text blockText;
    [SerializeField] private TMP_Text hpText;
    [SerializeField] private TMP_Text killsText;
    [SerializeField] private GunController gun;
    [SerializeField] private SwordController sword;
    [SerializeField] private Health playerHealth;

    private void Update()
    {
        if (ammoText != null) ammoText.text = $"AMMO {gun.Ammo}";
        if (chargeText != null) chargeText.text = $"LOCK {gun.Charge}/3";
        if (blockText != null) blockText.text = sword.IsBlocking
            ? $"BLOCK {sword.Block:F0}" : $"block {sword.Block:F0}";
        if (hpText != null) hpText.text = $"HP {playerHealth.HP}";
        if (killsText != null && GameManager.I != null)
            killsText.text = $"KILLS {GameManager.I.Kills}";
    }
}
```

- [ ] **Step 2: 场景搭建（用户）**

Canvas（Screen Space）→ 左上 TMP 文本 ×5（AMMO/LOCK/BLOCK/HP/KILLS）→ 拖到 HUD 槽。

- [ ] **Step 3: 提交**

```bash
git add Assets/Scripts/UI
git commit -m "feat(ui): HUD 弹药/充能/格挡/血量/击杀"
```

---

### Task 10: TestArena 地图与场景组装

**Files:**
- Create: `Assets/Scenes/TestArena.unity`（用户搭建）
- Create: `Assets/Scripts/Core/PlayerSpawner.cs`（可选：出生点管理）

**Interfaces:**
- Consumes: 全部前序组件
- Produces: 可玩场景（Play 即玩）

- [ ] **Step 1: 用户搭建场景**

```text
1. File → New Scene → Basic（3D）→ 保存 Assets/Scenes/TestArena.unity
2. 地面：Plane/Cube（100×50 灰材质）+ 若干墙体（跑墙路线：连续高墙 2 处 + 高低差台子）
3. Player：胶囊体 + CharacterController + PlayerInput + 武器两个子物体（Gun/Sword，Task 4/6 的 MonoBehaviour 部分）
4. 摄像机：第三人称跟随（简单：Follow 脚本或 Cinemachine——选 Cinemachine 省事）
5. 靶子 ×3：TargetDummy
6. Canvas + HUD（Task 9）
7. GameManager 空物体
```

- [ ] **Step 2: 玩家验证全机制（Play 通关清单）**

```text
□ WASD 快跑（速度明显快）
□ 空格跳
□ Shift 滑铲（略加速）
□ 贴墙跑墙 3 秒、跳开再上墙刷新
□ 左键双枪交替开火（火光左右交替）、弹匣打空换弹（R 键）
□ 右键锁头（充能 10s、存 3 发、HUD 显示）
□ 滚轮切刀：左键两刀、Shift 冲刺斩、右键格挡（挡枪减伤、格挡条下降/恢复、砍靶回条）
□ 靶子：4 枪/2 刀倒下 → 3 秒恢复 → HitsTaken 增加
□ 击杀计数（靶子是否计分——设计上靶子不计分，击杀数为 0 正常）
```

- [ ] **Step 3: 提交**

```bash
git add Assets/Scenes Assets/Scripts
git commit -m "feat(arena): TestArena 场景与全机制组装"
```

---

### Task 11: 手感打磨与验收

**Files:**
- Modify: `Assets/Scripts/Core/GameConfig.cs`（数值调整）

- [ ] **Step 1: 用户玩 100 局（Play 反复测试）**

重点手感项：
```text
□ 快跑速度（12 → 试 10-14，找到"快但可控"）
□ 跑墙 3 秒 + 切线速度（0.85 倍——是否太慢/太快）
□ 滑铲（1.1 倍——"略快"是否感知明显）
□ 双枪射速（0.09s 间隔——交替节奏是否爽）
□ 锁头充能 10s——是否卡节奏
□ 刀攻击范围 3m、冲刺斩 6m
□ 格挡手感：减伤 50%、砍人回 20
```

每项调整 = 改 `GameConfig` 一行 + Play 验证（**不写死在脚本**）。

- [ ] **Step 2: 手感记录**

把最终手感数值更新到 `GameConfig.cs` 注释 + 设计文档对应节。

- [ ] **Step 3: 提交**

```bash
git add Assets/Scripts/Core/GameConfig.cs docs
git commit -m "feat(tuning): M1 手感调参定稿"
```

- [ ] **Step 4: M1 验收**

```text
验收标准：自己玩 100 局不腻——机动链组合顺畅、双枪/刀各有用途、锁头有底牌感
通过 → M1 完成，进入 M2（服务器）
```

---

## M2-M4 大纲（后续计划细化）

### M2：服务器（复用 SignalDrift 架构）
- 网关（登录/匹配）复用 + 房间改 4-8 人广播
- 移动：客户端上报位置/速度，服务端校验（速度上限 + 瞬移检测）
- 命中：客户端上报开火 → 服务端射线判定 → 伤害广播
- 状态同步：30Hz × N 玩家位置/朝向/动画

### M3：联机打磨
- 多人实测、同步手感、延迟补偿
- 重生/计分/结算（击杀榜/命中率）

### M4：作品集打磨
- 地图精修（跑墙路线/交锋点）
- 美术完整（人物建模/12 个动画/双枪入场景）
- 演示视频录制

## Self-Review 备注

- 规格覆盖：机动（T2/T3）、双枪（T4/T5）、刀（T6）、伤害（T7）、靶子（T8）、HUD（T9）、地图（T10）、手感（T11）——M1 全覆盖
- 数值一致性：GameConfig 常量名在测试与实现中一致（HitsToKill/GunDamage/ChargeMax/BlockCostPerShot 等）
- 类型一致：SwordController.BlockDamage(ref int) 在 T6 定义、T7 测试消费，签名一致
