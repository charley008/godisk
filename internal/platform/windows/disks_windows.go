package windows

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"unsafe"

	"goDisk/internal/domain"
	"goDisk/internal/protocol"
	win "golang.org/x/sys/windows"
)

const (
	queryStorageProperty = 0x002d1400
	getDeviceNumber      = 0x002d1080
	getDiskGeometry      = 0x000700a0
	getVolumeExtents     = 0x00560000
	shareDisk            = win.FILE_SHARE_READ | win.FILE_SHARE_WRITE | win.FILE_SHARE_DELETE
)

var (
	diskInterface = win.GUID{Data1: 0x53f56307, Data2: 0xb6bf, Data3: 0x11d0, Data4: [8]byte{0x94, 0xf2, 0, 0xa0, 0xc9, 0x1e, 0xfb, 0x8b}}
	setup         = win.NewLazySystemDLL("setupapi.dll")
	enumInterface = setup.NewProc("SetupDiEnumDeviceInterfaces")
	getDetail     = setup.NewProc("SetupDiGetDeviceInterfaceDetailW")
)

type interfaceData struct {
	Size     uint32
	Class    win.GUID
	Flags    uint32
	Reserved uintptr
}

func Scan(ctx context.Context) ([]domain.Disk, []domain.Volume, error) {
	list, err := win.SetupDiGetClassDevsEx(&diskInterface, "", 0, win.DIGCF_PRESENT|win.DIGCF_DEVICEINTERFACE, 0, "")
	if err != nil {
		return nil, nil, fmt.Errorf("枚举磁盘接口: %w", err)
	}
	defer win.SetupDiDestroyDeviceInfoList(list)
	var disks []domain.Disk
	for index := uint32(0); ; index++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		row := interfaceData{Size: uint32(unsafe.Sizeof(interfaceData{}))}
		ret, _, err := enumInterface.Call(uintptr(list), 0, uintptr(unsafe.Pointer(&diskInterface)), uintptr(index), uintptr(unsafe.Pointer(&row)))
		if ret == 0 {
			if errors.Is(err, win.ERROR_NO_MORE_ITEMS) {
				break
			}
			return nil, nil, fmt.Errorf("读取磁盘接口 %d: %w", index, err)
		}
		path, instance, err := interfacePath(list, &row)
		if err != nil {
			return nil, nil, err
		}
		disks = append(disks, describe(path, instance))
	}
	counts := make(map[string]int)
	for _, disk := range disks {
		counts[disk.ID]++
	}
	for i := range disks {
		if counts[disks[i].ID] > 1 {
			disks[i].ID = identity(disks[i].Path)
		}
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Number < disks[j].Number })
	volumes, err := volumesFor(ctx, disks)
	return disks, volumes, err
}

func interfacePath(list win.DevInfo, row *interfaceData) (string, uint32, error) {
	var size uint32
	_, _, err := getDetail.Call(uintptr(list), uintptr(unsafe.Pointer(row)), 0, 0, uintptr(unsafe.Pointer(&size)), 0)
	if !errors.Is(err, win.ERROR_INSUFFICIENT_BUFFER) || size < 8 || size > 65536 || size%2 != 0 {
		return "", 0, fmt.Errorf("取得磁盘接口路径长度 %d: %w", size, err)
	}
	buffer := make([]uint16, size/2)
	buffer[0] = 8 // SP_DEVICE_INTERFACE_DETAIL_DATA_W.cbSize on AMD64.
	var dev struct {
		Size     uint32
		Class    win.GUID
		Instance uint32
		Reserved uintptr
	}
	dev.Size = uint32(unsafe.Sizeof(dev))
	ret, _, err := getDetail.Call(uintptr(list), uintptr(unsafe.Pointer(row)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), 0, uintptr(unsafe.Pointer(&dev)))
	if ret == 0 {
		return "", 0, fmt.Errorf("读取磁盘接口路径: %w", err)
	}
	return win.UTF16ToString(buffer[2:]), dev.Instance, nil
}

