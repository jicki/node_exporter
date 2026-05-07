# GPU Collector 使用说明

GPU collector 用于从 Linux PCI sysfs 中导出 GPU 设备清单。它关注设备是否存在、属于哪个厂商和型号，不读取显存、温度、利用率等运行时指标。

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

容器运行时需要确保 host 的 `/sys` 可见。常见方式是挂载 host 根目录并设置 rootfs：

```bash
docker run --rm \
  --net=host \
  --pid=host \
  -v /:/host:ro,rslave \
  node-exporter:latest \
  --path.rootfs=/host
```

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
  device_id="0x1eb8"
} 1
```

标签说明：

- `gpu_id`：PCI bus ID，对应 `/sys/bus/pci/devices/<gpu_id>`。
- `vendor`：厂商名称，优先来自 `pci.ids`。
- `model`：GPU 型号，优先来自 `pci.ids`。
- `vendor_id`：原始 PCI vendor ID。
- `device_id`：原始 PCI device ID。

### `node_gpu_cards_total`

按型号聚合后的 GPU 数量。

```text
node_gpu_cards_total{model="NVIDIA Tesla T4"} 2
```

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
