# UFI-TOOLS U60 Pro RE

面向 ZTE U60 Pro（MU5250）的 UFI-TOOLS 主要功能复刻项目，使用 Go 编写，提供网页管理界面与独立安装脚本。

当前为首个测试版本。目标是主要功能可用、设置可保存、错误能明确反馈；不追求与原版逐字段、逐时序完全一致。项目基于获授权的研究开展，用于个人学习与独立实现。

## 功能

- 首页状态：CPU、内存、温度、电池、存储、实时速率及蜂窝信号。
- 网络管理：蜂窝连接、Wi-Fi、APN、LAN／DHCP、接入设备及访问控制。
- 短信收发与转发、定时任务、流量统计和限额。
- 候选自身的文件、Hosts、日志、后台设置及插件接口。
- ADB 独立安装、升级、卸载和版本恢复。

部分功能已有实机验证，其余仍需日常使用验证。终端、AT 等能力还需要设备上存在对应辅助程序。详细进度见 [PLAN.md](PLAN.md)。

## 环境要求

- **设备**：已开启并授权 ADB、具备 root shell 的 U60 Pro，Linux ARM64。
- **构建电脑**：macOS 或 Linux，Go 1.26.10、Git、POSIX Shell。
- **安装工具**：Android Platform Tools 中的 `adb`。
- **开发测试**：另需 Node.js 18 或更新版本、Python 3 和 ripgrep（`rg`），用于网页与安装器回归。

确认设备连接及权限：

```sh
adb devices
adb -s YOUR_SERIAL shell id
```

将下文的 `YOUR_SERIAL` 替换为设备序列号。设备端应具有 root 权限；安装器不会自动开启调试或获取 root。

## 下载与构建

```sh
git clone --branch u60pro-parity https://github.com/scoltzero/UFI-TOOLS-U60-PRO-RE.git
cd UFI-TOOLS-U60-PRO-RE

VERSION=0.0.1 ./scripts/package-script.sh
```

输出目录为 `build/script-bundle-0.0.1/`，其中包含主程序、设备服务脚本、安装器、许可证和校验清单。它可以作为一个完整文件夹携带使用。构建脚本不会覆盖已有输出；再次构建时可指定新目录：

```sh
VERSION=0.0.1 ./scripts/package-script.sh build/script-bundle-test2
```

## 安装

先预览操作；不加 `--execute` 时不会连接或修改设备：

```sh
./installer/install-adb.sh --bundle build/script-bundle-0.0.1
```

执行安装：

```sh
./installer/install-adb.sh --execute --serial YOUR_SERIAL \
  --bundle build/script-bundle-0.0.1
```

默认安装位置：

| 项目 | 默认值 |
| --- | --- |
| 候选程序目录 | `/data/ufi-tools-re` |
| 候选数据目录 | `/data/ufi-tools-re/data` |
| 网页端口 | `2433` |
| 原版程序目录与端口 | `/data/ufi-tools`、`2333`，保留 |

