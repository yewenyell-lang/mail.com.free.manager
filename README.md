# mail.com 工作台

面向 mail.com 免费邮箱的批量管理工作台：导入账号、登录、收信、验证码提取、单封写信/回复/转发。

服务端**零持久化**。账号、token、邮件列表和正文只存在浏览器 IndexedDB。刷新页面不会丢账号；换浏览器或清站点数据需要重新导入。

## 功能

- 导入 `email:password` 或 `email----password`（密码含 `:` 时必须用 `----`）
- 账号勾选后批量登录 / 收信 / 导出 / 删除；右键复制邮箱、复制账号密码、重新登录
- 已登录账号默认跳过完整登录，行内刷新按钮只同步邮件
- 收件箱 / 已发送 / 回收站 / 垃圾箱切换；列表标收发方向
- 打开收件自动已读；勾选后才出现已读/星标/回收站等操作
- 验证码从主题/正文提取，一键复制
- 富文本写信（字体、颜色、表格、插图、附件）；回复/转发引用原信
- 正文内嵌图作为内嵌附件发送；详情页可下载附件并显示图片
- 列表与正文走本机缓存，点「刷新」才向 mail.com 同步

## 技术栈

| 层 | 选型 |
| --- | --- |
| 前端 | Vite 7 + React 19 + Tailwind 4 + Dexie + TinyMCE 8 |
| 后端 | Go 1.26 + Gin，协议按 [tanu360/maildotcom-sdk](https://github.com/tanu360/maildotcom-sdk) 用 tls-client 重写 |
| 运行时 | 推荐 [vfox](https://github.com/version-fox/vfox)，见 `.tool-versions`（Node 24 / Go 1.26） |

## 本地运行

开发环境若无法直连 mail.com，把代理写进环境变量（示例见 `.env.example`）：

```powershell
$env:MAILCOM_PROXY = 'http://127.0.0.1:7897'
$env:PORT = '8787'
$env:WEB_DIR = ''
```

后端：

```powershell
cd server
go test ./...
go run ./cmd/api
```

前端（另开终端，`/api` 会代理到 `8787`）：

```powershell
cd web
npm install
npm run dev
```

浏览器打开 Vite 提示的本地地址。生产构建：

```powershell
cd web
npm run build
$env:WEB_DIR = (Resolve-Path .\dist).Path
cd ..\server
go run ./cmd/api
```

然后访问 `http://127.0.0.1:8787`。

## Docker

生产机（能直连 mail.com）**不要**设置 `MAILCOM_PROXY`。

```powershell
docker compose up --build -d
```

镜像多阶段构建：前端 `npm run build`，后端 `go build`，最终镜像只含二进制和 `web/dist`，监听 `8787`。

交叉编译 Linux amd64（部署到 Debian/Ubuntu 时）：

```powershell
cd server
$env:GOOS = 'linux'
$env:GOARCH = 'amd64'
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags '-s -w' -o bin/api-linux-amd64 ./cmd/api
```

反向代理注意：`client_max_body_size` 至少 64m（附件走 JSON base64）。TLS 由你自己的 nginx/Caddy + Let's Encrypt 处理。

## 安全

- 不要把真实账号文件、`.env`、SSH 配置提交进仓库
- 服务端不存密码；密码只在登录请求里短暂使用
- 导出功能会把 `email----password` 写到剪贴板，注意使用环境
- mail.com 对并发敏感：批量登录/收信默认同时 3 个账号，单账号拉文件夹同时 5 个，没有额外限速

## 许可

仅供个人管理自己拥有的 mail.com 账号使用。请遵守 mail.com 服务条款。
