// runc is a command line client for running applications packaged according to
// the Open Container Initiative (OCI) format and is a compliant implementation
// of the Open Container Initiative specification.
// runc 是一个命令行客户端，用于运行按照开放容器倡议（Open Container Initiative，OCI）
// 格式打包的应用程序，同时也是 OCI 规范的一个符合标准的具体实现。
package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	//nolint:revive // Enable cgroup manager to manage devices
	_ "github.com/opencontainers/cgroups/devices"
	"github.com/opencontainers/runc/libcontainer/seccomp"
	"github.com/opencontainers/runtime-spec/specs-go"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v3"
)

// version is set from the contents of VERSION file.
//
//go:embed VERSION
var version string

// extraVersion is an optional suffix appended to runc version.
// It can be set via Makefile ("make EXTRA_VERSION=xxx") or by
// adding -X main.extraVersion=xxx option to the go build command.
var extraVersion = ""

// gitCommit will be the hash that the binary was built from
// and will be populated by the Makefile.
var gitCommit = ""

// 编译时可通过 go build -ldflags="-X 包路径.变量名=值" 设置包级字符串变量的初始值，源码本身不会被修改。
// 比如：
// go build -ldflags="-X main.extraVersion=-dev -X main.gitCommit=abc123"

func printVersion(c *cli.Command) {
	w := c.Writer

	fmt.Fprintln(w, "runc version", c.Version)
	if gitCommit != "" {
		fmt.Fprintln(w, "commit:", gitCommit)
	}
	fmt.Fprintln(w, "spec:", specs.Version)
	fmt.Fprintln(w, "go:", runtime.Version())

	major, minor, micro := seccomp.Version()
	if major+minor+micro > 0 {
		fmt.Fprintf(w, "libseccomp: %d.%d.%d\n", major, minor, micro)
	}

	// 根据 Build Tags 选择不同的实现；仅启用 libpathrs 标签时才获取并输出其版本信息。
	if v := pathrsVersionString(); v != "" {
		fmt.Fprintf(w, "libpathrs: %s\n", v)
	}
}

const (
	specConfig = "config.json"
	usage      = `Open Container Initiative runtime

runc is a command line client for running applications packaged according to
the Open Container Initiative (OCI) format and is a compliant implementation of
the Open Container Initiative specification.

runc integrates well with existing process supervisors to provide a production
container runtime environment for applications. It can be used with your
existing process monitoring tools and the container will be spawned as a
direct child of the process supervisor.

Containers are configured using bundles. A bundle for a container is a directory
that includes a specification file named "` + specConfig + `" and a root filesystem.
The root filesystem contains the contents of the container.

To start a new instance of a container:

    # runc run [ -b bundle ] <container-id>

Where "<container-id>" is your name for the instance of the container that you
are starting. The name you provide for the container instance must be unique on
your host. Providing the bundle directory using "-b" is optional. The default
value for "bundle" is the current directory.`
)

// runc 是一个命令行客户端，用于运行按照开放容器倡议（Open Container Initiative，OCI）格式打包的应用程序，同时也是 OCI 规范的一个符合标准的实现。
// runc 可以很好地与现有的进程监管器（Process Supervisor） 集成，为应用程序提供生产级容器运行时环境。它可以与现有的进程监控工具配合使用，并且容器进程会作为进程监管器的直接子进程创建。
// 容器通过 Bundle（容器包） 进行配置。一个容器 Bundle 是一个目录，其中包含一个名为 config.json 的规范配置文件，以及一个根文件系统（Root Filesystem，rootfs）。
// 根文件系统包含容器内部的文件内容。
// 要启动一个新的容器实例，可以执行：
// runc run [-b bundle] <container-id>
// 其中，<container-id> 是你为即将启动的容器实例指定的名称。
// 该名称必须在宿主机上对应的 runc 状态目录中保持唯一。
// 可以通过 -b 参数指定 Bundle 目录，该参数是可选的。如果没有指定，则默认使用当前工作目录。

// 一个典型的 OCI Bundle 目录如下：
// bundle/
// ├── config.json
// └── rootfs/
//       ├── bin/
//       ├── etc/
//       ├── usr/
//       └── ...


