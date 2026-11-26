# 维护指南

用户快速上手见 [README.md](../README.md)，领域术语见 [GLOSSARY.md](../GLOSSARY.md)，版本记录见 [CHANGELOG.md](../CHANGELOG.md)，macOS 说明见 [docs/macos-usage.md](macos-usage.md)，架构决策见 [docs/adr/](adr/)。

## 架构与目录

单二进制 Go TUI（Bubble Tea + Echo 代理适配），Windows 优先并支持 macOS 未签名测试版。

| 路径 | 职责 |
| --- | --- |
| `main.go`、`internal/system/` | 启动入口、Windows Terminal 选择与提权、系统代理与进程管理 |
| `internal/certificate/` | 根证书生成/复用、指纹信任检查、安装与旧 SunnyNet 共享证书清理 |
| `internal/app/` | 代理、目标、候选与下载器编排；下载阶段直播状态回写 |
| `internal/tui/` | 单主界面（目标选择区 / 目标条 / 分类 / 1 行扁平操作按钮栏 / 表格）、外观下拉与鼠标命中 |
| `internal/tui/ui/` | 外观模式（`downloads/ui.json`）、系统明暗、紫调扁平色板、断点与显示宽度 |
| `internal/tui/logo/` | Sixel/Kitty 能力探测与编码、Sixel 覆写清区、512×512 双主题个人紫色狸花猫圆角外框全身图标（失败降级纯文字） |
| `internal/interceptor/` | 来源过滤、业务域名统计、媒体与 HTTP-FLV 直播识别、`Content-Disposition`/H5 标题提取、播放列表缓存 |
| `internal/miniprogram/` | 目标、已发现列表（附业务域名提示）、候选存储、扩展 JSON 标题提取、六分类与资源类型目录 |
| `internal/minidownload/` | 直链/分段媒体/HTTP-FLV 直播录制、带真实标题的文件命名、按最终类型归档与晚修正 |
| `third_party/echo/` | 本地 patch 的 Echo 代理库（MITM listener 回收） |
| `build/windows/` | `build.bat`、Windows ffmpeg 获取、包校验与 `.syso` 图标资源生成（`genicon/`） |
| `build/macos/` | macOS 通用包构建、`Info.plist`、权限、`sign-macos.sh` 与 `verify-macos.py` |
| `build/ffmpeg/` | 固定源码自建 LGPLv3 ffmpeg 脚本、样片验证与 `verify-release-sources.py` |

核心边界遵循已记录的 ADR：

- **启动与权限**：Windows 以当前用户启动并写入 `HKCU` 代理，仅证书操作触发 UAC（[ADR 0004](adr/0004-windows-terminal-startup.md)、[ADR 0009](adr/0009-user-scoped-windows-runtime.md)）；macOS 通过系统授权管理活动网络服务代理并将证书写入当前用户钥匙串（[ADR 0006](adr/0006-macos-portable-distribution.md)）。
- **发现与严格归属**：启动即自动代理并持续发现（`target.appID` 仅预填输入框），只有能从 `servicewechat.com/<AppID>/<版本>/page-frame.html` 解析出匹配 AppID 的资源才是目标候选，其余媒体按 `?` 归入来源未确认（[ADR 0008](adr/0008-target-attribution.md)）。
- **UI、分类与标题命名**：固定 `1 全部 / 2 图片 / 3 视频 / 4 音频 / 5 直播 / 6 分段媒体` 六页签，HTTP-FLV 持续录制可随时按 `x` 停止或断流提交为 `.flv`（不支持 HLS 直播录制，见 [ADR 0013](adr/0013-live-stream-recording.md)）；下载目录按 `downloads/<目标 AppID>/{图片,视频,音频,直播,分段媒体,其他}` 隔离，优先使用真实资源标题命名，完整策略见 [ADR 0011](adr/0011-single-main-surface-ui.md)、[ADR 0012](adr/0012-per-appid-download-layout.md)、[ADR 0014](adr/0014-purple-flat-ui-and-readable-names.md)。

## 配置与证书

模板见 [internal/config/config.template.yaml](../internal/config/config.template.yaml)。发布包默认读取可执行文件旁 `wx-mini-video.yaml`，也可用环境变量 `WX_MINI_VIDEO_CONFIG` 指定；修改后重启生效。

| 配置 | 默认值 | 含义 |
| --- | --- | --- |
| `download.dir` | 空 | 配置目录下 `downloads/`；相对路径按当前工作目录解析 |
| `download.ffmpeg` | 空 | 显式 ffmpeg 路径，优先级最高；旧值 `ffmpeg` 视为空，不查 PATH |
| `download.mode` | `auto` | 分片 ≥20 时并发预取，否则由 ffmpeg 经本地随机入口拉取；可选 `prefetch` / `remote-ffmpeg` |
| `download.segmentConcurrency` / `segmentRetries` | `6` / `3` | 预取并发数与重试次数 |
| `download.keepSegments` | `false` | 合并成功后是否保留分片缓存 |
| `capture.showUnattributed` | `true` | 是否收集并显示来源未确认媒体 |
| `proxy.system` | `true` | 启动时设置、退出时恢复 Windows/macOS 系统代理 |
| `proxy.hostname` / `proxy.port` | `127.0.0.1` / `2023` | 本地监听地址 |
| `proxy.skipInstallRootCert` | `false` | 跳过根证书及旧共享证书检查安装；本地证书生成与导出仍执行 |
| `proxy.upstreamProxy` | 空 | 上游代理地址 |
| `target.appID` | 空 | 目标选择区输入框预填值；启动不自动确认 |
| `cert.file` / `cert.key` | 空、空 | 成对配置的 PEM 证书与私钥；优先于自动生成证书 |
| `cert.name` | `Echo` | 仅兼容保留，信任校验以指纹为准 |

