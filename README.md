# UFI-TOOLS U60 Pro RE

面向 ZTE U60 Pro（MU5250）的 UFI-TOOLS 主要功能复刻项目，提供网页管理界面和独立安装脚本。当前为首个测试版，部分硬件功能仍待日常使用验证。

## 一键安装

**通过电脑 USB／ADB 安装，保留原版 2333，候选使用 2433。** 脚本支持 macOS／Linux；电脑需要有 `adb`，设备需已授权 USB 调试，并具备 root shell。

### 使用预编译安装包（推荐，无需 Go）

1. 在仓库 [Actions](https://github.com/scoltzero/UFI-TOOLS-U60-PRO-RE/actions/workflows/ci.yml) 中打开最近一次成功的 `ci` 运行，下载 **UFI-TOOLS-U60-Pro-beta** 安装包。
2. 解压到能看到 `install.sh` 的文件夹，连接设备。
3. **Mac 可双击 `install.command`**；也可以在该文件夹的终端执行：

```sh
sh install.sh
```

脚本会自动使用唯一已授权的 ADB 设备，完成校验、安装和启动。不需要手动编译、填写序列号或指定安装包路径。安装包可以离线使用。

安装完成后打开 [http://192.168.98.1:2433/](http://192.168.98.1:2433/)；设备 IP 不同时使用对应 IP。原版的 `2333` 保留。

> Actions 下载文件可能包含一层 `.tar.gz`，继续解压即可。Mac 双击脚本若因执行权限无法启动，使用上面的 `sh install.sh`。

### 从源码安装

电脑另需 Git 和 Go 1.26.10。复制这一段即可；脚本会自动构建安装包：

```sh
git clone https://github.com/scoltzero/UFI-TOOLS-U60-PRO-RE.git
cd UFI-TOOLS-U60-PRO-RE
sh install.sh
```

已有源码或安装包时，直接运行 `sh install.sh` 即安装／升级，不需要重复下载源码。

## 登录

首次启动会生成候选口令，在你自己的终端查看：

```sh
adb shell cat /data/ufi-tools-re/data/device-token
```

用它登录首页。底部的“候选系统管理”提供文件、Hosts、日志和后台设置，与首页共用登录。

可以通过首页“更改口令”设置新口令，也可自行设成与 80 端口原厂页相同的密码；候选不会读取、修改或自动同步原厂密码。改口令后以新口令为准，初始口令文件不是实时密码查询接口。

## 升级、卸载和恢复

在源码或完整安装包目录中运行：

```sh
# 安装或升级，保留候选数据
sh install.sh

# 卸载候选，保留原版 2333 和恢复备份
sh install.sh uninstall

# 恢复此前的候选版本
sh install.sh restore

# 只预览安装，不连接或修改设备
sh install.sh preview
```

多台 ADB 设备同时连接时才需要指定目标：

```sh
sh install.sh --serial YOUR_SERIAL
```

默认候选目录为 `/data/ufi-tools-re`，数据位于其中的 `data/`。卸载后保留 `/data/ufi-tools-re.uninstalled`；恢复优先使用 `.previous`，没有时使用 `.uninstalled`。安装器只管理候选自己的进程和自启项，不重启整机或覆盖原版。

更多选项见 [安装器说明](installer/README.md)。

## 功能与使用开关

- 首页状态：CPU、内存、温度、电池、存储、速率和蜂窝信号。
- 网络管理：蜂窝连接、Wi-Fi、APN、LAN／DHCP、接入设备和访问控制。
- 短信收发／转发、定时任务、流量统计和限额。
- 文件、Hosts、日志、后台设置及插件接口。

**新安装默认只读。** 需要控制设备时，按需编辑设备上的 `/data/ufi-tools-re/.ufi-runtime`：

| 设置 | 用途 | 默认值 |
| --- | --- | --- |
| `PRIVILEGED_COMPAT` | 手动设备写入和固定特权操作 | `false` |
| `WORKERS_ENABLED` | 调度、流量采样、转发等后台任务 | `false` |
| `AT_ENABLED` | AT 辅助程序 | `false` |

只将需要的项目改为 `true`，然后重开候选服务：

```sh
adb shell sh /data/ufi-tools-re/service.sh restart
```

开启后台前核对已保存的任务；终端、AT 等功能还需要对应辅助程序。安装脚本不会自动开启这些能力。

## 测试状态

已实机验证独立安装、升级、卸载、恢复、回退，以及登录、首页读数、配置保留和部分 Wi-Fi／APN 操作。

全部网络控制组合、部分操作后的完整恢复、辅助工具、真实屏幕／触摸、长期稳定性与自然开机自启仍待后续验证。LAN 恢复式安装可能重启或断连，尚未完整实测；本版本推荐使用上面的 ADB 安装。

详细进度见 [主计划](PLAN.md)和[实机验收清单](docs/stage4-human-validation.md)。

## 开发

构建需要 Go 1.26.10；测试另需 Node.js 18+、Python 3 和 ripgrep（`rg`）。

```sh
make test lint
make installer-test
make build-arm64
```

本地模拟运行，不会自动操作连接的设备：

```sh
go run ./cmd/ufi-tools-u60pro --data-dir "$PWD/tmp/data" --http-addr 127.0.0.1:2433
```

制作可携带的一键安装包：

```sh
./scripts/package-script.sh build/my-install-package
```

## 许可证

项目基于获授权的研究，用于个人学习与独立实现。新代码采用 [AGPL-3.0-or-later](LICENSE)；复用的 [kanoqwq/UFI-TOOLS](https://github.com/kanoqwq/UFI-TOOLS) 网页代码保留 MIT 许可，见 [NOTICE](NOTICE)和 `LICENSES/`。

仓库不包含原厂闭源程序、固件、设备数据库、密码或原始取证材料；本地研究记录和构建产物不纳入 Git 提交。
