# GPU Collector 使用说明

GPU collector 用于从 Linux PCI sysfs 中导出 GPU 设备清单，并从 NVIDIA 驱动的 procfs 信息中补充可用的 GPU UUID。它关注设备是否存在、属于哪个厂商和型号，不读取显存、温度、利用率等运行时指标。

## 启用方式

`gpu` collector 在当前版本中默认启用。正常启动 `node_exporter` 即可采集：

```bash
node_exporter
```

如果使用 `--collector.disable-defaults`，需要显式启用：

```bash
node_exporter --collector.disable-defaults --collector.gpu
```

如需禁用：

```bash
node_exporter --no-collector.gpu
```

容器运行时需要确保同一宿主机的 `/sys` 和 `/proc` 可见。常见方式是挂载 host 根目录，并分别指定 rootfs、sysfs 和 procfs：

```bash
docker run --rm \
  --net=host \
  --pid=host \
  -v /:/host:ro,rslave \
  node-exporter:latest \
  --path.rootfs=/host \
  --path.sysfs=/host/sys \
  --path.procfs=/host/proc
```

`--path.rootfs` 不会自动修改 sysfs 或 procfs 的读取路径。只配置 rootfs 不能保证 UUID 来自目标宿主机；上述三个路径应指向同一宿主环境。

## 采集条件

设备必须同时满足以下条件才会导出 GPU 指标：

- PCI class 以 `0x03` 开头，即 display controller。
- vendor ID 是 NVIDIA `0x10de`、AMD `0x1002` 或 Intel `0x8086`。
- 已绑定 GPU 或透传驱动：`nvidia`、`nouveau`、`amdgpu`、`radeon`、`i915`、`xe`、`vfio-pci`。
- 不属于已知 BMC/管理显卡厂商，例如 ASPEED `0x1a03`、Matrox `0x102b`。

如果 GPU 存在但没有绑定上述驱动，collector 会跳过该设备。

## 导出指标

### `node_gpu_info`

每张 GPU 一条 info 指标，值固定为 `1`。

```text
node_gpu_info{
  gpu_id="0000:65:00.0",
  vendor="NVIDIA Corporation",
  model="NVIDIA Tesla T4",
  vendor_id="0x10de",
  device_id="0x1eb8",
  uuid="GPU-01234567-89ab-cdef-0123-456789abcdef"
} 1
```

标签说明：

- `gpu_id`：PCI bus ID，对应 `/sys/bus/pci/devices/<gpu_id>`。
- `vendor`：厂商名称，优先来自 `pci.ids`。
- `model`：GPU 型号，优先来自 `pci.ids`。
- `vendor_id`：原始 PCI vendor ID。
- `device_id`：原始 PCI device ID。
- `uuid`：NVIDIA 原生驱动提供的物理 GPU UUID；不可用或不支持时为空字符串。保留 `GPU-` 前缀及原始十六进制大小写，不以 PCI 地址、型号或随机值代替。

### `node_gpu_cards_total`

按型号聚合后的 GPU 数量。

```text
node_gpu_cards_total{model="NVIDIA Tesla T4"} 2
```

## UUID 采集与限制

仅对 vendor ID 为 NVIDIA `0x10de` 且绑定 `nvidia` 驱动的设备读取 UUID，路径为：

```text
<path.procfs>/driver/nvidia/gpus/<gpu_id>/information
```

默认 `path.procfs` 是 `/proc`，容器示例中则是 `/host/proc`。collector 按完整 PCI 地址关联设备，读取 `GPU UUID:` 字段，不依赖 `nvidia-smi`、NVML 或 CUDA 用户态库，也不会主动初始化 GPU。

UUID 处理规则：

- 文件或字段缺失时，仍导出该 GPU，使用 `uuid=""`。NVIDIA 驱动尚未提供 UUID 时也适用。
- 字段值必须是 `GPU-` 加 `8-4-4-4-12` 位十六进制文本，允许首尾空白，不限制 RFC UUID 的版本和变体位。
- 读取失败、空字段、格式异常、重复字段或信息文件超过 64 KiB 时，使用 `uuid=""`，并记录包含 PCI 地址和错误原因的 Debug 日志，不输出文件原文。
- AMD、Intel，以及绑定 `nouveau`、`vfio-pci` 等其他驱动的设备，仍按既有规则导出，UUID 为空。不将 AMD `unique_id` 当作 UUID，不枚举 MIG 实例。
- UUID 是否可用不影响 `node_gpu_cards_total`，也不决定 `node_scrape_collector_success{collector="gpu"}` 是否为 `1`。

每次采集都会重新读取 UUID，不缓存历史值。驱动信息暂时消失时 UUID 会变为空，恢复后重新读取当前值，避免设备更换后沿用旧 UUID。文件读取是同步的，读取量上限不等于超时保证；目标驱动异常时可能延长采集耗时，应在部署环境验证。

升级后，取得非空 UUID 的 `node_gpu_info` 会成为新的时间序列；后续 UUID 在非空值与空值之间切换同样会改变时序身份。Prometheus 将空标签值视为标签不存在。稳定情况下仍是每张 GPU 一条 info 指标，原有标签及数量统计不变；依赖完整标签集匹配的查询需要检查，按设备关联时可显式使用 `on(instance, gpu_id)`，并确认该组合在查询范围内唯一。

## GPU 型号解析规则

