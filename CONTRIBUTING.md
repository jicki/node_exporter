# Contributing

Prometheus uses GitHub to manage reviews of pull requests.

* If you have a trivial fix or improvement, go ahead and create a pull request,
  addressing (with `@...`) the maintainer of this repository (see
  [MAINTAINERS.md](MAINTAINERS.md)) in the description of the pull request.

* If you plan to do something more involved, first discuss your ideas
  on our [mailing list](https://groups.google.com/forum/?fromgroups#!forum/prometheus-developers).
  This will avoid unnecessary work and surely give you and us a good deal
  of inspiration.

* Relevant coding style guidelines are the [Go Code Review
  Comments](https://code.google.com/p/go-wiki/wiki/CodeReviewComments)
  and the _Formatting and style_ section of Peter Bourgon's [Go: Best
  Practices for Production
  Environments](http://peter.bourgon.org/go-in-production/#formatting-and-style).

* Sign your work to certify that your changes were created by yourself or you
  have the right to submit it under our license. Read
  https://developercertificate.org/ for all details and append your sign-off to
  every commit message like this:

        Signed-off-by: Random J Developer <example@example.com>


## Collector Implementation Guidelines

The Node Exporter is not a general monitoring agent. Its sole purpose is to
expose machine metrics, as oppose to service metrics, with the only exception
being the textfile collector.

The metrics should not get transformed in a way that is hardware specific and
would require maintaining any form of vendor based mappings or conditions. If
for example a proc file contains the magic number 42 as some identifier, the
Node Exporter should expose it as it is and not keep a mapping in code to make
this human readable. Instead, the textfile collector can be used to add a static
metric which can be joined with the metrics exposed by the exporter to get human
readable identifier.

A Collector may only read `/proc` or `/sys` files, use system calls or local
sockets to retrieve metrics. It may not require root privileges. Running
external commands is not allowed for performance and reliability reasons. Use a
dedicated exporter instead or gather the metrics via the textfile collector.

The Node Exporter tries to support the most common machine metrics. For more
exotic metrics, use the textfile collector or a dedicated Exporter.

## 跨平台验证

`golangci-lint` 按当前 `GOOS`、`GOARCH` 和构建标签选择源码；Linux lint 通过不代表
Darwin 也通过。平台专用声明应与其调用方使用兼容的构建约束。移动声明时还需保留
已有 CLI 参数、默认值及指标行为，例如非 Linux 平台仍接受 `--path.udev.data`。

已准备好 Makefile 指定版本的工具后，可在 macOS 上分别执行：

```bash
make style check_license lint
go test -short ./...
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 "$(go env GOPATH)/bin/golangci-lint" run ./...
```

交叉 lint 和构建不能替代 Linux 上的测试执行；涉及 Linux collector 时，还需在
Linux 环境运行相关回归测试。`make all` 会按需安装工具、执行 `go mod tidy`，并在
fixture 需要更新时删除后重新解包 `collector/fixtures/sys` 和 `collector/fixtures/udev`。
需要保留工作区中的 fixture 时，应在临时副本中解包和测试。
