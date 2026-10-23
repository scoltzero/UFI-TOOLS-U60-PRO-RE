# LAN 脚本安装器

新入口是 `installer/install-lan.sh`，提供 **备份 → 离线准备 → 预览/执行**。不依赖 ADB，不创建 Android/Windows 图形界面。当前支持独立候选版的 **首次安装、按安装记录卸载，以及失败后的反向恢复**；已有候选版升级继续用 ADB 脚本。独立工具包中，将下文 `./installer/install-lan.sh` 换为 `./install-lan.sh`，安装包目录使用 `candidate`。

## 与 ADB 的区别

LAN 使用原厂登录、备份上传和 `device_restore_proc`，**会重启设备并中断网络**。本工具只生成独立暂存目录和启动文件的改动，不覆盖 `/data/ufi-tools`。但是原厂恢复流程自身的其他副作用仍未在真实固件上完成验证，因此不能宣称“LAN恢复对原系统完全无影响”。实机接受容器、重启、恢复和基线比对仍属于主计划第6阶段。

旧的 `cmd/build-lan-backup`、`cmd/install-lan` 原版覆盖式入口已停用，调用会明确退出；不要把旧容器用于新流程。历史源码可从Git历史或本地研究备份查看。

## 准备条件

- 同一局域网内的用户自有 U60 Pro；默认官方后台 `http://192.168.98.1`。
- 原厂后台登录密码文件（不是UFI页面token）。密码不会作为命令行明文参数或输出日志。
- 匹配设备的15位IMEI文件，或备份加密密码文件；后者使用 `--backup-password-file` 替代 `--imei-file`。不尝试提取或破解设备凭据。
- 当前版本生成的ARM64脚本包。旧包没有部署记录/启动支持时会拒绝，请重新构建。
- 目标配置备份必须含常规文件 `data/etc/rc.local`。符号链接条目、复杂/歧义退出逻辑和未清理的旧LAN临时启动项会拒绝，不猜测固件布局。

在仓库内构建：

```sh
make build-lan
./scripts/package-script.sh build/script-bundle-lan
```

`install-lan.sh` 首次使用时也可构建本机CLI（需要Go）；已有CLI后可直接运行。修改代码后重新运行 `make build-lan`，避免使用旧二进制。打包的本机CLI和shell入口放在一起可脱离源码运行；本机CLI是当前构建主机的平台程序，不是通用跨平台二进制。

## 首次安装

### 1. 导出原厂备份

也可以在原厂网页手动下载备份。使用脚本导出：

```sh
./installer/install-lan.sh backup \
  --base http://192.168.98.1 \
  --router-password-file /secure/router-password \
  --out build/vendor-before.bin
```

这条命令会登录并请求原厂导出备份，但**不上传、不请求恢复、不重启**。输出文件0600，已有文件拒绝覆盖。

### 2. 离线准备并检查路径

```sh
./installer/install-lan.sh prepare \
  --action install \
  --bundle build/script-bundle-lan \
  --baseline-backup build/vendor-before.bin \
  --imei-file /secure/device-imei \
  --out build/lan-install-plan
```

生成私人目录：`plan.json`、`candidate.bin`、`rollback.bin`，以及首装时的 `candidate-token`（0600）。后者是新生成的候选后台登录令牌，不是从设备读取的原版凭据；进入2433后台时使用它，请保管好且不要公开。设备只在自己的数据库尚无令牌时写入一次，随后删除临时种子；不会重置用户后来修改的令牌。计划列出路径和哈希，不输出密码、设备标识或启动文件内容。候选容器只写 `data/etc/rc.local` 与唯一的 `/data/ufi-tools-re.lan-stage-…` 暂存树。

启动后先核对临时启动文件，再恢复准备时的内容，最后调用与ADB相同的设备服务脚本完成安装。原程序路径和端口保持原样。候选使用 `/data/ufi-tools-re`、2433端口。若独立候选目录或备份已存在，设备端拒绝首装；不会覆盖原有数据。

### 3. 预览，再显式执行

```sh
# 默认纯本地预览；不登录设备、不下载备份、不上传、不重启
./installer/install-lan.sh run --plan build/lan-install-plan

# 实际执行，必须明确承认重启；run-dir必须是新目录
./installer/install-lan.sh run \
  --plan build/lan-install-plan \
  --base http://192.168.98.1 \
  --router-password-file /secure/router-password \
  --imei-file /secure/device-imei \
  --run-dir build/lan-install-run \
  --execute --acknowledge-reboot
```

