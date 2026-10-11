# Magpie Intelligence Check

Magpie 插件，用于对指定的主力模型做质量探针，并在探针未通过或结果过期时拦截后续请求，等待自动检测或人工处理。

项目由两部分组成：

- **Magpie 插件**：`plugin/index.mjs` 负责启动后台、注册控制台入口和读取 Magpie 配置；`plugin/gate.middleware.js` 是 Magpie 请求门禁。
- **Go 后台**：`cmd/quality-check` 提供本机管理 API、SQLite 状态存储、日志监听、模型探针和临时远程控制台。管理页面嵌入在 Go 二进制中。

插件只拦截被明确加入监测的**完整模型 ID**。它不会替换 Magpie 的供应商配置、API 地址或 Key，也不会自动切换模型。

## 工作流程

```text
客户端 -> Magpie -> quality-check 门禁 -> 上游模型
                         |
                         +-- 未启用监测 ----------------> 直接通过
                         +-- 检测通过且未过期 ----------> 直接通过
                         +-- 未检测或检测已过期 --------> 返回 403
                                                          |
                                                          +-- 日志触发后台探针
                                                          +-- 控制台手动检测
```

首次使用或检测结果超过 15 分钟后，门禁会返回 `403`，原业务请求不会发往上游。后台完成检测后，客户端需要重新发送原请求；是否自动重试由客户端决定。

同一模型的并发触发会合并为一轮检测。检测通过结果从完成时间起有效 15 分钟；闲置期间不会主动补跑检测。

## 安装

### 使用发行包

发行包在 `dist/` 中，按平台提供完整插件目录和已编译的 Go 二进制：

| 平台 | 文件 |
| --- | --- |
| Linux x86_64 | `magpie-intelligence-check-linux-amd64.tgz` |
| Linux ARM64 | `magpie-intelligence-check-linux-arm64.tgz` |
| macOS Intel | `magpie-intelligence-check-darwin-amd64.tgz` |
| macOS Apple Silicon | `magpie-intelligence-check-darwin-arm64.tgz` |
| Windows x86_64 | `magpie-intelligence-check-windows-amd64.tgz` |

解压后，把解压出的**完整目录**添加到 Magpie：

```sh
tar -xzf magpie-intelligence-check-darwin-arm64.tgz
magpie plugin add /absolute/path/to/magpie-intelligence-check-darwin-arm64
```

安装目录需要保持固定并可写。插件会在其中读取 JavaScript 门禁文件，并由 Go 后台原子更新该文件。

发行包已经包含对应平台的 Go 二进制，运行机器不需要安装 Go 或 Node.js。

### 从源码构建

需要 Go `1.26` 工具链：

```sh
go build -o bin/quality-check ./cmd/quality-check
magpie plugin add /absolute/path/to/magpie-intelligence-check
```

开发目录需要包含 `plugin/`、`bin/quality-check`、`package.json` 和 `third_party/`。Magpie 主进程（桌面应用、`magpie web` 或 `magpie serve`）必须保持运行；插件后台跟随宿主进程生命周期。

## 首次配置

1. 在 Magpie 插件入口打开“本机质量检测控制台”。
2. 在“Magpie 连接与预算”中确认网关 URL、Key 和日志路径。
3. 点击“刷新模型”，选择完整模型 ID，例如 `provider/model`，添加监测目标。
4. 先运行一次“牛奶推理”确认网关和探针可用。
5. 从客户端发送一次该模型的业务请求，确认门禁、日志触发和检测流程正常。

插件会优先通过以下方式发现本机网关：

- `MAGPIE_ADDR` 环境变量；
- `$XDG_CONFIG_HOME/magpie/settings.json` 中的监听端口；
- 默认地址 `http://127.0.0.1:3425`。

发现只允许回环地址，并用 Key `magpie` 请求 `/v1/models` 验证连接。远程地址或其他 Key 需要在控制台手动填写。已有配置不会被自动覆盖。

只需要填写 Magpie 网关的 URL 和 Key，不需要重复填写各上游供应商凭据。

### 日志源

自动触发依赖 Magpie 日志。门禁在拦截请求时写入一条不包含对话内容和网关 Key 的标记，Go 后台从日志中读取新标记并启动牛奶题探针。

例如：

```sh
mkdir -p "$HOME/.local/state/magpie"
chmod 700 "$HOME/.local/state/magpie"
umask 077
magpie serve 2>> "$HOME/.local/state/magpie/gateway.log"
```

