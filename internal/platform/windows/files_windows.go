package windows

import (
	"fmt"
	"strings"

	win "golang.org/x/sys/windows"
)

// PinBenchmarkPath holds a normal file or directory open without delete sharing.
// This prevents renames while DiskSpd opens the same file for reading or writing.
func PinBenchmarkPath(path, volumeGUID string, directory bool) (win.Handle, error) {
	flags := uint32(win.FILE_FLAG_OPEN_REPARSE_POINT)
	if directory {
		flags |= win.FILE_FLAG_BACKUP_SEMANTICS
	} else {
		// Keep the guard unbuffered like DiskSpd. A cached handle held open
		// after file preparation can throttle DiskSpd's queued I/O.
		flags |= win.FILE_FLAG_NO_BUFFERING
	}
	h, err := open(path, win.FILE_READ_DATA, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE, flags)
	if err != nil {
		return 0, err
	}
	if err := verifyPinnedPath(h, volumeGUID, directory); err != nil {
		win.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

func verifyPinnedPath(h win.Handle, volumeGUID string, directory bool) error {
	var info win.ByHandleFileInformation
	if err := win.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&win.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(info.FileAttributes&win.FILE_ATTRIBUTE_DIRECTORY != 0) != directory ||
		(!directory && info.NumberOfLinks != 1) {
		return fmt.Errorf("测速目标类型不符合要求")
	}
	const volumeNameGUID = 1
	var resolved [1024]uint16
	n, err := win.GetFinalPathNameByHandle(h, &resolved[0], uint32(len(resolved)), volumeNameGUID)
	if err != nil {
		return err
	}
	if n >= uint32(len(resolved)) || !strings.HasPrefix(strings.ToUpper(win.UTF16ToString(resolved[:n])), strings.ToUpper(volumeGUID)) {
		return fmt.Errorf("测速路径所在卷与所选卷不匹配")
	}
	return nil
}
