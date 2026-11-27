package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"clipnote.local/app/history"
	"github.com/ebitengine/purego"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
)

const ownClipboardFormat = "application/x-clipnote-paste"

var (
	user32            = syscall.NewLazyDLL("user32.dll")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	getForeground     = user32.NewProc("GetForegroundWindow")
	setForeground     = user32.NewProc("SetForegroundWindow")
	getWindowPID      = user32.NewProc("GetWindowThreadProcessId")
	getSequence       = user32.NewProc("GetClipboardSequenceNumber")
	registerFormat    = user32.NewProc("RegisterClipboardFormatW")
	hasFormat         = user32.NewProc("IsClipboardFormatAvailable")
	openClipboard     = user32.NewProc("OpenClipboard")
	closeClipboard    = user32.NewProc("CloseClipboard")
	getData           = user32.NewProc("GetClipboardData")
	globalSize        = kernel32.NewProc("GlobalSize")
	globalUnlock      = kernel32.NewProc("GlobalUnlock")
	queryProcessImage = kernel32.NewProc("QueryFullProcessImageNameW")
	getDriveType      = kernel32.NewProc("GetDriveTypeW")
	getAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	sendInput         = user32.NewProc("SendInput")
)

type windowsDesktop struct {
	excluded, includeHistory, own uintptr
	lock                          func(uintptr) unsafe.Pointer
}

func newDesktop() (desktop, error) {
	format := func(name string) uintptr {
		s, _ := syscall.UTF16PtrFromString(name)
		id, _, _ := registerFormat.Call(uintptr(unsafe.Pointer(s)))
		return id
	}
	d := &windowsDesktop{
		excluded:       format("ExcludeClipboardContentFromMonitorProcessing"),
		includeHistory: format("CanIncludeInClipboardHistory"),
		own:            format(ownClipboardFormat),
	}
	if d.excluded == 0 || d.includeHistory == 0 || d.own == 0 {
		return nil, errors.New("无法注册剪贴板格式")
	}
	purego.RegisterLibFunc(&d.lock, kernel32.Handle(), "GlobalLock")
	return d, nil
}

func windowPID(hwnd uintptr) int32 {
	var pid int32
	getWindowPID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func (d *windowsDesktop) front() pasteTarget {
	hwnd, _, _ := getForeground.Call()
	return pasteTarget{pid: windowPID(hwnd), window: hwnd}
}

func (d *windowsDesktop) preparePaste(target pasteTarget, panel uintptr) bool {
	front := d.front()
	if target.window == 0 || target.pid <= 0 || windowPID(target.window) != target.pid {
		return false
	}
	if front == target {
		return true
	}
	// Only return focus from our own panel, never from a newly active app.
	if front.window != panel || front.pid != int32(os.Getpid()) {
		return false
	}
	setForeground.Call(target.window)
	return d.front() == target
}

func (d *windowsDesktop) frontInfo() (int32, string, string) {
	target := d.front()
	const queryLimitedInformation = 0x1000
	process, err := syscall.OpenProcess(queryLimitedInformation, false, uint32(target.pid))
	if err != nil {
		return target.pid, "", ""
	}
	defer syscall.CloseHandle(process)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	ok, _, _ := queryProcessImage.Call(uintptr(process), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if ok == 0 {
		return target.pid, "", ""
	}
	path := syscall.UTF16ToString(buf[:n])
	return target.pid, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), path
}

func (d *windowsDesktop) appIconPNG(path string) []byte {
	// History can outlive an installation. Only ask the shell for local icons.
	vol := filepath.VolumeName(path)
	if len(vol) != 2 || !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".exe") {
		return nil
	}
	root, _ := syscall.UTF16PtrFromString(vol + "\\")
	drive, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(root)))
	if drive != 3 { // DRIVE_FIXED
		return nil
	}
	icon, _ := mygo.Shell.FileIcon(path, 48)
	return icon
}