func open(path string, access, share, flags uint32) (win.Handle, error) {
	p, err := win.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return win.CreateFile(p, access, share, nil, win.OPEN_EXISTING, flags, 0)
}

func query(h win.Handle, ioctl uint32, input, output []byte) (int, error) {
	var got uint32
	err := win.DeviceIoControl(h, ioctl, unsafe.SliceData(input), uint32(len(input)), unsafe.SliceData(output), uint32(len(output)), &got, nil)
	return int(got), err
}

func describe(path string, instance uint32) domain.Disk {
	disk := domain.Disk{Path: path, Number: -1, Model: "未知型号", Bus: "未知", ID: identity(path)}
	h, err := open(path, 0, shareDisk, 0)
	if err != nil {
		disk.Issue = err.Error()
		return disk
	}
	defer win.CloseHandle(h)
	var number [12]byte
	if n, e := query(h, getDeviceNumber, nil, number[:]); e == nil && n == len(number) {
		disk.Number = int(binary.LittleEndian.Uint32(number[4:]))
	} else {
		disk.Issue = fmt.Sprintf("未取得磁盘编号 (%v)", e)
	}
	var geometry [256]byte
	if n, e := query(h, getDiskGeometry, nil, geometry[:]); e == nil && n >= 32 {
		disk.Capacity = binary.LittleEndian.Uint64(geometry[24:])
	}
	var property [12]byte
	var descriptor [4096]byte
	n, err := query(h, queryStorageProperty, property[:], descriptor[:])
	if err != nil || n < 36 || n > len(descriptor) {
		disk.Issue = fmt.Sprintf("设备描述读取失败 (%v)", err)
		return disk
	}
	buf := descriptor[:n]
	vendor := descriptorText(buf, 12)
	if product := strings.TrimSpace(vendor + " " + descriptorText(buf, 16)); product != "" {
		disk.Model = product
	}
	disk.Firmware = descriptorText(buf, 20)
	disk.Bus = busName(binary.LittleEndian.Uint32(buf[28:]))
	switch disk.Bus {
	case "NVMe":
		disk.RotationRate = 1
		if identify, err := protocolPage(h, 49, 3, 1, 1, 4096); err == nil {
			disk.Details = protocol.NVMeIdentifyController(identify)
		}
		disk.Details.CurrentLink, disk.Details.MaxLink = pciTransferMode(instance)
	case "ATA", "SATA":
		if identify, err := protocolPage(h, 50, 2, 1, 0, 512); err == nil {
			disk.RotationRate = protocol.ATARotationRate(identify)
			disk.Details = protocol.ATAIdentifyDevice(identify)
		}
	}
	if serial := descriptorText(buf, 24); serial != "" {
		disk.ID = identity(strings.Join([]string{disk.Bus, disk.Model, serial, fmt.Sprint(disk.Capacity)}, "\x00"))
		if disk.Details.Serial == "" {
			disk.Details.Serial = serial
		}
	}
	return disk
}

func identity(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToUpper(value))))
}

func descriptorText(buf []byte, index int) string {
	start := int(binary.LittleEndian.Uint32(buf[index:]))
	if start < 36 || start >= len(buf) {
		return ""
	}
	value, _, ok := bytes.Cut(buf[start:], []byte{0})
	if !ok {
		return ""
	}
	return strings.TrimSpace(strings.ToValidUTF8(string(value), "�"))
}

func busName(value uint32) string {
	names := map[uint32]string{1: "SCSI", 2: "ATAPI", 3: "ATA", 7: "USB", 8: "RAID", 10: "SAS", 11: "SATA", 14: "Virtual", 15: "File-backed Virtual", 16: "Storage Spaces", 17: "NVMe", 18: "SCM", 19: "UFS"}
	if name := names[value]; name != "" {
		return name
	}
	return fmt.Sprintf("未知 (%d)", value)
}