然后把这个**绝对路径**填入控制台。已有 systemd 或其他服务可以直接把标准错误重定向到可读文件，不需要启动第二个 Magpie 实例。

后台每秒检查日志，并支持文件截断、重命名和轮转。首次打开日志时从文件末尾开始，不会补跑历史内容。

没有可读日志时，手动检测仍然可用，但自动触发不可用；门禁仍会阻止未验证目标。日志文件可能包含其他 Magpie 敏感信息，请限制其权限。

## 远程控制台

选择“打开临时远程控制台（Cloudflare）”后，插件会启动 Cloudflare Quick Tunnel，把本机管理页面临时暴露为 HTTPS 地址：

- 一次性 `/open/...` 链接有效 10 分钟；
- 浏览器会话最长 30 分钟；
- 临时通道最长 30 分钟，也可以在页面中提前结束；
- 关闭通道会撤销远程会话，但不会解除模型暂停，也不会停止检测后台；
- 只开放管理页面，不开放 Magpie 模型 API，网关 Key 不返回浏览器。

插件优先使用 PATH 中已有的官方 `cloudflared`。找不到时，默认下载并校验固定版本 `2026.10.0`，缓存到数据目录的 `runtime/`，不需要 root，也不会修改系统 PATH。自动下载需要访问 GitHub 发行文件和 Cloudflare 隧道服务；可通过 `cloudflaredPath` 指定已有客户端，或关闭自动下载。

不要只打开临时域名首页，必须使用插件返回的完整 `/open/...` 链接。不要把管理端口直接绑定到公网。

## 检测规则

内置 7 道题：

| ID | 题目 | 是否参与自动门禁 |
| --- | --- | --- |
| `milk` | 牛奶推理 | 是 |
| `repeat` | 逐字复述 | 否，人工评价 |
| `japan` | 日本首相 | 否，人工评价 |
| `person` | 人物知识 | 否，人工评价 |
| `html` | HTML/SVG 动画 | 否，人工评价 |
| `svg` | SVG 创作 | 否，人工评价 |
| `map` | 世界海陆地图 | 否，人工评价 |

提示词正文按源码原样发送，保留字符、标点、空格和空行。题目文本有 SHA-256 回归测试；代码围栏、语言标记和评价说明不会追加到发送正文。

自动门禁只使用 `milk`：最终答案为 `46.24`，或最后一个带 `inches`/`inch`/`英寸` 单位的数字为 `46.24` 时通过。首次答案异常会独立复测一次；两次都未通过则进入“需要人工确认”。

其他题目会保存原始回答和 token 用量，供控制台人工查看或下载。后台不会执行模型生成的 HTML、SVG 或 Python。

### 预算与超时

- 默认每个模型每天最多 120 次探针请求，可在控制台设置为 1–10000；
- 每轮自动检测最多调用 2 次；
- 单次请求 `max_tokens` 为 8192；
- 单轮总超时为 180 秒；
- 网关调用失败、超时、空回答、截断、无效响应或预算耗尽会标记为“检测不可用”；
- 检测不可用后自动冷却 15 分钟，手动检测可以立即发起。

探针请求使用 Magpie 的 Chat Completions 接口，并把模型改写为内部别名 `quality-probe/<随机别名>/<完整模型 ID>`。门禁会放行这个内部别名，避免探针再次触发门禁。

这些题目只是行为与能力线索，不是模型身份认证、供应商认证或通用智商排名。单题失败不能证明模型被替换；知识题有时效性，复述指纹也可能随版本变化。

## 状态与人工操作

控制台中的目标状态包括：

- **未检测**：目标已启用，但没有有效检测结果；
- **检测中**：后台正在运行自动检测；
- **通过**：结果有效期内允许请求；
- **需要人工确认**：自动题复测仍异常，继续拦截；
- **检测不可用**：网关、超时或预算等原因导致本轮无法完成；
- **监测关闭**：目标不再受门禁控制。

可执行操作：

| 操作 | 作用 |
| --- | --- |
| 重新检测 | 立即运行所选题目，计入预算 |
| 放行 15 分钟 | 暂时允许该目标请求，保留异常记录 |
| 暂停 | 设置为人工确认状态并持续拦截 |
| 关闭监测 | 关闭该目标的检测和门禁 |

