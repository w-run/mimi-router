<div align="center">

# mimi-router

_✨ 基于标准 OpenAI API 格式访问所有大模型的开源 AI 网关 ✨_

</div>

<p align="center">
  <a href="https://raw.githubusercontent.com/w-run/mimi-router/main/LICENSE">
    <img src="https://img.shields.io/github/license/w-run/mimi-router?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/w-run/mimi-router/releases/latest">
    <img src="https://img.shields.io/github/v/release/w-run/mimi-router?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://hub.docker.com/repository/docker/wrundev/mimi-router">
    <img src="https://img.shields.io/docker/pulls/wrundev/mimi-router?color=brightgreen" alt="docker pull">
  </a>
</p>

> 本仓库为 **w-run 二次开发版本**，基于上游 [songquanpeng/one-api](https://github.com/songquanpeng/one-api) 进行定制开发。
> 完整功能介绍与使用文档请参考 [原项目 README](https://github.com/songquanpeng/one-api/blob/main/README.md)。

---

## 软件信息

| 项目 | 内容 |
|---|---|
| 当前版本 | `1.4.0` |
| 上游版本 | 基于 [songquanpeng/one-api](https://github.com/songquanpeng/one-api) |
| 许可证 | MIT（保留原作者署名） |
| 镜像仓库 | `wrundev/mimi-router`（Docker Hub）/ `ghcr.io/w-run/mimi-router`（GHCR） |
| Go 模块 | `github.com/w-run/mimi-router` |
| 前端目录 | `web/default`（基于 [MartialBE/berry](https://github.com/MartialBE) 主题） |

---

## 相比上游的主要变更

### 1. UI 主题
- 移除 `berry` / `air` 主题，**统一使用 `default` 主题**（原 berry 主题重命名）
- 暗色模式修饰色统一为蓝色（`rgb(33, 150, 243)`），移除原紫色
- 移除 **检测更新** 板块
- 移除 **用户头像** 组件及所有 `user-round.svg` 引用
- 优化顶栏 Chip 按钮、菜单按钮、用户名显示等细节样式

### 2. 渠道管理
- **获取可用模型**支持从数据库回退读取密钥（前端未填写时自动 fallback）
- 密钥回退逻辑包含详细日志输出，便于排查
- 新增 **自动生成模型映射关系** 功能：
  - 规则：全小写 + 短横线格式，去除厂商前缀（`/` 前部分）
  - 重复时增加 `-厂商` 后缀
  - 示例：`deepseek-ai/DeepSeek-V4-Pro` → `deepseek-v4-pro`
  - 映射关系 JSON：`{"deepseek-v4-pro": "deepseek-ai/DeepSeek-V4-Pro"}`

### 3. 版本管理
- 重新起版本号 `1.1.0`，前后端版本号统一（`v1.1.0`）
- 修复版本更新提示逻辑

### 4. 基础设施
- GitHub CI 配置 Docker Hub 自动构建
- 仓库脱离 fork 状态
- Go 模块路径迁移至 `github.com/w-run/mimi-router`
- Docker 镜像仓库迁移至 `wrundev/mimi-router`

### 5. 模型名称格式化
- 重构渠道编辑中的「自动生成模型映射」算法，支持：
  - 路由前缀（`Pro/`、`LoRA/` 等）后置
  - 整词保留（`DeepSeek`、`Qwen`、`GLM`、`Kimi`、`Hunyuan`、`Wan`、`Ling`、`ERNIE`、`MiniMax`、`FunAudioLLM`、`PaddlePaddle`、`BAAI` 等）
  - 全大写缩写（`OCR`、`ASR`、`VL`、`LLM`）在驼峰边界拆 `-`
  - 特殊命名硬编码（`Pro/moonshotai/Kimi-K2.6` → `kimi-k2.6`、`BAAI/bge-m3` → `bge-m3-baai` 等）

### 6. 渠道管理增强
- 渠道表格支持按字段排序，**默认 ID 倒序**
- 排序状态切换：同列再点切换升降序，切换列重置为升序（ID 列除外）

### 7. 顶栏 / 侧边栏调整
- 主题按钮旁的「设置」改名为「**账号设置**」
- 移除「关于」页面及相关路由、导航入口

### 8. 首页重构
- 首页极简风重写，**M3 Material You 风格**：
  - 整页垂直水平居中
  - 大字标题（响应式 xs 3rem / sm 4.5rem / md 6rem）
  - 副标题弱化（`text.secondary`）
  - Logo + 标题 + 文案 + GitHub 按钮垂直排列
  - GitHub 按钮采用胶囊形（`borderRadius: 999`）

### 9. 渠道回退（fallback）机制
> v1.2.0 引入。解决「同模型多渠道」场景下，调用出错时直接重试同一渠道导致网络超时/重复扣费的问题。

**核心能力**
- **触发器驱动的渠道回退**：调用同名模型遇错时按渠道 `priority`（升序）回退，支持三种错误触发器：
  - `429` — 上游 `429 Too Many Requests`
  - `5xx` — 上游 500~599 服务端错误
  - `timeout` — 网络超时 / 连接失败 / `context.DeadlineExceeded`
- **管理员可控**：`fallback_enabled` 字段允许管理员把某渠道从回退队列移除，仅作主选；`fallback_triggers` 限定该渠道参与哪些类型的回退。
- **429 软禁用（soft-ban）**：当上游返回 429 且带 `Retry-After` 头时，自动将该渠道在内存中临时屏蔽至 `Retry-After` 到期，期间不再被选入回退池，避免重复 429。
- **防环回退**：维护 `usedIDs` 集合记录本次请求已尝试过的渠道 ID，防止 A→B→A 死循环；尝试满 `RelayTimes` 后退出。

**使用方式（管理员）**
1. 进入「渠道管理」页面，新增/编辑渠道时勾选「参与回退」并按需填入触发器集合（逗号分隔，留空表示全匹配）：
   ```
   fallback_enabled = true
   fallback_triggers = "429,5xx"   # 仅对 429 和 5xx 触发回退
   ```
2. 多个渠道挂载同一 `models` 字段时，调用会按 `priority ASC, id ASC` 顺序挑选，失败时按触发器命中下一个可用渠道。
3. 对**只愿意作主选**的渠道（如成本高、速度快的旗舰渠道），关闭「参与回退」开关即可。

**注意事项**
- 4xx（非 429）错误、余额不足、参数错误等不会被回退，避免在错误请求上浪费下游配额。
- 软禁用基于进程内存（`sync.Map`），重启或主备切换会清空；上游 429 短期高峰可被自动恢复。
- 回退链最多尝试 `RelayTimes` 次（默认 5，可在系统设置中调整）。
- 渠道表新增了 `fallback_enabled`（TINYINT(1) / BOOLEAN）和 `fallback_triggers`（VARCHAR(64)）两列，启动时通过 GORM AutoMigrate 自动补齐，无需手动迁移。

**API 变更**
- `Channel` 模型新增两个 JSON 字段（`omitempty` 已开启，旧数据无影响）：
  - `fallback_enabled` *bool — `true` 参与回退，`false` 仅作主选，缺省 `true`。
  - `fallback_triggers` *string — 逗号分隔的触发器集合，缺省 `""` 视为全匹配。
- `GET /api/channel/`、`GET /api/channel/:id`、`POST /api/channel/`、`PUT /api/channel/` 响应/请求体均包含上述字段；前端「渠道管理」表格已新增「回退」与「触发器」列。
- 新增 Go 公共包 `relay/fallback`，关键函数：
  - `fallback.ClassifyError(statusCode, err) string` — 错误归类
  - `fallback.ShouldFallback(channel, trigger) bool` — 渠道是否参与此次回退
  - `fallback.SoftBanFromError(ctx, channelId, bizErr)` — 收到 429 时软禁用
  - `fallback.ParseRetryAfter(value) int` — 解析 HTTP `Retry-After` 头
- 新增 `model.SoftBanChannel / UnsoftBanChannel / IsChannelSoftBanned / CleanupExpiredSoftBan` 软禁用工具函数。

### 6. Anthropic Messages API 兼容（v1.4.0）
- 新增 `POST /v1/messages` 和 `POST /v1/messages/count_tokens` 端点
- 入站协议 Anthropic SDK 兼容；内部自动转 OpenAI 协议分发到任意 OpenAI 兼容渠道
- 支持文本、多模态（图）、工具调用（`tool_use` / `tool_result`）、流式 SSE（多 event 格式）
- 支持 `count_tokens` 端点（本地估算，不消耗上游额度）
- 鉴权沿用 mimi-router 颁发的 `sk-*`（`Authorization: Bearer ...`），`anthropic-version` 头被忽略
- 协议转换层位于 `relay/relaymode/anthropic/`，不修改 OpenAI 链路，二者共享 `Distribute` / `RateLimit` / `Fallback` / 计费 / 日志中间件
- 错误响应按 Anthropic `error.type` 体系（`invalid_request_error` / `authentication_error` / `rate_limit_error` / `api_error` / ...）输出

**客户端使用示例**：

```python
# Python Anthropic SDK
from anthropic import Anthropic
client = Anthropic(
    base_url="https://your-mimi-router/v1",
    api_key="sk-your-mimi-router-token",  # mimi-router 颁发的 token
)
resp = client.messages.create(
    model="claude-3-5-sonnet-20241022",
    max_tokens=1024,
    messages=[{"role":"user","content":"Hello"}],
    stream=True,
)
```

```bash
# curl 非流式
curl -X POST https://your-mimi-router/v1/messages \
  -H "Authorization: Bearer sk-your-mimi-router-token" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "max_tokens": 1024,
    "messages": [{"role":"user","content":"Hello"}]
  }'
```

---

## 开发说明

### 环境要求
- Go ≥ 1.21
- Node.js ≥ 16
- npm ≥ 8

### 本地开发

```bash
# 1. 克隆仓库
git clone https://github.com/w-run/mimi-router.git
cd mimi-router

# 2. 前端开发
cd web/default
npm install
npm start            # 启动开发服务器（http://localhost:3000）

# 3. 后端开发（新终端）
cd ../..
go mod download
go run main.go       # 启动后端（http://localhost:3000）
```

### 构建生产版本

```bash
# 1. 构建前端
cd web
sh build.sh
# 输出：web/build/default/

# 2. 构建后端
cd ..
go build -trimpath \
  -ldflags "-s -w -X 'github.com/w-run/mimi-router/common.Version=$(cat VERSION)' \
           -linkmode external -extldflags '-static'" \
  -o mimi-router
```

### Docker 镜像

```bash
# 拉取
docker pull wrundev/mimi-router

# 运行（SQLite）
docker run --name mimi-router -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v /home/ubuntu/data/mimi-router:/data \
  wrundev/mimi-router

# 运行（MySQL）
docker run --name mimi-router -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/mimirouter" \
  -e TZ=Asia/Shanghai \
  -v /home/ubuntu/data/mimi-router:/data \
  wrundev/mimi-router
```

初始账号：`root` / `123456`（登录后请立即修改）。

---

## 项目结构

```
mimi-router/
├── common/           # 公共模块（配置、数据库、工具等）
├── controller/       # HTTP 控制器
├── relay/            # 中继转发核心逻辑
│   ├── adaptor/      # 各模型厂商适配器
│   └── billing/      # 计费逻辑
├── router/           # 路由
├── web/
│   ├── default/      # 前端项目（React + MUI）
│   ├── build.sh      # 前端构建脚本
│   └── THEMES        # 主题列表（仅 default）
├── docs/             # API 文档
├── .github/workflows # CI/CD 配置
├── Dockerfile
├── go.mod
└── VERSION
```

### 主题相关
- `web/THEMES`：当前为 `default`（唯一主题）
- `web/build.sh`：构建前端并输出到 `web/build/<主题名>/`
- `web/default/`：原 `web/berry/` 重命名而来
- Go 代码通过 `//go:embed web/build/*` 嵌入前端构建产物

### CI/CD
- `.github/workflows/docker-image.yml`：tag 触发时自动构建并推送 Docker 镜像
- 镜像地址：`wrundev/mimi-router` + `ghcr.io/w-run/mimi-router`
- 所需 Secrets：
  - `DOCKERHUB_USERNAME`：Docker Hub 用户名
  - `DOCKERHUB_TOKEN`：Docker Hub Access Token

---

## 版权与致谢

### 原项目
本项目基于 [songquanpeng/one-api](https://github.com/songquanpeng/one-api) 进行二次开发，遵循 MIT 协议。

- **原项目地址**：https://github.com/songquanpeng/one-api
- **原项目作者**：JustSong
- **许可证**：MIT License（Copyright © 2023 JustSong）

### 前端主题
- 原 Berry 主题：[MartialBE](https://github.com/MartialBE)

### 协议要求
依据 MIT 协议，使用本项目（含二次开发版本）**必须**在页面底部保留原作者署名以及指向原项目的链接。

> 本项目使用 MIT 协议进行开源，**在此基础上**，必须在页面底部保留署名以及指向本项目的链接。如果不想保留署名，必须首先获得授权。
> 同样适用于基于本项目的二开项目。
> 依据 MIT 协议，使用者需自行承担使用本项目的风险与责任，本开源项目开发者与此无关。
