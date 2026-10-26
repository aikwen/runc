# Spec conformance

This branch of runc implements the [OCI Runtime Spec v1.3.0](https://github.com/opencontainers/runtime-spec/tree/v1.3.0)
for the `linux` platform.

当前 runc 分支实现了适用于 Linux 平台的 OCI 运行时规范 v1.3.0。

## Deviations

runc deviates from the spec in the following ways:

* [`ociVersion`](https://github.com/opencontainers/runtime-spec/blob/v1.3.0/config.md#specification-version)
  is not validated: runc does not generate an error if it is missing, empty, not
  in SemVer format, or specifies a version not supported by runc (e.g. with a
  different major version).
* If [`umask`](https://github.com/opencontainers/runtime-spec/blob/v1.3.0/config.md#user)
  is not specified, the umask of the container init process is set to `0022`,
  rather than left unchanged from the calling process' umask. For `runc exec`,
  the umask is left unchanged.

runc 在以下方面与 OCI 运行时规范存在差异：
- ociVersion 字段不进行校验：如果该字段缺失、为空、不符合语义化版本（SemVer）格式，或者指定了 runc 不支持的版本（例如主版本号不同），runc 也不会因此报错。
- umask 的默认行为不同：如果没有指定 umask，容器初始化进程的 umask 将被设置为 0022，而不是保持调用进程的 umask 不变。对于 runc exec，则保持原有的 umask 不变。

## Architectures

The following architectures are supported:

- runc binary:runc 程序本身所运行的 CPU 架构。
- Seccomp：限制进程能够执行哪些系统调用。

runc binary  | seccomp
-------------|-------------------------------------------------------
`amd64`      | `SCMP_ARCH_X86`, `SCMP_ARCH_X86_64`, `SCMP_ARCH_X32`
`arm64`      | `SCMP_ARCH_ARM`, `SCMP_ARCH_AARCH64`
`armel`      | `SCMP_ARCH_ARM`
`armhf`      | `SCMP_ARCH_ARM`
`ppc64le`    | `SCMP_ARCH_PPC64LE`
`riscv64`    | `SCMP_ARCH_RISCV64`
`s390x`      | `SCMP_ARCH_S390`, `SCMP_ARCH_S390X`
`loong64`    | `SCMP_ARCH_LOONGARCH64`

The runc binary might be compilable for i386, big-endian PPC64,
and several MIPS variants too, but these architectures are not officially supported.

此外，runc 二进制程序也可能能够在 i386、大端序 PPC64 以及部分 MIPS 架构上成功编译，但这些架构不属于官方正式支持的范围。