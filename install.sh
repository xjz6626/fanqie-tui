#!/usr/bin/env bash
set -Eeuo pipefail

readonly SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${FANQIE_INSTALL_DIR:-${XDG_BIN_HOME:-${HOME}/.local/bin}}"
UPDATE=false
TEMP_DIR=""
INSTALL_TEMP=""

cleanup() {
  if [[ -n "${INSTALL_TEMP}" && -e "${INSTALL_TEMP}" ]]; then
    rm -f -- "${INSTALL_TEMP}"
  fi
  if [[ -n "${TEMP_DIR}" && -d "${TEMP_DIR}" ]]; then
    rm -rf -- "${TEMP_DIR}"
  fi
}
trap cleanup EXIT

usage() {
  cat <<'EOF'
安装 fanqie 到本地用户目录。

用法：
  ./install.sh [--update] [--dir 路径]

选项：
  --update    安装前执行 git pull --ff-only；工作区必须干净
  --dir 路径 自定义安装目录
  -h, --help  显示帮助

环境变量：
  FANQIE_INSTALL_DIR  自定义安装目录，默认 ~/.local/bin
  FANQIE_VERSION      覆盖编译进二进制的版本号
EOF
}

while (($# > 0)); do
  case "$1" in
    --update)
      UPDATE=true
      shift
      ;;
    --dir)
      if (($# < 2)); then
        echo "错误：--dir 需要一个路径" >&2
        exit 2
      fi
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "错误：未知选项 $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! command -v go >/dev/null 2>&1; then
  echo "错误：未找到 Go，请先安装 Go 1.24.2 或更高版本。" >&2
  exit 1
fi

if [[ "${UPDATE}" == true ]]; then
  if ! command -v git >/dev/null 2>&1 || ! git -C "${SCRIPT_DIR}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    echo "错误：--update 只能在 Git 仓库检出目录中使用。" >&2
    exit 1
  fi
  if [[ -n "$(git -C "${SCRIPT_DIR}" status --porcelain)" ]]; then
    echo "错误：工作区存在未提交改动，未执行更新。请先提交或保存改动。" >&2
    exit 1
  fi
  echo "==> 更新源码"
  git -C "${SCRIPT_DIR}" pull --ff-only
fi

VERSION="${FANQIE_VERSION:-0.2.0}"
if command -v git >/dev/null 2>&1 && git -C "${SCRIPT_DIR}" rev-parse --short HEAD >/dev/null 2>&1; then
  VERSION="${VERSION}+$(git -C "${SCRIPT_DIR}" rev-parse --short HEAD)"
fi

TEMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/fanqie-install.XXXXXX")"
echo "==> 构建 fanqie ${VERSION}"
(
  cd -- "${SCRIPT_DIR}"
  CGO_ENABLED=0 go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o "${TEMP_DIR}/fanqie" \
    ./cmd/fanqie
)

mkdir -p -- "${INSTALL_DIR}"
INSTALL_TEMP="$(mktemp "${INSTALL_DIR}/.fanqie.XXXXXX")"
install -m 0755 -- "${TEMP_DIR}/fanqie" "${INSTALL_TEMP}"
mv -f -- "${INSTALL_TEMP}" "${INSTALL_DIR}/fanqie"
INSTALL_TEMP=""

# Keep Fish installs immediately discoverable in future shells. Other shells
# receive a concise instruction instead of having their startup files edited.
if [[ ":${PATH}:" != *":${INSTALL_DIR}:"* ]]; then
  if [[ "${SHELL:-}" == */fish ]] && command -v fish >/dev/null 2>&1; then
    fish -c 'fish_add_path --universal $argv[1] >/dev/null 2>&1; or contains -- $argv[1] $fish_user_paths' "${INSTALL_DIR}"
    if fish -c 'contains -- $argv[1] $fish_user_paths' "${INSTALL_DIR}"; then
      echo "==> 已将 ${INSTALL_DIR} 加入 Fish PATH"
    else
      echo "警告：无法持久化 Fish PATH，请手动运行：fish_add_path ${INSTALL_DIR}" >&2
    fi
  else
    echo "提示：请将 ${INSTALL_DIR} 加入 PATH。"
  fi
fi

echo "==> 安装完成：${INSTALL_DIR}/fanqie"
"${INSTALL_DIR}/fanqie" --version
