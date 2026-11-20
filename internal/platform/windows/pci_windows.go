package windows

import (
	"fmt"
	"strings"
	"unsafe"

	win "golang.org/x/sys/windows"
)

var (
	cfgmgr           = win.NewLazySystemDLL("cfgmgr32.dll")
	cmParent         = cfgmgr.NewProc("CM_Get_Parent")
	cmDeviceID       = cfgmgr.NewProc("CM_Get_Device_IDW")
	cmDeviceProperty = cfgmgr.NewProc("CM_Get_DevNode_PropertyW")
	pciProperties    = win.GUID{Data1: 0x3ab22e31, Data2: 0x8264, Data3: 0x4b4e, Data4: [8]byte{0x9a, 0xf5, 0xa8, 0xd2, 0xd8, 0xe3, 0x3e, 0x62}}
)

func pciTransferMode(instance uint32) (current, maximum string) {
	for range 16 {
		var id [512]uint16
		status, _, _ := cmDeviceID.Call(uintptr(instance), uintptr(unsafe.Pointer(&id[0])), uintptr(len(id)), 0)
		if status == 0 && strings.HasPrefix(win.UTF16ToString(id[:]), "PCI\\") {
			return pciLink(pciValue(instance, 9), pciValue(instance, 10)),
				pciLink(pciValue(instance, 11), pciValue(instance, 12))
		}
		var parent uint32
		status, _, _ = cmParent.Call(uintptr(unsafe.Pointer(&parent)), uintptr(instance), 0)
		if status != 0 || parent == instance {
			break
		}
		instance = parent
	}
	return "", ""
}

func pciValue(instance, property uint32) uint32 {
	key := struct {
		GUID win.GUID
		ID   uint32
	}{pciProperties, property}
	var value, valueType uint32
	size := uint32(unsafe.Sizeof(value))
	status, _, _ := cmDeviceProperty.Call(uintptr(instance), uintptr(unsafe.Pointer(&key)),
		uintptr(unsafe.Pointer(&valueType)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&size)), 0)
	if status != 0 {
		return 0
	}
	switch win.DEVPROPTYPE(valueType) {
	case win.DEVPROP_TYPE_BYTE:
		if size == 1 {
			return value & 0xff
		}
	case win.DEVPROP_TYPE_UINT16:
		if size == 2 {
			return value & 0xffff
		}
	case win.DEVPROP_TYPE_UINT32:
		if size == 4 {
			return value
		}
	}
	return 0
}

func pciLink(generation, lanes uint32) string {
	if generation < 1 || generation > 7 || lanes < 1 || lanes > 32 {
		return ""
	}
	return fmt.Sprintf("PCIe %d.0 x%d", generation, lanes)
}
