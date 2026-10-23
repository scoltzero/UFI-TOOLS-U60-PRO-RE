#!/bin/sh
# One entry for source checkouts and prebuilt bundles. The existing installer
# retains ownership checks, payload verification, rollback and ADB diagnostics.
set -eu
BASE=$(CDPATH='' cd -- "$(dirname "$0")" && pwd)
ACTION=install
case "${1:-}" in
  install|upgrade|uninstall|restore|preview) ACTION=$1; shift ;;
  -h|--help)
    cat <<'EOF'
UFI-TOOLS U60 Pro 一键安装
  ./install.sh                 安装／升级；自动使用唯一已授权的 ADB 设备
  ./install.sh uninstall       卸载候选，保留原版与恢复备份
  ./install.sh restore         恢复此前候选版本
  ./install.sh preview         仅预览安装，不连接或修改设备
  ./install.sh --serial SERIAL 多台设备时指定目标
其他参数（如 --no-autostart）传给现有 ADB 安装器。
预编译安装包无需 Go；源码目录会自动编译，需要 Go 1.26.10。
EOF
    exit 0 ;;
esac

if [ "$ACTION" = preview ]; then
  for option do
    [ "$option" != --execute ] || {
      echo 'preview 不能同时使用 --execute。' >&2
      exit 2
    }
  done
fi

if [ -f "$BASE/install-adb.sh" ] && [ -f "$BASE/service.sh" ]; then
  INSTALLER=$BASE/install-adb.sh
  BUNDLE=$BASE
  SOURCE=0
elif [ -f "$BASE/installer/install-adb.sh" ] && [ -f "$BASE/go.mod" ]; then
  INSTALLER=$BASE/installer/install-adb.sh
  SOURCE=1
else
  echo '找不到完整安装文件，请重新解压安装包或下载项目源码。' >&2
  exit 1
fi

if [ "$ACTION" != preview ]; then
  command -v "${ADB:-adb}" >/dev/null 2>&1 || {
    echo '未找到 adb。请先安装 Android Platform Tools，并连接、授权设备。' >&2
    exit 1
  }
fi

TEMP=
cleanup() { [ -z "$TEMP" ] || rm -rf "$TEMP"; }
trap cleanup 0
trap 'exit 130' INT
trap 'exit 143' HUP TERM

case "$ACTION" in
  uninstall) set -- --uninstall "$@" ;;
  restore) set -- --uninstall --restore "$@" ;;
  *)
    if [ "$SOURCE" = 1 ]; then
      command -v go >/dev/null 2>&1 || {
        echo '源码安装需要 Go 1.26.10。也可以使用预编译安装包，无需安装 Go。' >&2
        exit 1
      }
      TEMP=$(mktemp -d "${TMPDIR:-/tmp}/ufi-oneclick.XXXXXX")
      BUNDLE=$TEMP/bundle
      echo '正在准备安装文件…'
      sh "$BASE/scripts/package-script.sh" "$BUNDLE"
    fi
    set -- --bundle "$BUNDLE" "$@"
    ;;
esac

if [ "$ACTION" = preview ]; then
  sh "$INSTALLER" "$@"
else
  sh "$INSTALLER" --execute "$@"
  echo '操作完成。'
fi
