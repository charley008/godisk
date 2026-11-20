# goDisk — Disk Health & Performance Monitor

技术调研与可行性验证 · 2026-11-20 · Windows 11 x64

## 结论

Go + MyGo Native UI 可以支撑首期原型。Go 在本机以普通权限从三块 NVMe 读取了 Identify 与健康日志，并成功启动 DiskSpd、解析 XML 和取消任务。继续采用附件规定的 Windows 原生健康采集、MyGo Native UI、独立 DiskSpd 和 SQLite 历史记录。SATA 真机、屏幕阅读器和多显示器缩放需要独立验收。DiskSpd 发布包的 EULA 影响分发选择，建议先配置用户从微软官网下载的独立工具，待澄清再分发条件后再决定是否随包交付。

| 项目 | 证据 | 本阶段结论 |
| --- | --- | --- |
| Go 工具链 | `go version`、`go env`、实际构建 | Go 1.27.4，`windows/amd64`，`CGO_ENABLED=0`，满足固定版本 MyGo 的要求 |
| MyGo 原生窗口 | Windows 本机窗口、绘制回调、PE 资源和外部文件 | 可运行，基础控件可构建，交互和无障碍仍需按下文验收 |
| NVMe | 普通令牌、3 块设备、真实 IOCTL 响应 | Identify Controller、Identify Namespace、SMART/Health 和首批 Error Log 字节均可读取 |
| SATA | Microsoft WDK 与参考源码 | API 和读指令方案明确，本机无 SATA，真机通路与权限待测 |
| DiskSpd | 官方 v2.3 x64，Go 启动和解析，测试文件读负载 | 正常退出、测量阶段取消、预热取消、强制终止和非法参数均有实测 |

项目工作区在调研开始时为空，且没有 Git 元数据。面向本机的短原型、上游检出和测试输出放在 `%TEMP%\goDisk-stage1-20261120`。测速探针只在其临时目录生成一个 16 MiB 文件，执行短时缓存读，运行后清理测试文件。没有保存真实设备的原始 SMART dump 或序列号。

## 本机环境与 MyGo

