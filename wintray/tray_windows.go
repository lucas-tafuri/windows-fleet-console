//go:build windows

package wintray

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmRButtonUp   = 0x0205
	wmLButtonUp   = 0x0202
	wmContextMenu = 0x007B
	wmApp         = 0x8000
	wmTray        = wmApp + 1
	wmNull        = 0x0000

	nimAdd    = 0
	nimDelete = 2

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	mfString = 0x00000000

	tpmLeftAlign   = 0x0000
	tpmBottomAlign = 0x0020
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100

	idiApplication = 32512

	dibRGBColors = 0
	biRGB        = 0
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")

	procRegisterClassExW     = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW      = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW       = modUser32.NewProc("DefWindowProcW")
	procGetMessageW          = modUser32.NewProc("GetMessageW")
	procTranslateMessage     = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW     = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage      = modUser32.NewProc("PostQuitMessage")
	procPostMessageW         = modUser32.NewProc("PostMessageW")
	procDestroyWindow        = modUser32.NewProc("DestroyWindow")
	procGetModuleHandleW     = modKernel32.NewProc("GetModuleHandleW")
	procShellNotifyIconW     = modShell32.NewProc("Shell_NotifyIconW")
	procLoadIconW            = modUser32.NewProc("LoadIconW")
	procCreatePopupMenu      = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW          = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu       = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu          = modUser32.NewProc("DestroyMenu")
	procSetForegroundWindow  = modUser32.NewProc("SetForegroundWindow")
	procGetCursorPos         = modUser32.NewProc("GetCursorPos")
	procGetDC                = modUser32.NewProc("GetDC")
	procReleaseDC            = modUser32.NewProc("ReleaseDC")
	procCreateDIBSection     = modGdi32.NewProc("CreateDIBSection")
	procCreateBitmap         = modGdi32.NewProc("CreateBitmap")
	procCreateIconIndirect   = modUser32.NewProc("CreateIconIndirect")
	procDeleteObject         = modGdi32.NewProc("DeleteObject")
	procDestroyIcon          = modUser32.NewProc("DestroyIcon")
)

var wndProcCallback = windows.NewCallback(wndProc)

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type msg struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

type point struct {
	X, Y int32
}

type notifyIconData struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UTimeoutOrVer    uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

var (
	mu      sync.Mutex
	hwnd    windows.Handle
	hIcon   windows.Handle
	nid     notifyIconData
	items   []Item
	onCmd   func(uint32)
)

func run(cfg Config) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	mu.Lock()
	items = append([]Item(nil), cfg.Items...)
	onCmd = cfg.OnCommand
	mu.Unlock()

	inst, _, _ := procGetModuleHandleW.Call(0)
	className, err := windows.UTF16PtrFromString("FleetTrayWnd")
	if err != nil {
		return err
	}

	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   wndProcCallback,
		HInstance:     windows.Handle(inst),
		LpszClassName: className,
	}
	atom, _, classErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return fmt.Errorf("register class: %v", classErr)
	}

	title, _ := windows.UTF16PtrFromString("Fleet")
	h, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		0,
		0, 0, 0, 0,
		0, 0, inst, 0,
	)
	if h == 0 {
		return fmt.Errorf("create window: %v", createErr)
	}

	mu.Lock()
	hwnd = windows.Handle(h)
	mu.Unlock()

	icon := makeFleetIcon()
	if icon == 0 {
		fallback, _, _ := procLoadIconW.Call(0, uintptr(idiApplication))
		icon = windows.Handle(fallback)
	}
	hIcon = icon

	nid = notifyIconData{
		HWnd:             hwnd,
		UID:              1,
		UFlags:           nifMessage | nifIcon | nifTip,
		UCallbackMessage: wmTray,
		HIcon:            hIcon,
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	tip := cfg.Tooltip
	if tip == "" {
		tip = "Fleet"
	}
	copyUTF16(nid.SzTip[:], tip)

	r, _, notifyErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	if r == 0 {
		return fmt.Errorf("notify icon: %v", notifyErr)
	}

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	removeIcon()
	return nil
}

