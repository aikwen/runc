# cgroup v2

runc fully supports cgroup v2 (unified mode) since v1.0.0-rc93.

从 v1.0.0-rc93 版本开始，runc 已全面支持 Cgroups v2（统一层级模式，Unified Mode）。

To use cgroup v2, you might need to change the configuration of the host init system.
The following distributions are known to use cgroup v2 by default:

要使用 Cgroups v2，可能需要修改宿主机初始化系统（init system）的配置。
目前已知以下 Linux 发行版默认使用 Cgroups v2：

<!-- the list should be kept in sync with https://github.com/rootless-containers/rootlesscontaine.rs/blob/master/content/getting-started/common/cgroup2.md -->
- Fedora (since 31)
- Arch Linux (since April 2021)
- openSUSE Tumbleweed (since c. 2021)
- Debian GNU/Linux (since 11)
- Ubuntu (since 21.10)
- RHEL and RHEL-like distributions (since 9)

On other systemd-based distros, cgroup v2 can be enabled by adding `systemd.unified_cgroup_hierarchy=1` to the kernel cmdline.

对于其他基于 systemd 的 Linux 发行版，可以通过在内核启动参数中添加 systemd.unified_cgroup_hierarchy=1 来启用 Cgroups v2。

## Am I using cgroup v2?

Yes if `/sys/fs/cgroup/cgroup.controllers` is present.

如果系统中存在 /sys/fs/cgroup/cgroup.controllers 文件，就说明该位置使用的是 Cgroups v2。

## Host Requirements
### Kernel
* Recommended version: 5.2 or later
* Minimum version: 4.15

Kernel older than 5.2 is not recommended due to lack of freezer.

推荐使用 Linux 5.2 或更高版本的内核。
最低要求为 Linux 4.15。
不推荐使用低于 5.2 的内核，因为这些版本缺少 Cgroups v2 的 Freezer 功能。

Notably, kernel older than 4.15 MUST NOT be used (unless you are running containers with user namespaces), as it lacks support for controlling permissions of devices.

特别需要注意的是，不得使用低于 Linux 4.15 的内核，除非容器使用了 User Namespace，因为这些旧内核缺乏对设备访问权限进行控制所需的支持。

### Systemd
On cgroup v2 hosts, it is highly recommended to run runc with the systemd cgroup driver (`runc --systemd-cgroup`), though not mandatory.

在使用 Cgroups v2 的宿主机上，强烈建议通过 **systemd Cgroup Driver** 运行 runc（即使用 runc --systemd-cgroup），但这不是强制要求。

The recommended systemd version is 244 or later. Older systemd does not support delegation of `cpuset` controller.

推荐使用 systemd 244 或更高版本。更早的 systemd 版本不支持对 cpuset 控制器进行权限委派。

Make sure you also have the `dbus-user-session` (Debian/Ubuntu) or `dbus-daemon` (CentOS/Fedora) package installed, and that `dbus` is running. On Debian-flavored distros, this can be accomplished like so:

还需要确保安装了 dbus-user-session（Debian/Ubuntu）或 dbus-daemon（CentOS/Fedora）软件包，并且 D-Bus 服务正在运行。

对于 Debian 系发行版，文档提供了以下命令：

```bash
sudo apt install -y dbus-user-session
systemctl --user start dbus
```

> D-Bus 是 Linux 上常见的进程间通信（IPC）机制。runc 的 systemd driver 可以通过它向 systemd 发出创建和管理 Cgroup 的请求。

## Rootless
On cgroup v2 hosts, rootless runc can talk to systemd to get cgroup permissions to be delegated.

在使用 Cgroups v2 的宿主机上，以非 root 模式运行的 runc 可以通过与 systemd 通信，获得被委派的 Cgroup 管理权限。

- Rootless：无需以宿主机 root 身份运行容器运行时和容器进程。
- Delegation（权限委派）：systemd 将某个 Cgroup 子树的管理权限授予普通用户或其服务。

```bash
runc spec --rootless 
# 生成适用于 Rootless 容器的 config.json，其中包括 User Namespace 等相关配置。
jq '.linux.cgroupsPath="user.slice:runc:foo"' config.json | sponge config.json
# jq：修改 JSON 数据中的 .linux.cgroupsPath。
# sponge：读取完整输入后再写回文件，避免直接重定向导致原文件提前被清空。
# user.slice:runc:foo：systemd driver 使用的 Cgroup 路径描述格式。
runc --systemd-cgroup run foo
# 使用 systemd Cgroup Driver 启动名为 foo 的容器。
```

The container processes are executed in a cgroup like `/user.slice/user-$(id -u).slice/user@$(id -u).service/user.slice/runc-foo.scope`.

容器进程将在类似下面的 Cgroup 路径中运行：
/sys/fs/cgroup/
└── user.slice/
    └── user-1000.slice/
        └── user@1000.service/
            └── user.slice/
                └── runc-foo.scope/

> 普通用户的容器并没有直接放在 /sys/fs/cgroup/ 根目录下，而是放在 systemd 为用户管理的层级中。

### Configuring delegation
Typically, only `memory` and `pids` controllers are delegated to non-root users by default.

通常情况下，系统默认只向非 root 用户委派 memory 和 pids 两种资源控制器。

```console
$ cat /sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service/cgroup.controllers
# 检查命令
memory pids
# 这表示该 Cgroup 下可供子级启用的控制器包括内存和进程数量控制器。
```

> cgroup.controllers 显示的是可用控制器，并不直接等同于当前已经启用的控制器；后者可以通过 cgroup.subtree_control 查看。


To allow delegation of other controllers, you need to change the systemd configuration as follows:

通过 systemd 的 Delegate 属性进行配置委派更多控制器：

```bash
sudo mkdir -p /etc/systemd/system/user@.service.d
cat <<EOF | sudo tee /etc/systemd/system/user@.service.d/delegate.conf
[Service]
Delegate=cpu cpuset io memory pids
EOF
sudo systemctl daemon-reload
```

控制器	       功能
cpu	      CPU 时间分配与限制
cpuset	  限制可使用的 CPU 核心及内存节点
io	      块设备 I/O 资源控制
memory	  内存使用控制
pids	    进程数量限制