- Windows 11 26H2，构建 26300.9974，当前进程未提升。Windows Storage 枚举报告三块 NVMe，`Win32_DiskDrive.InterfaceType` 对它们均返回 `SCSI`，因此该 WMI 字段不适合作为协议选择依据。
- Go 安装在 `C:\Program Files\Go`，`GOMOD=NUL`，`GOPATH=%USERPROFILE%\go`，`GOBIN=%USERPROFILE%\go\bin`。MyGo 无全局 CLI，Go module 查询返回 v0.10.0，固定 tag 的 `go.mod` 要求 Go 1.27.3。官方入门文档给出 Go 1.27+ 的一般要求。使用固定版本的临时 CLI 通过了 `doctor` 的 Go 检查，Bun 1.3.14 和 WebView2 已存在，NSIS、SignTool 未发现。Windows Native UI 便携构建不依赖 Bun、WebView2、NSIS 或 C 编译器。 [MyGo 入门](https://mygo.run/docs/getting-started) · [v0.10.0 go.mod](https://github.com/egoist/mygo/blob/v0.10.0/go.mod) · [构建与分发](https://mygo.run/docs/distribution)
- `go tool mygo build -platform windows/amd64 -skip-installer` 产出约 10.7 MiB 的 `goDisk.exe`。Go 构建信息显示 `CGO_ENABLED=0`。PE 资源表含 Icon、Group Icon、Version、Manifest。构建器将 `icon.png` 和测试文本复制到 EXE 同级，`App.Path(mygo.PathResources)` 实际解析到该目录。窗口调用 `SetIcon` 成功，`WM_GETICON` 返回图标句柄。当前打包使用临时图标，正式图标待设计。 [MyGo Windows 打包](https://mygo.run/docs/distribution#windows) · [MyGo 窗口 API](https://mygo.run/docs/windows)
- 使用 `WindowOptions.Content: ui.View(...)` 打开标题为 goDisk 的响应窗口。通过 `ui.Scroll`、`ui.Table`、`ui.Tabs`、`ui.Progress` 和 `Painter.Line` 搭建拟真页面。绘制回调在原生窗口触发，后台 goroutine 通过 `Window.Update` 把进度更新到 60%，窗口缩放到 805×520 后继续重绘，`GetDpiForWindow` 读到 192，即该显示器的 200%。当前版本提供 D3D11 渲染路径和 CPU 后备。运行中的进程快照显示已加载 `d3d11.dll`、`dxgi.dll`、`dwrite.dll`，未发现 WebView 模块或子进程，程序退出码为 0。WebView2 已安装，仍应在无 WebView2 的干净环境复测。 [MyGo Native UI](https://mygo.run/docs/ui) · [架构与渲染](https://mygo.run/docs/architecture) · [Windows 渲染实现](https://github.com/egoist/mygo/blob/v0.10.0/ui/gpu_windows.go)
- `ui.NewTester` 检查了磁盘列表、SMART 表格文字和磁盘条目的可访问性节点。初版布局的标签点击命中失败，给列表和表格设置明确尺寸后，`go test -v` 验证滚轮移动列表、鼠标切到性能页和方向键返回健康页。真实 Windows 输入、UI Automation/Narrator、多个 DPI 显示器间切换、中文长属性名、数百行表格的焦点和性能仍需实测。折线绘制证实底层能力，正式图表还需坐标轴、缺失样本断线和等效文字。 [MyGo UI 文档](https://mygo.run/docs/ui) · [上游图表示例](https://github.com/egoist/mygo/blob/v0.10.0/examples/gallery/charts.go)

## 参考源码

| 项目与快照 | 实现要点与对 goDisk 的影响 |
| --- | --- |
| DiskLens · `21cc6d4` | Rust 的 `device/windows.rs` 对 NVMe 走协议属性查询、对 ATA 走透传。采集页交给 `smart/{nvme,ata,vendor,health,derive}.rs` 转成 `DiskReport`。显式 Native/smartctl 模式与 Auto 回退保留来源。 `dump.rs` 支持离线分析，`tools` 有脱敏、对照与样本转换。借鉴原始数据到报告的分层及回归方式。其 Windows Namespace 查询带有固定编号假设，goDisk 要验证实际设备与命名空间关联。 [Windows 采集](https://github.com/PabloSoage/disklens/blob/21cc6d4/src/device/windows.rs) · [回退逻辑](https://github.com/PabloSoage/disklens/blob/21cc6d4/src/scan.rs) · [解析](https://github.com/PabloSoage/disklens/tree/21cc6d4/src/smart) · [存档](https://github.com/PabloSoage/disklens/blob/21cc6d4/src/dump.rs) · [对照工具](https://github.com/PabloSoage/disklens/tree/21cc6d4/tools) |
| CrystalDiskInfo · `cabe686` | C++/MFC。 `AtaSmart.cpp` 的 `UpdateSmartInfo` 根据命令类型刷新设备，含标准 NVMe 属性查询、传统 ATA、SAT、USB 桥与 RAID 等分支；`CheckDiskStatus` 及厂商和计量规则解释温度、读写量、通电与健康状态。将其作为同设备对照，优先实现标准字段，逐型号复核私有单位和阈值。 [AtaSmart.cpp](https://github.com/hiyohiyo/CrystalDiskInfo/blob/cabe686/AtaSmart.cpp) · [AtaSmart.h](https://github.com/hiyohiyo/CrystalDiskInfo/blob/cabe686/AtaSmart.h) |
| CrystalDiskMark · `0ceaa7a` | C++/MFC。 `DiskBench.cpp` 准备、填充和清理测试文件，按块大小、队列、线程、模式、次数和间隔编排 DiskSpd。该版本用 `-A<PID>` 配合定制工具，通过退出值计算吞吐、共享内存取得延迟，停止操作调用 `TerminateProcess`。goDisk 使用微软版 XML 合同，参数和聚合策略明确记录，结果比较要求匹配文件内容、缓存、测量时长和重复统计方法。 [DiskBench.cpp](https://github.com/hiyohiyo/CrystalDiskMark/blob/0ceaa7a/DiskBench.cpp) · [取消实现](https://github.com/hiyohiyo/CrystalDiskMark/blob/0ceaa7a/DiskMarkDlg.cpp) |
| Microsoft DiskSpd · `v2.3` / `5e7025b` | `CmdLineParser` 解析 profile、CLI 和同步事件，`IORequestGenerator` 编排 Windows I/O 及预热、测量、冷却，`XmlResultParser` 写系统、配置、计数和延迟。2.3 版本改变了部分默认亲和与缓冲区策略，测速记录应固化工具版本和实际参数。 [发布](https://github.com/microsoft/diskspd/releases/tag/v2.3) · [CLI 源码](https://github.com/microsoft/diskspd/tree/v2.3/CmdLineParser) · [I/O 源码](https://github.com/microsoft/diskspd/tree/v2.3/IORequestGenerator) · [XML 源码](https://github.com/microsoft/diskspd/tree/v2.3/XmlResultParser) |

## Windows 设备读取方案

1. 用 SetupAPI 枚举存在的 `GUID_DEVINTERFACE_DISK` 接口，`IOCTL_STORAGE_GET_DEVICE_NUMBER` 和 `IOCTL_STORAGE_QUERY_PROPERTY` 获取当前编号、设备描述和 BusType，容量用设备长度查询。用卷 GUID、挂载点和 `IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS` 建立卷到物理设备的关系。跨多个物理磁盘的卷必须保留所有 extent。设备接口和编号只用于当前访问。 [SetupAPI](https://learn.microsoft.com/en-us/windows/win32/api/setupapi/nf-setupapi-setupdigetclassdevsw) · [卷 extent](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntddvol/ni-ntddvol-ioctl_volume_get_volume_disk_extents)
2. NVMe 先用 `CreateFileW` 的 `dwDesiredAccess=0`、读写共享打开设备；调用 `IOCTL_STORAGE_QUERY_PROPERTY`。Controller 用 `StorageAdapterProtocolSpecificProperty` + Identify CNS Controller，Namespace 用 `StorageDeviceProtocolSpecificProperty` + Identify CNS Namespace 并验证对应关系，Health 使用 Log Page 0x02（512 B），Error Log 0x01 先验证首批字节，后续按能力决定是否解析更多。输入的协议结构从 query 的 `AdditionalParameters` 开始，返回的 `ProtocolDataOffset` 相对于协议结构。校验描述符版本、大小、返回总长、偏移下界和载荷范围后才解码。实际探针用已知的 `PhysicalDrive0–2`，生产 SetupAPI 枚举和动态卷映射仍待实现。 [Microsoft NVMe 查询说明](https://learn.microsoft.com/en-us/windows/win32/fileio/working-with-nvme-devices)
3. NVMe 保留 `Critical Warning` 位、`Available Spare` 与阈值、`Percentage Used` 原值（允许超过 100）、复合温度与独立传感器，按规范处理未提供的温度。用无符号 128 位无损表示累计计数，Data Units Read/Written 按每单位 1,000 × 512 字节换算并保留原值。健康判断留存依据及规则版本。Windows 样本在三盘均得到 4096 B Controller、4096 B Namespace、512 B Device Health、512 B Adapter Health 和 512 B Adapter Error Log。复合温度 317–322 K、Spare 均 100、Percentage Used 为 1/2/6。还需解析 Namespace 容量与 Error Log 内容并做 CrystalDiskInfo 同时点对照。 [NVMe 规范](https://nvmexpress.org/specifications/) · [Microsoft NVMe 字段定义](https://learn.microsoft.com/en-us/windows/win32/api/nvme/ns-nvme-nvme_health_info_log)
4. SATA 先面向直连 AHCI，按 WDK 的 `ATA_PASS_THROUGH_EX` 组织同步、缓冲式 `IOCTL_ATA_PASS_THROUGH`。依次研究 IDENTIFY DEVICE（`0xEC`）、SMART READ DATA（`0xB0/0xD0`）、READ THRESHOLDS（`0xB0/0xD1`）和 RETURN STATUS（`0xB0/0xDA`，无数据传输，检查返回寄存器）。只允许固定只读指令。以 WDK 编译的小型 C 布局检查交叉确认 AMD64 的 48 字节头、`ULONG_PTR`、任务文件和数据缓冲偏移，再检查返回长度、ATA Status/Error、可用校验和及阈值 ID 配对。微软说明调用需要设备读写访问权，实际能否打开取决于设备和驱动 ACL，按操作提示权限。每页独立标记成功、缺失或失败。 [ATA IOCTL](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntddscsi/ni-ntddscsi-ioctl_ata_pass_through) · [ATA 结构](https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/ntddscsi/ns-ntddscsi-_ata_pass_through_ex)
5. ATA 原始属性、归一化 current/worst、threshold 和厂商解释分别存储。SATA SSD 寿命和 SATA HDD 温度、通电、扇区规则按可识别型号解释。USB SAT、厂商私有 NVMe 桥、VMD/RST、RAID 和 Storage Spaces 单独验证，失败时显示原因。 `context` 用于排队和调用间取消；同步 ATA 在途调用需验证驱动超时和取消能力，必要时评估单次隔离 helper。 [CancelSynchronousIo](https://learn.microsoft.com/en-us/windows/win32/fileio/cancelsynchronousio-func)

## DiskSpd 调用、输出与取消

测试了官方 v2.3 下载包的 `amd64/diskspd.exe`，工具帮助和 XML 报 2.3.0。EXE SHA-256 为 `DD4E57E1E8CCAF5D6437938F8AAB7F17E9A1E6D8FBA8A093006B7CADF16FAEA2`。用 Go `os/exec` 逐参数传递绝对路径和含空格的测试目标，基准配置为 `-Rxml -L -b4K -t1 -o1 -w0 -W0 -C0 -d1`。这轮只证明流程，磁盘性能数值受缓存主导。 [官方包](https://github.com/microsoft/diskspd/releases/download/v2.3/DiskSpd.zip) · [参数](https://github.com/microsoft/diskspd/wiki/Command-line-and-parameters)

| 场景 | 实测现象 |
| --- | --- |
| 正常 1 秒读 | 退出码 0。XML 测量时长 1.010 秒，成功解析版本、字节数、IOCount，有延迟节点 |
| 测量中取消 | 20 秒任务在约 1.3 秒触发预先创建的 `Local\...` 手动重置事件，传入 `-yp<event>`，调用 `SetEvent`。约 1.33 秒回收，退出码 0，有 1.300 秒部分数据 |
| 预热取消 | 10 秒预热在约 1.3 秒取消，退出码 0，有 XML，测量时长和 I/O 均为 0，无延迟节点 |
| 强制终止 | Go `Process.Kill` 后 `Wait` 返回，实测退出码 1，无 XML |
| 无效参数 | 实测退出码 3，stdout 空，stderr 有 invalid option |

- 指标采用 `encoding/xml` 读取 `Results/System/Tool/Version` 和 `TimeSpan/TestTimeSeconds`，汇总 `Thread/Target` 的 `BytesCount`、`ReadBytes`、`WriteBytes` 和 `IOCount`。以实际测量秒数做分母，B/s 同时支持十进制 MB/s 与二进制 MiB/s。延迟优先读取 `TimeSpan/Latency` 的整体均值和 Bucket 分位统计，保留毫秒基准。禁止跨线程直接平均分位数。非正测量时长、缺失字段、版本不支持和截断 XML 都需要显式状态。 [结果格式](https://github.com/microsoft/diskspd/wiki/Analyzing-test-results) · [XML 输出实现](https://github.com/microsoft/diskspd/blob/v2.3/XmlResultParser/XmlResultParser.cpp)
- 进度用阶段与测量时间推算；评估 `-ys`（测量开始）和 `-yf`（测量结束）事件。取消优先由父进程置位 `-yp` 事件，必要时限时 Kill/Wait。生产运行器应持续读取并限制 stdout/stderr，记录退出码及取消来源，支持强杀后回收，考虑 Windows Job Object 管理进程树和父进程崩溃。Job Object 归属竞态、超时、终止后文件释放仍需验证。 [同步参数](https://github.com/microsoft/diskspd/wiki/Command-line-and-parameters#synchronization-parameters) · [Go os/exec](https://pkg.go.dev/os/exec)
- 正式负载只用 goDisk 独占创建的普通文件。每项记录 `-b`、`-r`/顺序策略、`-o` 每线程每目标 outstanding、`-t`、`-w`、`-W/-d/-C`、缓存和数据模式。基线候选为顺序 1 MiB 与随机 4 KiB 的独立读写，默认参数和写入预算待专门验证。含写负载需由用户明确选择目标卷后开始；本轮没有测写入 profile 或文件清理故障注入。 [命令参数](https://github.com/microsoft/diskspd/wiki/Command-line-and-parameters)

## 许可证与风险

- DiskLens 与 MyGo 固定版本的仓库许可证为 MIT，DiskLens 的部分健康判断注明源自 CrystalDiskInfo。CrystalDiskInfo 根许可证为 MIT，资源和具体文件需逐项核对。CrystalDiskMark 该快照的 `DiskBench.cpp` 声明 MIT，仓库根没有许可证文件，来源码和资源在任何复用前逐项确认。本阶段只作参考。 [DiskLens LICENSE](https://github.com/PabloSoage/disklens/blob/21cc6d4/LICENSE) · [MyGo LICENSE](https://github.com/egoist/mygo/blob/v0.10.0/LICENSE) · [CrystalDiskInfo LICENSE](https://github.com/hiyohiyo/CrystalDiskInfo/blob/cabe686/LICENSE.txt) · [CrystalDiskMark 源码声明](https://github.com/hiyohiyo/CrystalDiskMark/blob/0ceaa7a/DiskBench.cpp)
- DiskSpd v2.3 源码仓库有 MIT 文件，所下载的 ZIP 另含 `EULA`，第 2 节限制发布和转让。源码许可证与该二进制包条款需要分别核查。建议开发阶段要求用户显式配置官方工具路径，分发阶段核查源码构建产物的条款和通知义务，或取得对应二进制授权后再决定随包。 [DiskSpd 源码 LICENSE](https://github.com/microsoft/diskspd/blob/v2.3/LICENSE) · [含 EULA 的官方二进制包](https://github.com/microsoft/diskspd/releases/download/v2.3/DiskSpd.zip)

| 优先级 | 风险 | 验收或缓解措施 |
| --- | --- | --- |
| 高 | SATA 透传、USB/VMD/RAID 与权限差异 | 直连 SATA SSD/HDD 分别用普通和提升权限验收；桥接与 RAID 单列设备矩阵 |
| 高 | 错盘写入、卷映射变化、异常退出遗留 | 仅普通测试文件，启动前重核卷身份与剩余空间，极限条件和清理失败用例 |
| 高 | 取消期间部分结果误当成功；同步驱动调用超时 | 显式终态，等待进程或设备线程清理，验证事件、超时和驱动挂起 |
| 高 | 厂商 SMART 数据误读及敏感信息外泄 | 原始页回放、脱敏样本、按设备并行对照 CrystalDiskInfo，未知字段保留未知 |
| 高 | DiskSpd 官方二进制再分发限制 | 默认可配置外置工具，确定许可证策略后才做正式安装包 |
| 中 | MyGo v0.x API、控件和可访问性变化 | 固定版本；Windows UIA/Narrator、键鼠和跨显示器 DPI 验收；UI 通过应用接口隔离 |
| 中 | SSD 缓存、温度、I/O 策略与温度轮询影响结果 | 保存参数和环境，比较同 profile；测量关/开采样差异 |
| 中 | SQLite 驱动体积、迁移及身份碰撞 | 评估无 CGO 的驱动、Windows 便携构建、迁移回滚及设备重新识别样本 |

后续开发顺序及各阶段验收见 [架构设计](ARCHITECTURE.md)。本阶段结论支持进入设备枚举，但正式开发以用户确认为起点。

## CrystalDiskMark 8.0.6 复查

对应正式 ZIP 含 `CdmResource/DiskSpd/DiskSpd64.exe`、其他架构组件和 `License/DiskSpd-LICENSE.txt`。版本源码包的 `CrystalDiskMark/DiskBench.cpp` 通过固定路径选择相应二进制，使用 `CreateProcess` 启动，常用配置为顺序 1 MiB Q8T1/Q1T1 和随机 4 KiB Q32T1/Q1T1。其 `diskspd2_0_21a/CmdRequestCreator/CmdRequestCreator.cpp` 有退出值及共享内存的特制通路，同时可输出 XML。goDisk 0.2.x 随包附带这份 MIT x64 组件，沿用 goDisk 的实际测量时长和 XML 指标，不依赖 CrystalDiskMark 的共享内存协议。 [8.0.6 官方源码及二进制档案](https://sourceforge.net/projects/crystaldiskmark/files/8.0.6/)

## 本机复现

临时目录中的 `nvme-probe.go` 可运行 `go run nvme-probe.go 0 1 2`。 `diskspd-probe.go` 可在同目录运行 `go run diskspd-probe.go`，使用解压在 `diskspd-bin\amd64` 的官方工具并生成三份 XML。XML 可能含本机路径和环境信息，分享前需脱敏。

`goDisk-ui` 存放独立 MyGo 原型。在该目录运行 `go test -v` 和 `go tool mygo build -platform windows/amd64 -skip-installer`。将 `GODISK_PROBE_LOG` 设置为可写的绝对日志路径后启动 `build\windows-amd64\goDisk.exe`，窗口约六秒后自行退出。
