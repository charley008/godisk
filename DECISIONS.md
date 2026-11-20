# goDisk 架构决定

## 0.1.0 测试版

### Windows 原生与应用层边界

Go 负责 SetupAPI 枚举、Windows IOCTL、SMART 解析和任务编排，MyGo v0.10.0 负责原生界面。保持域和解析层可独立测试。延后 Windows 桥接和厂商专用指令；采用单卷本地固定盘符、单个物理磁盘的测速范围。这样可以在本机验证普通权限和卷映射，厂商与复杂卷覆盖仍需实物扩充。

### DiskSpd v2.3 外置

使用微软官网下载包中的 x64 二进制，由用户配置路径。测试版固定已验证的 SHA-256，防止把其他构建或版本的 XML 当作相同结果。随包分发与自动下载暂缓，因为官方二进制包有单独的 EULA，且固定版本便于复现。更换工具版本需要重新对照参数和解析。

父进程先将 DiskSpd 分配到 Job Object，再放行 `-yr`。取消优先发 `-yp`，最后才强制回收。测试发现官方 v2.3 的 `-ys/-yf` 未触发，进度采用明确标注的时间估计，完成从实际 XML 时长和进程状态判断。

### 本地历史与原始快照

使用 `modernc.org/sqlite` 保持 Windows x64 构建无 CGO。SQLite 存储健康指标和测速配置、结果、温度样本。测试版使用带 schema 版本的 JSON 记录，先保证写入与回看，设备纵向查询可在后续 schema 迁移中增强。离线抓取导出采用带摘要的标准 NVMe 健康页，省去含硬件标识的 Identify 页。

## 0.2.x 测试版

### 内附测速组件

核对 CrystalDiskMark 8.0.6 的正式 ZIP、`DiskBench.cpp` 和同版源码包后，改为随 goDisk 提供该发布包内的 MIT 版 DiskSpd 2.0.21a x64。构建脚本只提取组件及许可并核对摘要，应用从固定资源路径调用。继续使用普通临时文件、Job Object、取消事件和 XML 解析。微软 v2.3 下载包的 EULA 与这个发布物分别处理。 [8.0.6 源码和发布包](https://sourceforge.net/projects/crystaldiskmark/files/8.0.6/)

### 压缩健康页

Windows 原生标题栏显示产品名称、所选磁盘与采集时间。文件、查看、测试和帮助使用原生菜单。健康页用三行摘要与固定表头的 SMART 数据表展示设备信息。NVMe 的 ID 用于表格对照，原始字段名和快照格式保持兼容。

## 0.3.0 测试版

### 使用 Microsoft DiskSpd 2.3 x64

按用户决定迁移到微软最新正式版。官方二进制的 EULA 限制转发，ZIP 提供 `setup-diskspd.ps1` 供用户设备直接从微软获取固定版本。脚本校验 ZIP 和 EXE 的 SHA-256 后安装到 goDisk 程序目录。保留固定组件路径、无缓冲文件保护、Job Object 和 XML 结果。测速详情显示实际引擎版本，便于对照历史。 [Microsoft DiskSpd 2.3 发布说明](https://github.com/microsoft/diskspd/releases/tag/v2.3)
