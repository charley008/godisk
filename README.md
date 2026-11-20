# goDisk

goDisk — Disk Health & Performance Monitor

goDisk 源于把两件常用的事放进同一个 Windows 窗口的需求。一边查看磁盘的温度、健康状态和 SMART 属性，一边测顺序与随机读写，还能回看以往的成绩。项目结合了 CrystalDiskInfo 的健康检测和 CrystalDiskMark 的性能测试，用 Go 和 MyGo Native UI 实现。

## 项目参考

- **CrystalDiskInfo** 启发了健康页的信息布局。磁盘型号、接口、温度和健康状态集中显示，下方保留 SMART 原始值，方便核对读数。 [CrystalDiskInfo](https://github.com/hiyohiyo/CrystalDiskInfo)
- **CrystalDiskMark** 启发了性能页的测试组织。按块大小、队列深度和线程数区分顺序与随机负载，把读写结果并排展示。 [CrystalDiskMark](https://github.com/hiyohiyo/CrystalDiskMark)
- **DiskLens** 提供了健康检测的架构参考。设备采集、原始 SMART 数据解析和健康报告分层，便于用保存的 NVMe 健康快照复核结果。 [DiskLens](https://github.com/PabloSoage/disklens)
- **Microsoft DiskSpd** 是实际执行测试的引擎。goDisk 负责准备测试文件、管理任务和读取 XML 结果。 [Microsoft DiskSpd](https://github.com/microsoft/diskspd)

[下载 Windows x64 便携版](https://github.com/charley008/godisk/releases/latest)
