//go:build windows

package clipboard

import (
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Copying files in Explorer does not put their contents on the clipboard. It
// puts CF_HDROP: a handle to a list of paths, the same payload a drag-and-drop
// delivers. golang.design/x/clipboard reads only text and images, so this
// format is handled here directly against the Win32 API.
var (
	user32  = syscall.NewLazyDLL("user32.dll")
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procDragQueryFileW   = shell32.NewProc("DragQueryFileW")
)

const (
	// cfHDrop is the standard clipboard format for a list of file paths.
	cfHDrop = 15
	// queryFileCount asks DragQueryFile for the number of paths rather than
	// for one of them.
	queryFileCount = 0xFFFFFFFF

	// The clipboard is a single global resource and any other process may
	// hold it open for a moment. Windows' own guidance is to retry rather
	// than treat the first failure as final.
	openAttempts = 6
	openBackoff  = 10 * time.Millisecond
)

func readFiles() ([]string, error) {
	// OpenClipboard associates the clipboard with the calling thread, and
	// CloseClipboard must come from that same thread. A tea.Cmd runs on a
	// goroutine that the scheduler may move, so pin it for the duration.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var opened bool
	for range openAttempts {
		if r, _, _ := procOpenClipboard.Call(0); r != 0 {
			opened = true
			break
		}
		time.Sleep(openBackoff)
	}
	if !opened {
		return nil, ErrEmpty
	}
	defer procCloseClipboard.Call() //nolint:errcheck

	// The handle stays owned by the clipboard; it must not be freed here,
	// and it is only valid until CloseClipboard.
	hDrop, _, _ := procGetClipboardData.Call(cfHDrop)
	if hDrop == 0 {
		return nil, ErrEmpty
	}

	count, _, _ := procDragQueryFileW.Call(hDrop, queryFileCount, 0, 0)
	if count == 0 {
		return nil, ErrEmpty
	}

	paths := make([]string, 0, count)
	for i := range count {
		// Called with a nil buffer, DragQueryFile returns the length in
		// characters excluding the terminator, hence the +1.
		size, _, _ := procDragQueryFileW.Call(hDrop, i, 0, 0)
		if size == 0 {
			continue
		}
		buf := make([]uint16, size+1)
		if r, _, _ := procDragQueryFileW.Call(
			hDrop, i, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
		); r == 0 {
			continue
		}
		paths = append(paths, syscall.UTF16ToString(buf))
	}
	if len(paths) == 0 {
		return nil, ErrEmpty
	}
	return paths, nil
}
