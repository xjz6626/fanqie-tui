# fanqie-tui

一个 Go 编写的对话式番茄小说终端阅读器。启动后直接进入全屏 TUI，你可以像和本地 agent 对话一样搜索书籍、选择结果、打开目录并连续阅读。

界面借鉴现代终端 agent 的信息层次：上方是消息流，中间会显示网络工具状态，底部是持续输入框和当前书籍进度。它不是 AI 服务，不调用 OpenAI API；自然语言命令由本地状态机解析。

> 本项目不是番茄小说官方客户端，只读取无需登录即可访问的公开内容。付费或锁定章节不会尝试绕过限制；上游网页结构变化时，部分功能可能暂时失效。

## 安装与更新

需要 Go 1.24.2 或更高版本：

```bash
git clone https://github.com/xjz6626/fanqie-tui.git
cd fanqie-tui
./install.sh
```

脚本会构建静态单文件二进制并原子安装到 `~/.local/bin/fanqie`。Fish 用户会自动配置本地 PATH；其他 Shell 如尚未包含该目录，脚本会给出提示。

以后在仓库目录一条命令完成拉取和安装：

```bash
./install.sh --update
```

如果工作区存在未提交修改，`--update` 会停止，不会覆盖本地内容。安装到其他目录：

```bash
./install.sh --dir /自定义/目录
```

仅构建而不安装：

```bash
make build
./build/fanqie
```

也可以直接安装到 Go 的二进制目录：

```bash
go install github.com/xjz6626/fanqie-tui/cmd/fanqie@latest
fanqie
```

## 对话式使用

直接启动：

```bash
fanqie
```

或者附带首条指令：

```bash
fanqie "搜索 三体"
```

进入界面后可以输入：

```text
搜索 三体
打开 1
从第 3 章开始读
下一章
上一章
查看目录
现在读到哪
```

在尚未打开书籍时，直接输入书名也会自动搜索。斜杠命令可作为精确后备：

```text
/search 三体
/open 1
/read 3
/catalog
/next
/prev
/status
/clear
/quit
```

快捷键：

- `Enter`：发送消息
- `Ctrl+J`：输入换行
- `PageUp` / `PageDown` 或鼠标滚轮：滚动消息流
- `Esc`：取消正在进行的网络请求
- `Ctrl+C`：取消请求，再按一次退出；空闲时直接退出

网络较慢时可以调整超时：

```bash
fanqie -timeout 45s
```

## 开发

```bash
make test
make vet
make build
```

项目分为三层：`internal/fanqie` 负责公开网页解析和字体解码，`internal/agent` 管理自然语言意图与阅读上下文，`internal/tui` 负责消息流、输入框和状态栏。