执行前先在本地解密复核容器的路径、操作、部署记录和二进制哈希；再重新获取原厂备份，比较启动文件的内容哈希与权限；不一致则在上传前停止。新的加密备份会保存为 `run-dir/baseline.bin`，可用它重新准备计划。核对与原厂恢复不是一个原子事务，执行期间不要同时修改启动文件或设备配置。候选端口已有响应时拒绝首装。

安装成功必须观察到对应部署ID、新进程标识及原版服务可达；不会把原版2333仍在线或任意HTTP 200当作候选成功。默认原版健康地址是同主机2333端口；可通过 `--original-health-url` 显式指定同主机的合适健康入口，候选同样可设 `--candidate-health-url`。拒绝跨主机、URL凭据及重定向。

## LAN 卸载

保留第一次安装的整个 `lan-install-plan` 目录作为安装记录。卸载前重新导出备份，保留用户后来修改的启动内容：

```sh
./installer/install-lan.sh backup \
  --base http://192.168.98.1 \
  --router-password-file /secure/router-password \
  --out build/vendor-before-uninstall.bin

./installer/install-lan.sh prepare \
  --action uninstall \
  --installed-plan build/lan-install-plan \
  --bundle build/script-bundle-lan \
  --baseline-backup build/vendor-before-uninstall.bin \
  --imei-file /secure/device-imei \
  --out build/lan-uninstall-plan

./installer/install-lan.sh run \
  --plan build/lan-uninstall-plan \
  --base http://192.168.98.1 \
  --router-password-file /secure/router-password \
  --imei-file /secure/device-imei \
  --run-dir build/lan-uninstall-run \
  --execute --acknowledge-reboot
```

只处理部署ID与安装记录一致的候选实例；使用保留备份的卸载模式，数据留在 `.uninstalled`，不自动彻底清空。卸载临时启动项替换原有候选启动行，避免已删目录让 `set -e` 提前终止启动脚本。经过ADB升级或手工替换后，旧LAN安装记录可能不再匹配；此时使用ADB卸载，不强行跳过核验。

卸载成功观察条件是原版健康经历不可用→恢复，且候选端口不再可达。若未捕获到短暂重启或被网络隔离，工具会按未确认处理，而不是宣称成功。

## 失败恢复与结果

每个计划都带反向容器：安装失败对应移除同部署候选；卸载失败对应恢复同部署备份。工具保留原厂备份和反向容器副本，并在重启后重新登录再尝试恢复。

`result.json` 记录实际阶段、是否请求恢复、是否得到应答、是否观察到目标/反向结果。`outcome_observed`、`rollback_outcome_observed` 仅证明服务端点状态，不代表整机分区、所有文件或原厂设置已逐字节恢复。固件恢复RPC连接中断可能意味着已经重启，因此会继续观察，不能简单假定“没有执行”。

如果结果是 `needs-inspection`，保存计划目录和run目录，检查设备；不要循环重试未知恢复。部署后的临时文件成功时清理，失败时可能保留用于诊断，启动日志位于 `/data/ufi-tools-re.lan-stage-<计划ID>.log`；默认卸载保留自身数据备份。卸载后若要再次安装或清理这些残留，请使用ADB脚本的恢复/清理流程；LAN首装不会覆盖保留目录。它们不是原厂固件分区镜像。

## 验证范围

离线测试覆盖包校验/目标路径、基线字节与权限、备份错密钥、安装记录、原版启动行保留、登录/上传拒绝、端口已占用、跨主机/重定向、候选身份、反向恢复和重登录；CLI备份、预览无网络和私有输出也有测试。

真实主机进程测试用临时文件模拟原厂套用启动文件，运行候选模拟器，验证首装、错误记录拒绝、卸载、反向恢复和启动文件恢复。**没有向真实U60上传容器，没有实际设备恢复/重启测试。**

```sh
go test ./internal/lanbundle ./internal/laninstall ./internal/ztebackup ./cmd/lan-installer
make test
make lint
go build -buildvcs=false -o /tmp/ufi-host ./cmd/ufi-tools-u60pro
python3 installer/test-lan-bootstrap.py /tmp/ufi-host
```
