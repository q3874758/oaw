# OAW (OpenClaw Agent Work)

> 让 AI 的贡献像比特币一样可验证

## 功能

- 💰 钱包管理 - 创建、签名、AES加密存储
- ⛏️ 挖矿 - 工作量证明 (PoW)
- 📊 工作量追踪 - 量化 AI 贡献
- 🔗 OpenClaw 集成 - 同步实际工作量
- ⛓️ PoLE 链集成 - 链上验证
- 📈 Web Dashboard - 可视化统计
- 💾 数据备份导出 - JSON/CSV 格式

## 快速开始

```bash
# 克隆
git clone https://github.com/q3874758/oaw.git
cd oaw

# 初始化
./bin/oaw init

# 创建钱包 (使用 secp256k1 曲线，与 PoLE 链兼容)
./bin/oaw wallet create

# 开始挖矿 (自动启动 PoLE 节点)
./bin/oaw mine start

# 同步 OpenClaw 工作量
./bin/oaw sync

# 同步到 PoLE 链
./bin/oaw pole sync

# 启动 Web Dashboard
./bin/oaw dashboard 8080
```

## 命令

| 命令 | 说明 |
|------|------|
| `oaw init` | 初始化数据目录 |
| `oaw wallet create [name]` | 创建钱包 (默认: default) |
| `oaw wallet list` | 列出钱包 |
| `oaw wallet balance` | 查看余额 |
| `oaw mine start` | 开始挖矿 (自动启动 PoLE 节点) |
| `oaw mine stop` | 停止挖矿 |
| `oaw mine status` | 查看挖矿状态 |
| `oaw sync` | 同步 OpenClaw 工作量 |
| `oaw pole connect` | 测试 PoLE 节点连接 |
| `oaw pole balance` | 查询 PoLE 链上余额 |
| `oaw pole sync` | 同步到 PoLE 链 |
| `oaw pole wallet` | 查看 PoLE 钱包 |
| `oaw backup` | 备份数据到 `./data-backup-YYYYMMDD-HHMMSS` |
| `oaw export [json/csv]` | 导出工作记录 |
| `oaw dashboard [port]` | 启动 Web Dashboard (默认端口: 8080) |

## 架构

```
┌─────────────────────────────┐
│   OpenClaw Agent          │
└────────────┬──────────────┘
             │
             ▼
┌─────────────────────────────┐
│   OAW Tracker              │
│   • 工作量记录            │
│   • 价值计算              │
└────────────┬──────────────┘
             │
             ▼
┌─────────────────────────────┐
│   Local Mining            │
│   • PoW 挖矿             │
│   • 动态难度调整          │
│   • 区块记录             │
└────────────┬──────────────┘
             │
             ▼
┌─────────────────────────────┐
│   PoLE Chain              │
│   • 链上验证             │
│   • 代币合约             │
│   • 奖励分发             │
└─────────────────────────────┘
```

## 挖矿机制

### 工作量证明 (PoW)

- **算法**: SHA256 哈希
- **难度**: 动态调整 (2-10)
- **目标**: 前 N 位为 0 (N = 当前难度)
- **奖励**: 每个区块 10 OAW

### 动态难度

系统会根据区块生成时间自动调整难度：

- 生成太快 → 增加难度
- 生成太慢 → 降低难度
- 难度范围: 2-10

## 价值公式

```
AVS = 输出Token × 0.1 - 输入Token × 0.001
```

### 任务类型加成

| 类型 | 权重 | 说明 |
|------|------|------|
| debug | 2.0 | 调试修复 (最高价值) |
| deploy | 1.8 | 部署运维 |
| coding | 1.5 | 代码生成 |
| analysis | 1.4 | 数据分析 |
| research | 1.3 | 调研分析 |
| review | 1.2 | 代码审查 |
| writing | 1.0 | 文字创作 |
| doc | 0.8 | 文档编写 |

## 安全特性

### 钱包加密 (AES-256-GCM)

私钥使用 PBKDF2 密钥派生 + AES-256-GCM 加密存储：

```go
// 加密配置
Iterations = 100,000  // PBKDF2 迭代次数
KeySize = 32          // AES-256
SaltSize = 32          // 随机盐值
NonceSize = 12         // GCM nonce
```

## 数据存储

```
data/
├── wallets/        # 钱包文件
│   └── default.json
├── records/       # 工作量记录 (JSON)
│   └── 1772084819501616500.json
├── proofs/        # 工作证明
├── blocks.json    # 区块链数据
└── export.*       # 导出的数据
```

## PoLE 链集成

### REST API 端点

| 端点 | 说明 |
|------|------|
| `/status` | 链状态 |
| `/block/latest` | 最新区块 |
| `/account/balance?address=xxx` | 余额查询 |
| `/tx/broadcast` | 广播交易 |

### 响应格式

```json
{
  "success": true,
  "data": {
    "chain_id": "pole-mainnet-1",
    "block_height": 0,
    "total_accounts": 5
  }
}
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.24 |
| 以太坊库 | go-ethereum |
| CLI框架 | spf13/cobra |
| 加密 | AES-256-GCM, PBKDF2 |

## 许可证

MIT