安装完成后访问 `http://设备IP:2433/`，例如 [http://192.168.98.1:2433/](http://192.168.98.1:2433/)。使用设备提供的网页地址，无需直接打开仓库里的 HTML 文件。

安装器只管理候选自己的目录、进程和自启项，不重启整机，也不修改原版数据库、Wi-Fi、USB 或系统网络设置。可用 `--no-autostart` 取消候选自启登记。

## 首次登录与使用

首次启动会生成候选登录口令。在你自己的终端查看：

```sh
adb -s YOUR_SERIAL shell cat /data/ufi-tools-re/data/device-token
```

使用该口令登录首页。首页底部的“候选系统管理”入口提供文件、Hosts、日志和后台设置；它与首页共用登录。

可以在首页的“更改口令”中自行设置新口令，支持 8～256 位英文、数字或符号，不含空格。也可以手动设成与 80 端口原厂管理页相同的密码；这不会修改原厂密码，也不会自动跟随原厂之后的改密。首次口令文件不是网页改密后的实时查询接口，之后以你设置的新口令为准。

### 启用设备控制

新安装默认关闭设备写入、后台任务和 AT；查看状态不需要开启它们。需要使用控制功能时，按需编辑设备上的 `/data/ufi-tools-re/.ufi-runtime`：

| 设置 | 用途 | 默认值 |
| --- | --- | --- |
| `PRIVILEGED_COMPAT` | 手动设备写入和固定特权操作 | `false` |
| `WORKERS_ENABLED` | 调度、流量采样、转发等后台执行 | `false` |
| `AT_ENABLED` | AT 指令辅助程序 | `false` |

例如，仅需要手动控制时，将 `PRIVILEGED_COMPAT=false` 改为 `PRIVILEGED_COMPAT=true`，其余保持原值。修改后只重开候选服务：

```sh
adb -s YOUR_SERIAL shell sh /data/ufi-tools-re/service.sh restart
```

开启后台前，应核对已经保存的任务和设置。开启开关不会安装缺失的辅助工具；设备实际控制会改变对应设置。

## 升级

构建新目录后，再执行一次安装命令即可升级：

```sh
./installer/install-adb.sh --execute --serial YOUR_SERIAL \
  --bundle build/script-bundle-test2
```

升级保留候选自身的数据；旧版本保存于 `/data/ufi-tools-re.previous`。安装器会校验文件、检查空间并核对新服务健康状态，失败时尝试回退。

## 卸载与恢复

卸载候选，保留 2333 原版：

```sh
./installer/install-adb.sh --execute --serial YOUR_SERIAL --uninstall
```

默认行为是停止候选、移除所属自启项，并将当前程序和数据移至 `/data/ufi-tools-re.uninstalled`，以便恢复。已有同名备份时拒绝覆盖；更多清理选项见 [安装器说明](installer/README.md#卸载和恢复)。

恢复候选版本：

```sh
./installer/install-adb.sh --execute --serial YOUR_SERIAL --uninstall --restore
```

恢复优先使用 `.previous`，没有时使用 `.uninstalled`。此操作只恢复候选版本，不恢复或覆盖整机固件。

## 测试状态与限制

已实机验证独立安装、同包升级、默认卸载、恢复、回退，以及登录、首页读数、配置保留和部分 Wi-Fi／APN 操作。当前测试设备上的候选已按使用者要求卸载，原版保留；需要时可按上面的步骤重新安装。

以下项目不作为首个测试版的完整实机通过结论：

- 全部网络模式、APN 实际拨号、LAN／DHCP、访问控制、SIM／锁频等组合。
- 已记录的仅 5GHz 模式限制，以及部分设备操作后的完整恢复问题。
- 全部系统辅助功能、插件真实屏幕／触摸效果。
- 长期稳定性及真实整机开机后的自启。
- LAN 恢复式安装。该入口可能重启或断开设备，尚无完整实机验收，首次使用建议采用 ADB。

详情见 [实机验收清单](docs/stage4-human-validation.md)和 [LAN 安装说明](installer/lan-backup/README.md)。

## 本地开发

开发模式默认使用模拟器，不会自动操作连接的设备：

```sh
go run ./cmd/ufi-tools-u60pro \
  --data-dir "$PWD/tmp/data" \
  --http-addr 127.0.0.1:2433
```

打开 [http://127.0.0.1:2433/](http://127.0.0.1:2433/)，首次口令位于 `tmp/data/device-token`。

常用检查：

```sh
make test
make lint
make installer-test
make build-arm64
```

可选的 `make stage5-integration` 使用相邻的 `ufitools_plugins` 工作区、Chrome、C 编译器和 Python，用于配套组件本地联调；它不属于普通安装所需步骤。

## 项目结构

```text
cmd/          程序及命令行入口
internal/     设备适配、接口、存储、网页及后台实现
installer/    安装、升级、卸载与恢复脚本
scripts/      构建和联调工具
docs/         功能清单、安装边界和验收说明
```

## 许可证与研究材料

新代码采用 [AGPL-3.0-or-later](LICENSE)。网页复用了 [kanoqwq/UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) 的 MIT 授权代码，对应版权和许可说明保留于 [NOTICE](NOTICE)及 `LICENSES/`。

仓库仅提交独立实现、测试和项目文档，不包含原厂闭源程序、固件、设备数据库、密码、取证记录或本地构建产物。`research/`、`research-inputs/`、`output/` 和 `build/` 均为本地目录，不纳入提交。原问题库作为研究参考保留，当前开发范围以主计划为准。