func (*windowsDesktop) canPaste() bool { return true }
func (*windowsDesktop) askPaste() bool { return true }
func (*windowsDesktop) count() int64 {
	n, _, _ := getSequence.Call()
	return int64(n)
}

func clipboardHas(format uintptr) bool {
	ok, _, _ := hasFormat.Call(format)
	return ok != 0
}

// Hold the same clipboard lock while checking privacy marks and reading text.
func (d *windowsDesktop) read() (string, bool) {
	for attempt := 0; ; attempt++ {
		if ok, _, _ := openClipboard.Call(0); ok != 0 {
			break
		}
		if attempt == 4 {
			return "", false
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer closeClipboard.Call()
	const cfHDrop, cfUnicodeText = 15, 13
	if clipboardHas(cfHDrop) || clipboardHas(d.excluded) || clipboardHas(d.own) || !clipboardHas(cfUnicodeText) {
		return "", false
	}
	if clipboardHas(d.includeHistory) {
		h, _, _ := getData.Call(d.includeHistory)
		size, _, _ := globalSize.Call(h)
		if size < 4 {
			return "", false
		}
		ptr := d.lock(h)
		if ptr == nil {
			return "", false
		}
		allowed := *(*uint32)(ptr) == 1
		globalUnlock.Call(h)
		if !allowed {
			return "", false
		}
	}
	h, _, _ := getData.Call(cfUnicodeText)
	size, _, _ := globalSize.Call(h)
	if size < 2 {
		return "", false
	}
	ptr := d.lock(h)
	if ptr == nil {
		return "", false
	}
	defer globalUnlock.Call(h)
	units := unsafe.Slice((*uint16)(ptr), min(size/2, history.MaxText+1))
	end := slices.Index(units, 0)
	if end <= 0 {
		return "", false
	}
	text := syscall.UTF16ToString(units[:end])
	return text, history.Valid(text)
}

// INPUT contains a union sized for MOUSEINPUT even for keyboard events.
// Both supported Windows architectures use the 64-bit ABI.
type keyboardInput struct {
	kind        uint32
	_           uint32
	key, scan   uint16
	flags, time uint32
	_           uint32
	extra       uintptr
	_           [8]byte
}

func keyDown(key uintptr) bool {
	state, _, _ := getAsyncKeyState.Call(key)
	return state&0x8000 != 0
}

func (d *windowsDesktop) paste(target pasteTarget, text string) error {
	if target.pid <= 0 || target.pid == int32(os.Getpid()) || d.front() != target {
		return errors.New("原窗口不在前台，已取消粘贴")
	}
	data := transfer.New(transfer.NewItem(transfer.Bytes(transfer.Text, []byte(text)), transfer.Bytes(ownClipboardFormat, []byte{1})))
	if err := mygo.Clipboard.Write(data); err != nil {
		return errors.New("无法写入系统剪贴板")
	}
	if err := mygo.Clipboard.Flush(); err != nil {
		return errors.New("无法保留系统剪贴板，请重试")
	}
	const vkCtrl, vkV = 0x11, 0x56
	const keyUp = 2
	events := []keyboardInput{{kind: 1, key: vkCtrl}, {kind: 1, key: vkV}, {kind: 1, key: vkV, flags: keyUp}, {kind: 1, key: vkCtrl, flags: keyUp}}
	if keyDown(vkCtrl) {
		events = events[1:3] // A global Ctrl+number shortcut can still be held.
	}
	if d.front() != target || !clipboardHas(d.own) || keyDown(0x10) || keyDown(0x12) || keyDown(0x5b) || keyDown(0x5c) {
		return errors.New("窗口、剪贴板或修饰键已改变，已取消自动粘贴")
	}
	n, _, _ := sendInput.Call(uintptr(len(events)), uintptr(unsafe.Pointer(&events[0])), unsafe.Sizeof(events[0]))
	if n != uintptr(len(events)) {
		return errors.New("窗口未接受粘贴按键。文本已复制，可手动按 Ctrl+V")
	}
	return nil
}