自动门禁返回的是实际 API `403`，消息包含目标、原因和事件编号，例如：

```text
[MAGPIE_QUALITY_REVIEW_REQUIRED] 需要人工确认。
本次请求未发往上游。
```

插件只保证匹配目标的模型请求被拦截，不控制客户端已经开始的本地工具操作或整个任务流程。

## 插件选项

可以在 Magpie 插件配置中传入：

```json
{
  "dataDir": "/srv/magpie-quality/data",
  "gatewayURL": "http://127.0.0.1:3425",
  "gatewayKey": "magpie",
  "logPath": "/srv/magpie/gateway.log",
  "binaryPath": "/srv/magpie-quality/bin/quality-check",
  "cloudflaredPath": "/usr/local/bin/cloudflared",
  "autoInstallCloudflared": true
}
```

字段说明：

- `dataDir`：SQLite、门禁状态和 Cloudflare 缓存目录。默认 `~/.config/magpie-quality-check`；
- `gatewayURL`、`gatewayKey`、`logPath`：用于首次自动连接配置，不覆盖控制台已有设置；
- `binaryPath`：指定 Go 后台二进制；
- `cloudflaredPath`：指定官方 `cloudflared`；
- `autoInstallCloudflared`：设为 `false` 时禁止自动下载 Cloudflare 客户端。

数据目录会以私有权限创建，SQLite 中保存网关凭据、历史结果和目标状态。停用、退出或重启不会清除数据；卸载插件也不会自动删除数据目录，请按需手动备份或清理。

同一个 `dataDir` 只允许一个检测后台运行。多个 Magpie 实例必须使用独立的插件目录和 `dataDir`。

## 限制与兼容性

- 依赖 Magpie 的插件系统、中间件热加载和可读日志；
- 门禁按最终请求中的完整模型 ID 判断，自动路由别名不应当当作目标 ID；
- 由于门禁文件热加载存在传播窗口，Magpie 中间件加载失败时默认放行；这不是强安全隔离；
- 网关 Key 需要有权限调用 `/v1/models` 和 `/v1/chat/completions`，并允许探针模型别名；
- 当前探针使用非流式 Chat Completions，由 Magpie 负责上游协议转换；
- 发行脚本生成 Linux、macOS 和 Windows 的交叉编译包，实际部署仍需检查网络、文件权限、日志路径和 Magpie 版本；
- Cloudflare Quick Tunnel 是临时通道，不保证公网可用性或稳定性。

## 开发与验收

运行 Go 测试和插件测试：

```sh
go test -race ./...
go vet ./...
node --test plugin/*.test.mjs
```

构建发行包和校验清单：

```sh
node scripts/build-release.mjs
node scripts/package-check.mjs
```

可选的集成检查：

```sh
MAGPIE_BINARY=/absolute/path/to/magpie node scripts/magpie-integration.mjs
MAGPIE_BINARY=/absolute/path/to/magpie node scripts/install-check.mjs
node scripts/plugin-check.mjs
node scripts/client-check.mjs
node scripts/cloudflare-check.mjs
node scripts/browser-check.mjs
```

其中：

- `magpie-integration.mjs` 验证真实 Magpie 网关中的门禁、探针别名和日志触发；
- `install-check.mjs` 验证发行包解压、插件注册和宿主退出清理；
- `client-check.mjs` 验证客户端能看到 `MAGPIE_QUALITY_REVIEW_REQUIRED` API 错误；
- `cloudflare-check.mjs` 验证临时 HTTPS 入口、会话交换和关闭；
- `browser-check.mjs` 需要 Playwright，可通过 `PLAYWRIGHT_MODULE` 指定模块位置。

## 目录结构

```text
cmd/quality-check/       Go 后台入口
internal/check/          状态、探针、HTTP 控制台和日志监听
internal/check/web/      嵌入 Go 二进制的管理页面
plugin/index.mjs         Magpie 插件入口
plugin/gate.middleware.js 请求门禁（运行时由后台原子更新）
plugin/cloudflared-runtime.mjs  Cloudflare 客户端发现、下载和校验
scripts/                 构建、集成和客户端验收脚本
third_party/cloudflared/ Cloudflare 客户端许可文件
```

## 许可

本项目采用 MIT License。Cloudflare 客户端相关许可随包保留在 `third_party/cloudflared/LICENSE`。