型号解析按以下顺序进行：

1. 优先读取 `pci.ids`，使用 PCI vendor/device ID 查询厂商和型号。
2. 如果 `pci.ids` 没有匹配，NVIDIA 设备回退到内置常见型号表。
3. 如果仍无法匹配，`model` 使用原始 `device_id`，例如 `0xffff`。

默认会按 rootfs 解析以下路径：

```text
/usr/share/misc/pci.ids
/usr/share/hwdata/pci.ids
/var/lib/pciutils/pci.ids
```

如果需要指定自定义 `pci.ids` 文件，可以使用已有参数：

```bash
node_exporter --collector.pcidevice.idsfile=/path/to/pci.ids
```

该参数同时影响 `pcidevice` 和 `gpu` 的名称解析。自定义路径按 `node_exporter` 进程可见的路径读取，不会再自动拼接 `--path.rootfs`。

### 自定义 `pci.ids` 文件示例

`pci.ids` 中 vendor 行不缩进，device 行需要使用一个 tab 缩进。下面是一个最小示例：

完整的 NVIDIA fallback 示例可参考 [examples/pci.ids](../examples/pci.ids)。

```text
10de  NVIDIA Corporation
	1eb8  TU104GL [Tesla T4]
	2330  GH100 [H100 PCIe]
1002  Advanced Micro Devices, Inc. [AMD/ATI]
	744c  Navi 31 [Radeon RX 7900 XTX]
8086  Intel Corporation
	56a0  DG2 [Arc A770 Graphics]
```

如果将上述内容保存为 `/etc/node_exporter/pci.ids`，可以这样启动：

```bash
node_exporter --collector.pcidevice.idsfile=/etc/node_exporter/pci.ids
```

当 metrics 中出现：

```text
vendor_id="0x10de", device_id="0x1eb8"
```

`gpu` collector 会把它解析为：

```text
vendor="NVIDIA Corporation", model="TU104GL [Tesla T4]"
```

## 查询示例

查看当前导出的 GPU 指标：

```bash
curl -s http://127.0.0.1:9100/metrics | grep '^node_gpu_'
```

PromQL 查询所有 GPU：

```promql
node_gpu_info
```

按实例和型号统计 GPU 数量：

```promql
sum by (instance, model) (node_gpu_cards_total)
```

按实例、厂商和型号统计 GPU 数量：

```promql
count by (instance, vendor, model) (node_gpu_info)
```

查询某个 PCI bus ID 对应的 GPU：

```promql
node_gpu_info{gpu_id="0000:65:00.0"}
```

查询已成功取得 UUID 的 GPU：

```promql
node_gpu_info{uuid!=""}
```

## 如何查询 GPU 对应关系

先从 metrics 中拿到 `gpu_id`、`vendor_id`、`device_id`：

```bash
curl -s http://127.0.0.1:9100/metrics | grep '^node_gpu_info'
```

再在主机上用 sysfs 查询同一个设备：

```bash
gpu_id=0000:65:00.0
cat /sys/bus/pci/devices/${gpu_id}/vendor
cat /sys/bus/pci/devices/${gpu_id}/device
cat /sys/bus/pci/devices/${gpu_id}/class
readlink -f /sys/bus/pci/devices/${gpu_id}/driver
```

也可以用 `lspci` 查询：

```bash
lspci -Dnn -s 0000:65:00.0
```

示例输出：

```text
0000:65:00.0 3D controller [0302]: NVIDIA Corporation TU104GL [Tesla T4] [10de:1eb8]
```

其中 `[10de:1eb8]` 对应 metrics 中的：

```text
vendor_id="0x10de", device_id="0x1eb8"
```

如果需要直接查看 `pci.ids` 中的名称，可以搜索 vendor 和 device：

```bash
grep -i '^10de' /usr/share/misc/pci.ids
grep -i '1eb8' /usr/share/misc/pci.ids
```

不同发行版的 `pci.ids` 路径可能不同，可先检查：

```bash
ls /usr/share/misc/pci.ids /usr/share/hwdata/pci.ids /var/lib/pciutils/pci.ids
```

## 排查无 GPU 指标

如果没有 `node_gpu_*` 指标，可以按顺序检查：

1. collector 是否启用：

```promql
node_scrape_collector_success{collector="gpu"}
```

2. host sysfs 是否可见：

```bash
ls /sys/bus/pci/devices
```

3. 设备是否是 display controller：

```bash
cat /sys/bus/pci/devices/<gpu_id>/class
```

4. vendor 是否在支持范围内：

```bash
cat /sys/bus/pci/devices/<gpu_id>/vendor
```

5. 是否绑定支持的驱动：

```bash
readlink -f /sys/bus/pci/devices/<gpu_id>/driver
```

如果驱动 symlink 不存在，或驱动不是 `nvidia`、`nouveau`、`amdgpu`、`radeon`、`i915`、`xe`、`vfio-pci`，该设备不会导出为 GPU 指标。

如果设备指标存在但 UUID 为空，先确认该设备绑定的是 `nvidia` 驱动，再以 exporter 实际运行用户检查对应 `information` 文件是否可读、是否包含有效的 `GPU UUID:`。容器中应使用 `--path.procfs` 指向的路径；同时确认 procfs 与 sysfs 来自同一宿主机。文件或字段缺失不等于 GPU 不存在，也不应通过重新生成 UUID 来补齐。