- **ffmpeg 优先级**：`download.ffmpeg` > 随包 ffmpeg（Windows `ffmpeg\ffmpeg.exe`，macOS `ffmpeg/ffmpeg-amd64|arm64`）> 受管下载兜底（[ADR 0007](adr/0007-bundled-ffmpeg.md)、[ADR 0010](adr/0010-source-built-ffmpeg.md)）。
- **独立根证书**：Windows 保存在 `%LOCALAPPDATA%\wx-mini-video\certificates\root-ca.pem` 并通过 `certutil -addstore Root` 写入信任库；启动前若发现旧共享证书指纹 `D70CD039051F77C30673B8209FC15EFA650ED52C` 会提示清理（[ADR 0005](adr/0005-local-ca-lifecycle.md)）。

## 排障

| 现象 | 处理 |
| --- | --- |
| 异常退出后网络不通 | 重开程序 → 等“代理运行中”→ `Ctrl+Q` 正常退出（或点页脚退出）；仍异常时在系统代理设置移除残留地址 |
| 启动即发现但无目标 | 正常：发现不等于选定；选择列表项（可附业务域名提示）或输入 AppID 后按 `Enter`/点击确认 |
| 输入后确认了错误目标或按 `r` 没反应 | 输入框内 `Enter` 确认文本，`Tab` 切到列表再 `↑/↓` 选列表项；输入框聚焦时 `r` 为文本，重试请用 `Ctrl+R` |
| 证书安装失败 | 确认 UAC/系统授权；仍失败可手动导入 `downloads/WxMiniVideoRoot.cer`，或启动代理后访问 `http://127.0.0.1:2023/cert` |
| 没有候选或缺少音频 | 核对目标 AppID，切回 `1 全部` 或 `4 音频` 后在微信重放；`?` 项不计入目标候选 |
| 来源未确认媒体较多 | H5/web-view 与部分 CDN 缺少 page-frame Referer，列表上限 500 条，可按 `c` 清空 |
| 分类里找不到直播或 m3u8 | 直播仅在“全部/直播”，m3u8 在“分段媒体”；疑似直播确认为有限文件后会回退至“视频” |
| 想录制 HLS/m3u8 直播 | 当前不支持 HLS 持续录制；无 `#EXT-X-ENDLIST` 的列表只抓取当前快照合并为 mp4 |
| 直播一直不结束 | 正常：HTTP-FLV 无总长度；按 `x` 停止即可提交为 `.flv`，异常断流也会自动保存已录内容并提示 |
| 确认退出或下载归档失败 | 保存失败会保留 `.part`/原成品并在提示中给出路径，不写入 `history.jsonl` |
| 诊断嗅探 | 启动加 `--debug` 或设 `WX_MINI_VIDEO_DEBUG=1`，日志写 `wx-mini-video-debug.log`（query/Cookie 脱敏） |
| m3u8 预取或合并失败 | 预取失败多可调低 `download.segmentConcurrency`；合并失败查看 `.ffmpeg/<候选ID>.log` |
| 中文显示缺失 | 使用 Windows Terminal 并选择支持中文的字体 |

## 开发验证与分发

```powershell
go test -count=1 ./...
go vet ./...
go test -count=1 github.com/ltaoo/echo/...
python -B -m unittest discover -s build/macos -p "test_*.py"
python -B -m unittest discover -s build/ffmpeg -p "test_*.py"
git diff --check
```

Windows 与 macOS 各只发布一个完整包；正式自建随包 ffmpeg 通过 `WXMV_FFMPEG_DIR` 传入（未设置时构建脚本会联网下载第三方预编译包，仅作兜底）：

```powershell
$env:WXMV_FFMPEG_DIR = 'D:\path\to\windows-ffmpeg-bundle'
cmd /c build\windows\build.bat windows                # 产出 dist\wx-mini-video-windows-amd64.zip（自动清理根级临时 .syso）
$env:WXMV_FFMPEG_DIR = 'D:\path\to\macos-ffmpeg-bundle'
go run ./build/macos                                  # 产出 dist\wx-mini-video-macos-universal-unsigned.zip
python -B build/macos/verify-macos.py --package-only  # Windows 上检查 Mac ZIP 契约；Mac 上不加 --package-only 运行完整验证
Remove-Item Env:WXMV_FFMPEG_DIR
```

公开发布前在 Mac 完成签名公证（`bash build/macos/sign-macos.sh`）并运行 `python3 build/ffmpeg/verify-release-sources.py` 核对随包自建 ffmpeg 源码索引与摘要。推送、打标签与创建 Release 须有明确授权。