func quit() {
	mu.Lock()
	h := hwnd
	mu.Unlock()
	if h != 0 {
		procPostMessageW.Call(uintptr(h), wmClose, 0, 0)
	}
}

func wndProc(h, msg, wparam, lparam uintptr) uintptr {
	switch msg {
	case wmTray:
		switch lparam {
		case wmRButtonUp, wmLButtonUp, wmContextMenu:
			showMenu(h)
		}
		return 0
	case wmCommand:
		id := uint32(wparam & 0xffff)
		mu.Lock()
		fn := onCmd
		mu.Unlock()
		if fn != nil && id != 0 {
			fn(id)
		}
		return 0
	case wmClose:
		procDestroyWindow.Call(h)
		return 0
	case wmDestroy:
		removeIcon()
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(h, msg, wparam, lparam)
	return r
}

func showMenu(h uintptr) {
	mu.Lock()
	menuItems := append([]Item(nil), items...)
	fn := onCmd
	mu.Unlock()

	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	for _, item := range menuItems {
		title, err := windows.UTF16PtrFromString(item.Title)
		if err != nil {
			continue
		}
		procAppendMenuW.Call(menu, mfString, uintptr(item.ID), uintptr(unsafe.Pointer(title)))
	}

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(h)
	flags := uintptr(tpmLeftAlign | tpmBottomAlign | tpmRightButton | tpmReturnCmd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, flags, uintptr(pt.X), uintptr(pt.Y), 0, h, 0)
	procPostMessageW.Call(h, wmNull, 0, 0)
	if cmd != 0 && fn != nil {
		fn(uint32(cmd))
	}
}

func removeIcon() {
	mu.Lock()
	defer mu.Unlock()
	if nid.HWnd != 0 {
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		nid.HWnd = 0
	}
	if hIcon != 0 {
		procDestroyIcon.Call(uintptr(hIcon))
		hIcon = 0
	}
	hwnd = 0
}

func copyUTF16(dst []uint16, s string) {
	u, err := windows.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(u)
	if n > len(dst) {
		n = len(dst)
		u[n-1] = 0
	}
	copy(dst[:n], u[:n])
}

func makeFleetIcon() windows.Handle {
	const size = 32
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return 0
	}
	defer procReleaseDC.Call(0, hdc)

	bmi := bitmapInfo{}
	bmi.Header.Size = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.Width = size
	bmi.Header.Height = -size
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32
	bmi.Header.Compression = biRGB

	var bits unsafe.Pointer
	color, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&bmi)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if color == 0 || bits == nil {
		return 0
	}

	pix := unsafe.Slice((*byte)(bits), size*size*4)
	cx, cy, r2 := 15.5, 15.5, 13.0*13.0
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			i := (y*size + x) * 4
			if dx*dx+dy*dy <= r2 {
				pix[i+0] = 0x2a // B
				pix[i+1] = 0xa0 // G
				pix[i+2] = 0xe8 // R amber
				pix[i+3] = 0xff
			} else {
				pix[i+0] = 0
				pix[i+1] = 0
				pix[i+2] = 0
				pix[i+3] = 0
			}
		}
	}

	maskBits := make([]byte, size*size/8)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			i := (y*size + x) * 4
			if pix[i+3] == 0 {
				byteI := y*(size/8) + x/8
				maskBits[byteI] |= 0x80 >> (x % 8)
			}
		}
	}
	mask, _, _ := procCreateBitmap.Call(size, size, 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		procDeleteObject.Call(color)
		return 0
	}

	ii := iconInfo{
		FIcon:    1,
		XHotspot: 16,
		YHotspot: 16,
		HbmMask:  windows.Handle(mask),
		HbmColor: windows.Handle(color),
	}
	icon, _, _ := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	procDeleteObject.Call(color)
	procDeleteObject.Call(mask)
	return windows.Handle(icon)
}