func volumesFor(ctx context.Context, disks []domain.Disk) ([]domain.Volume, error) {
	mask, err := win.GetLogicalDrives()
	if err != nil {
		return nil, err
	}
	var volumes []domain.Volume
	for letter := range 26 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if mask&(1<<letter) == 0 {
			continue
		}
		v, err := ProbeVolume(fmt.Sprintf("%c:\\", 'A'+letter))
		if err == nil {
			volumes = append(volumes, bind(v, disks))
		}
	}
	return volumes, nil
}

func bind(v domain.Volume, disks []domain.Disk) domain.Volume {
	for _, number := range v.Disks {
		for _, disk := range disks {
			if disk.Number == number {
				v.Identities = append(v.Identities, disk.ID)
				break
			}
		}
	}
	return v
}

func CheckVolume(ctx context.Context, expected domain.Volume) (domain.Volume, error) {
	disks, volumes, err := Scan(ctx)
	if err != nil {
		return domain.Volume{}, err
	}
	for _, volume := range volumes {
		if strings.EqualFold(volume.Root, expected.Root) && strings.EqualFold(volume.GUID, expected.GUID) &&
			slices.Equal(volume.Disks, expected.Disks) && slices.Equal(volume.Identities, expected.Identities) &&
			len(diskSubset(disks, volume.Disks)) == len(volume.Disks) && len(volume.Disks) == 1 {
			return volume, nil
		}
	}
	return domain.Volume{}, errors.New("卷身份或物理磁盘映射已改变，请刷新设备并重新选择")
}

func diskSubset(disks []domain.Disk, numbers []int) []domain.Disk {
	var selected []domain.Disk
	for _, disk := range disks {
		if slices.Contains(numbers, disk.Number) {
			selected = append(selected, disk)
		}
	}
	return selected
}

func ProbeVolume(root string) (domain.Volume, error) {
	if len(root) != 3 || root[0] < 'A' || root[0] > 'Z' || root[1:] != ":\\" {
		return domain.Volume{}, errors.New("仅接受本地固定卷的盘符")
	}
	p, _ := win.UTF16PtrFromString(root)
	if win.GetDriveType(p) != win.DRIVE_FIXED {
		return domain.Volume{}, errors.New("目标不是本地固定卷")
	}
	var guid, label [260]uint16
	if err := win.GetVolumeNameForVolumeMountPoint(p, &guid[0], uint32(len(guid))); err != nil {
		return domain.Volume{}, err
	}
	v := domain.Volume{Root: root, GUID: win.UTF16ToString(guid[:]), System: strings.EqualFold(root[:2], os.Getenv("SystemDrive"))}
	_ = win.GetVolumeInformation(p, &label[0], uint32(len(label)), nil, nil, nil, nil, 0)
	v.Label = win.UTF16ToString(label[:])
	if err := win.GetDiskFreeSpaceEx(p, &v.Free, nil, nil); err != nil {
		return domain.Volume{}, err
	}
	h, err := open(strings.TrimSuffix(v.GUID, "\\"), 0, shareDisk, 0)
	if err != nil {
		return domain.Volume{}, err
	}
	defer win.CloseHandle(h)
	out := make([]byte, 8+128*24)
	n, err := query(h, getVolumeExtents, nil, out)
	if err != nil {
		return domain.Volume{}, err
	}
	count := int(binary.LittleEndian.Uint32(out))
	if n < 8 || n > len(out) || count == 0 || count > (n-8)/24 {
		return domain.Volume{}, fmt.Errorf("卷 extent 长度无效 (%d)", n)
	}
	for i := range count {
		v.Disks = append(v.Disks, int(binary.LittleEndian.Uint32(out[8+i*24:])))
	}
	slices.Sort(v.Disks)
	v.Disks = slices.Compact(v.Disks)
	return v, nil
}
