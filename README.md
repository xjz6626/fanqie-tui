# fanqie-tui

一个 Go 编写的对话式番茄小说终端阅读器。启动后直接进入全屏 TUI，你可以像和本地 agent 对话一样搜索书籍、选择结果、打开目录并连续阅读。

界面借鉴现代终端 agent 的信息层次：上方是消息流，中间会显示网络工具状态，底部是持续输入框和当前书籍进度。它不是 AI 服务，不调用 OpenAI API；自然语言命令由本地状态机解析。

配色会根据终端背景自动切换浅色或深色方案，并在真彩色、256 色和基础 ANSI 终端中自动降级。

章节正文会自动进行中文段落缩进和标点避头尾换行，并正确计算中英文、宽字符与 emoji 的终端显示宽度。阅读时可用左右方向键切章，按 `F2` 在标准、加粗、宽松三种正文显示模式间切换。

阅读历史和显示设置保存在本机，默认位置为 `$XDG_STATE_HOME/fanqie-tui/library.json`，未设置 XDG 时为 `~/.local/state/fanqie-tui/library.json`。文件使用 `0600` 权限原子更新。未登录时收藏夹也保存在这里；登录后收藏和收藏夹直接使用官网书架。

> 本项目不是番茄小说官方客户端。默认只读取公开内容；登录后可同步官网书架与阅读进度。付费或锁定章节不会尝试绕过限制；上游网页或接口变化时，部分功能可能暂时失效。

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
热门榜
编辑推荐
男频精选
女频精选
最近更新
出版榜
搜索 三体
更多结果
打开 1
从第 3 章开始读
下一章
上一章
查看目录
收藏
取消收藏
收藏夹
同步账号
书评
最新书评
历史记录
继续阅读
登录状态
现在读到哪
```

在尚未打开书籍时，直接输入书名也会自动搜索。斜杠命令可作为精确后备：

在输入框键入 `/` 会弹出命令补全菜单；继续输入可实时筛选，使用 `↑` / `↓` 选择，按 `Tab` 或 `Enter` 补全，需要参数的命令会保留空格等待继续输入，按 `Esc` 收起。

```text
/popular
/recommend
/male
/female
/updates
/published
/categories
/category female 古风世情
/authors
/author 1
/search 三体
/more
/open 1
/read 3
/catalog
/next
/prev
/favorite
/unfavorite
/favorites
/history
/resume
/account
/login
/cloud-history
/read-items
/bookshelf
/sync
/reviews
/review-feed
/comment bookID-commentID  # 也可直接粘贴完整官网书评链接
/logout
/font relaxed
/status
/clear
/quit
```

快捷键：

- `Enter`：发送消息
- `Ctrl+J`：输入换行
- `F1` / `Ctrl+K`：打开或关闭命令面板
- `←` / `→`：输入框为空且正在阅读时，切换上一章 / 下一章
- `F2`：循环切换标准、加粗、宽松正文显示模式
- `PageUp` / `PageDown` 或鼠标滚轮：滚动消息流
- `Esc`：取消正在进行的网络请求
- `Ctrl+C`：取消请求，再按一次退出；空闲时直接退出

网络较慢时可以调整超时：

```bash
fanqie -timeout 45s
```

自定义本地资料库位置：

```bash
fanqie -state-file /私密目录/library.json
```

## 可选登录会话

程序不会接收账号密码，也不会把 Cookie 写入阅读历史或收藏文件。它支持浏览器导出的 Netscape `cookies.txt`，以及从开发者工具复制的单行 `Cookie` 请求头。

如果当前目录已经有 `fanqienovel.com_cookies.txt`（Firefox/扩展常见的导出格式），首次登录直接运行：

```bash
chmod 600 ./fanqienovel.com_cookies.txt
fanqie -cookie-file ./fanqienovel.com_cookies.txt
```

也可以先直接运行 `fanqie`；启动提示检测到该文件后，在界面输入“登录”，会立即完成同样的验证和保存流程。

自动发现也兼容 `cookies.txt`、`cookie.txt`、`cooike.txt` 和无扩展名的 `cooike`；这些名称都已加入项目的 Git 忽略规则。

程序会先调用官网账号接口验证；只有验证成功才会原子保存到 `$XDG_CONFIG_HOME/fanqie-tui/session.cookie`，未设置 XDG 时保存到 `~/.config/fanqie-tui/session.cookie`。配置目录权限为 `0700`，文件为 `0600`。以后直接运行 `fanqie` 即可自动登录。

登录后可用：

```text
登录状态          # 或 /account
同步账号          # 或 /sync，一次刷新账号、官网书架和云进度
云端历史          # 或 /cloud-history
已读章节          # 打开一本书后使用，或 /read-items
官网书架          # 或 /bookshelf
收藏              # 把当前书加入官网书架
取消收藏          # 把当前书移出官网书架
书评              # 当前书在官网公开索引中的书评
最新书评          # 随后可输入“书评 1”查看全文与回复
退出登录          # 或 /logout，同时清除内存和默认配置
```

官网书架使用官网当前的 `v:version` 信息、检查、加入和删除路由，并通过批量书籍信息接口补全书名。登录后成功打开章节会同步官网阅读进度；“收藏”和“取消收藏”会直接同步官网书架，番茄网页与 App 会看到相同状态。

书籍详情会组合官网详情和搜索数据，显示聚合评分、阅读量、加书架数、字数和章节数。书评详情来自官网公开评论接口，包含主评、追评、评分、阅读时长、点赞和回复；由于官网 PC 端没有提供完整的按书分页列表，`/reviews` 使用官网公开书评索引，可能看不到较旧书籍的全部评价，`/review-feed` 可浏览最新公开书评。

公开功能还包括 `/categories`、`/category`、`/authors` 和 `/author`。Cookie 只会发送给标准 HTTPS `fanqienovel.com` 上明确列入允许清单的账号接口，公开搜索、书评和章节请求不携带 Cookie，跨域重定向会剥离。Cookie 等同登录凭据，请勿提交到 Git、粘贴到聊天、截图或共享；过期后重新导入即可。

终端应用无法可靠控制终端模拟器使用的字体家族或字号，因此应用内提供的是正文显示模式：`F2`，或 `/font standard`、`/font bold`、`/font relaxed`。字体家族与字号仍请在终端设置中选择。

## 开发

```bash
make test
make vet
make build
```

项目按职责拆分：`internal/fanqie` 负责网页、公开接口和可选会话接口，`internal/agent` 管理自然语言意图与阅读上下文，`internal/library` 持久化本地资料，`internal/session` 安全加载浏览器会话，`internal/tui` 负责消息流、输入框和状态栏。

发现页使用番茄小说无需登录的公开榜单接口，包括热门榜、每周推荐、男频精选、女频精选、最近更新和出版榜。可选登录接入官网账号、云端进度、单书已读、官网书架与进度写入接口。登录状态下翻章会同步官网进度；与正文阅读一样，上游接口变化时可能需要同步更新解析逻辑。
