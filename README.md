<p align="center">
  <a href="https://github.com/w-run/one-api"><img src="https://raw.githubusercontent.com/w-run/one-api/main/web/default/public/logo.png" width="150" height="150" alt="one-api logo"></a>
</p>

<div align="center">

# One API · w-run 二次开发版

_✨ 基于标准 OpenAI API 格式访问所有大模型的开源 AI 网关 ✨_

</div>

<p align="center">
  <a href="https://raw.githubusercontent.com/w-run/one-api/main/LICENSE">
    <img src="https://img.shields.io/github/license/w-run/one-api?color=brightgreen" alt="license">
  </a>
  <a href="https://github.com/w-run/one-api/releases/latest">
    <img src="https://img.shields.io/github/v/release/w-run/one-api?color=brightgreen&include_prereleases" alt="release">
  </a>
  <a href="https://hub.docker.com/repository/docker/wrundev/one-api">
    <img src="https://img.shields.io/docker/pulls/wrundev/one-api?color=brightgreen" alt="docker pull">
  </a>
</p>

> 本仓库为 **w-run 二次开发版本**，基于上游 [songquanpeng/one-api](https://github.com/songquanpeng/one-api) 进行定制开发。
> 完整功能介绍与使用文档请参考 [原项目 README](https://github.com/songquanpeng/one-api/blob/main/README.md)。

---

## 软件信息

| 项目 | 内容 |
|---|---|
| 当前版本 | `1.1.0` |
| 上游版本 | 基于 [songquanpeng/one-api](https://github.com/songquanpeng/one-api) |
| 许可证 | MIT（保留原作者署名） |
| 镜像仓库 | `wrundev/one-api`（Docker Hub）/ `ghcr.io/w-run/one-api`（GHCR） |
| Go 模块 | `github.com/w-run/one-api` |
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
- Go 模块路径迁移至 `github.com/w-run/one-api`
- Docker 镜像仓库迁移至 `wrundev/one-api`

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

### 4. 基础设施
- GitHub CI 配置 Docker Hub 自动构建
- 仓库脱离 fork 状态
- Go 模块路径迁移至 `github.com/w-run/one-api`
- Docker 镜像仓库迁移至 `wrundev/one-api`

---

## 开发说明

### 环境要求
- Go ≥ 1.21
- Node.js ≥ 16
- npm ≥ 8

### 本地开发

```bash
# 1. 克隆仓库
git clone https://github.com/w-run/one-api.git
cd one-api

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
  -ldflags "-s -w -X 'github.com/w-run/one-api/common.Version=$(cat VERSION)' \
           -linkmode external -extldflags '-static'" \
  -o one-api
```

### Docker 镜像

```bash
# 拉取
docker pull wrundev/one-api

# 运行（SQLite）
docker run --name one-api -d --restart always \
  -p 3000:3000 \
  -e TZ=Asia/Shanghai \
  -v /home/ubuntu/data/one-api:/data \
  wrundev/one-api

# 运行（MySQL）
docker run --name one-api -d --restart always \
  -p 3000:3000 \
  -e SQL_DSN="root:123456@tcp(localhost:3306)/oneapi" \
  -e TZ=Asia/Shanghai \
  -v /home/ubuntu/data/one-api:/data \
  wrundev/one-api
```

初始账号：`root` / `123456`（登录后请立即修改）。

---

## 项目结构

```
one-api/
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
- 镜像地址：`wrundev/one-api` + `ghcr.io/w-run/one-api`
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
