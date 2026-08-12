# fanqie-tui

一个用于番茄小说公开网页的轻量终端阅读器。它可以搜索书籍、查看书籍信息与目录，并在终端中阅读未锁定章节。

> 本项目不是番茄小说官方客户端，只读取无需登录即可访问的公开内容。付费或锁定章节不会尝试绕过限制；上游网页结构变化时，部分功能可能暂时失效。

## 安装

需要 Python 3.10 或更高版本。在项目目录中执行：

```bash
python -m venv .venv
. .venv/bin/activate
python -m pip install -e .
```

也可以不安装，直接在源码目录运行：

```bash
PYTHONPATH=src python -m fanqie_tui --help
```

## 使用

先搜索书籍并记下书籍 ID：

```bash
fanqie search "书名或作者"
```

查看详情和目录：

```bash
fanqie info <书籍ID>
fanqie catalog <书籍ID>
fanqie catalog <书籍ID> --start 51 --limit 50
```

读取单章需要目录中括号内的章节 ID：

```bash
fanqie read <章节ID>
fanqie read <章节ID> --no-pager
```

交互式连续阅读：

```bash
fanqie browse <书籍ID>
fanqie browse <书籍ID> --chapter 10
```

阅读时按回车或 `n` 前往下一章，`p` 返回上一章，`g` 跳转，`q` 退出。默认使用系统分页器显示正文；如果当前终端不适合分页，可加 `--no-pager`。

所有命令都支持 `-h` 查看帮助。网络较慢时可以在子命令前调整超时：

```bash
fanqie --timeout 40 search "关键词"
```

## 开发

```bash
python -m pip install -e '.[dev]'
pytest
```

代码将上游访问封装在 `NovelProvider` 接口后，终端层不依赖具体数据来源。网页解析失败会转换为简短的用户错误，而不是输出 Python 堆栈。
