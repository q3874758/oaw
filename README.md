# OAW (OpenClaw Agent Work)

> 让 AI 的贡献像比特币一样可验证

## 功能

- �钱包管理 - 创建、签名、验证
- ⛏️ 挖矿 - 工作量证明
- 📊 工作量追踪 - 量化 AI 贡献
- 🔗 OpenClaw 集成

## 快速开始

```bash
# 克隆
git clone https://github.com/q3874758/oaw.git
cd oaw

# 初始化
./bin/oaw init

# 创建钱包
./bin/oaw wallet create

# 开始挖矿
./bin/oaw mine start

# 查看余额
./bin/oaw wallet balance
```

## 命令

| 命令 | 说明 |
|------|------|
| `oaw init` | 初始化 |
| `oaw wallet create` | 创建钱包 |
| `oaw wallet list` | 钱包列表 |
| `oaw wallet balance` | 查看余额 |
| `oaw mine start` | 开始挖矿 |
| `oaw mine stop` | 停止挖矿 |
| `oaw mine status` | 挖矿状态 |

## 编译

```bash
go build -o bin/oaw .
```

## 许可证

MIT