func main() {
	app := &cli.Command{}
	app.Name = "runc"
	app.Version = strings.TrimSpace(version) + extraVersion
	app.Usage = usage
	// Disable comma as separator for slice flags.
	// 禁止将逗号作为切片类型命令行参数的分隔符。
	// --args alice,bob 不会被解析为 ["alice", "bob"]
	app.DisableSliceFlagSeparator = true

	cli.VersionPrinter = printVersion

	// 保存容器 ID、进程信息、运行状态等数据
	// Linux 中的 /run 通常是基于 tmpfs 的运行时目录，适合存储这类临时状态。
	root := "/run/runc"
	// XDG_RUNTIME_DIR 是 XDG Base Directory Specification 定义的环境变量，表示当前用户用于保存运行时数据的目录。
	// 对于 UID 为 1000 的普通用户，常见的值是 /run/user/1000
	xdgDirUsed := false
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")

	// XDG_RUNTIME_DIR 环境变量不为空 和 shouldHonorXDGRuntimeDir() 返回 true
	if xdgRuntimeDir != "" && shouldHonorXDGRuntimeDir() {
		root = xdgRuntimeDir + "/runc"
		xdgDirUsed = true
	}

	// 注册参数
	// 参数	             类型	作用
	// --debug	        BoolFlag	开启调试日志
	// --log	        StringFlag	指定日志文件
	// --log-format	    StringFlag	指定日志格式，默认为 text
	// --root	        StringFlag	指定容器运行时状态目录
	// --systemd-cgroup	BoolFlag	使用 systemd Cgroup Driver
	// --rootless	    StringFlag	控制非特权 Cgroup 操作错误的处理方式，默认为 auto
	app.Flags = []cli.Flag{
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug logging",
		},
		&cli.StringFlag{
			Name:  "log",
			Value: "",
			Usage: "set the log file to write runc logs to (default is '/dev/stderr')",
		},
		&cli.StringFlag{
			Name:  "log-format",
			Value: "text",
			Usage: "set the log format ('text' (default), or 'json')",
		},
		&cli.StringFlag{
			Name:  "root",
			Value: root,
			// 用于保存容器状态的根目录，该目录应该位于 tmpfs 文件系统上。
			Usage: "root directory for storage of container state (this should be located in tmpfs)",
		},
		&cli.BoolFlag{
			Name:  "systemd-cgroup",
			Usage: "enable systemd cgroup support, expects cgroupsPath to be of form \"slice:prefix:name\" for e.g. \"system.slice:runc:434234\"",
		},
		&cli.StringFlag{
			Name:  "rootless",
			Value: "auto",
			Usage: "ignore cgroup permission errors ('true', 'false', or 'auto')",
		},
	}
	// 注册子命令
	app.Commands = []*cli.Command{
		checkpointCommand,
		createCommand,
		deleteCommand,
		eventsCommand,
		execCommand,
		killCommand,
		listCommand,
		pauseCommand,
		psCommand,
		restoreCommand,
		resumeCommand,
		runCommand,
		specCommand,
		startCommand,
		stateCommand,
		updateCommand,
		featuresCommand,
	}

	// 在执行子命令的 Action 之前，完成一些初始化操作。
	app.Before = func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
		// !cmd.IsSet("root")：用户没有显式指定 --root 参数。
		// xdgDirUsed：此前选择了 XDG 运行时目录。
		if !cmd.IsSet("root") && xdgDirUsed {
			// According to the XDG specification, we need to set anything in
			// XDG_RUNTIME_DIR to have a sticky bit if we don't want it to get
			// auto-pruned.
			// 创建目录及其缺失的父目录，权限设置 rwx------
			if err := os.MkdirAll(root, 0o700); err != nil {
				fmt.Fprintln(os.Stderr, "the path in $XDG_RUNTIME_DIR must be writable by the user")
				fatal(err)
			}
			// 设置 0700 权限，还增加了 Sticky Bit
			if err := os.Chmod(root, os.FileMode(0o700)|os.ModeSticky); err != nil {
				fmt.Fprintln(os.Stderr, "you should check permission of the path in $XDG_RUNTIME_DIR")
				fatal(err)
			}
		}
		// 负责处理用户指定的 --root 路径，包括转换为绝对路径，以及执行必要的路径检查。
		if err := reviseRootDir(cmd); err != nil {
			return ctx, err
		}

		if err := configLogrus(cmd); err != nil {
			return ctx, err
		}

		return ctx, nil
	}

	// If the command returns an error, cli takes upon itself to print
	// the error on cli.ErrWriter and exit.
	// Use our own writer here to ensure the log gets sent to the right location.
	// 如果命令返回错误，CLI 框架会自行将错误信息输出到 cli.ErrWriter，并以错误状态退出。
	// 这里使用我们自定义的 Writer，以确保错误日志被写入正确的位置。
	cli.ErrWriter = &FatalWriter{cli.ErrWriter}
	// 例如执行 runc create demo 时，os.Args 通常为 ["runc", "create", "demo"]。
	if err := app.Run(context.Background(), os.Args); err != nil {
		fatal(err)
	}
}

type FatalWriter struct {
	cliErrWriter io.Writer
}

func (f *FatalWriter) Write(p []byte) (n int, err error) {
	logrus.Error(string(p))
	// 判断 Logrus 当前是否直接向标准错误输出 stderr 写日志。
	// 如果是，那么不再通过原始 CLI Writer 输出，避免重复
	// 如果是日志文件之类的，那么再通过 cliErrWriter.Write(p) 输出一次
	if !logrusToStderr() {
		return f.cliErrWriter.Write(p)
	}
	return len(p), nil
}

// 根据 CLI 参数配置日志级别、调用位置、输出格式和输出目标。
func configLogrus(cmd *cli.Command) error {
	if cmd.Bool("debug") {
		logrus.SetLevel(logrus.DebugLevel)
		logrus.SetReportCaller(true)
		// Shorten function and file names reported by the logger, by
		// trimming common "github.com/opencontainers/runc" prefix.
		// This is only done for text formatter.
		_, file, _, _ := runtime.Caller(0)
		prefix := filepath.Dir(file) + "/"
		logrus.SetFormatter(&logrus.TextFormatter{
			CallerPrettyfier: func(f *runtime.Frame) (string, string) {
				function := strings.TrimPrefix(f.Function, prefix) + "()"
				fileLine := strings.TrimPrefix(f.File, prefix) + ":" + strconv.Itoa(f.Line)
				return function, fileLine
			},
		})
	}

	switch f := cmd.String("log-format"); f {
	case "":
		// do nothing
	case "text":
		// do nothing
	case "json":
		logrus.SetFormatter(new(logrus.JSONFormatter))
	default:
		return errors.New("invalid log-format: " + f)
	}

	if file := cmd.String("log"); file != "" {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		logrus.SetOutput(f)
	}

	return nil
}
