#!/bin/sh
# Finder entry for the macOS prebuilt bundle or source checkout.
cd "$(dirname "$0")" || exit 1
sh ./install.sh
ufi_status=$?
if [ -t 0 ]; then
  printf '\n按回车关闭窗口…'
  IFS= read -r ufi_answer || :
fi
exit "$ufi_status"
