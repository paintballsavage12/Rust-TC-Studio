//go:build windows

// Rust TC Studio — self-contained Windows application built using Win32.
// No injection, Rust memory access, drivers, network requests, or bypasses.
package main

import (
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"rusttcstudio/authclient"
	"rusttcstudio/automation"
	"rusttcstudio/housekeeping"
	"rusttcstudio/rustbind"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"rusttcstudio/license"
	"rusttcstudio/trader"
	"rusttcstudio/updates"
)

const (
	appTitle                          = "RUSTPILOT"
	appVersion                        = "1.5.7"
	className                         = "RustTCStudioNativeWindow"
	width                             = 1280
	height                            = 820
	TCID                              = "-97956382"
	WM_CREATE                         = 0x0001
	WM_DESTROY                        = 0x0002
	WM_MOVE                           = 0x0003
	WM_SIZE                           = 0x0005
	WM_SETFOCUS                       = 0x0007
	WM_PAINT                          = 0x000F
	WM_CLOSE                          = 0x0010
	WM_ERASEBKGND                     = 0x0014
	WM_SETCURSOR                      = 0x0020
	WM_COMMAND                        = 0x0111
	WM_TIMER                          = 0x0113
	WM_HOTKEY                         = 0x0312
	WM_KEYDOWN                        = 0x0100
	WM_LBUTTONUP                      = 0x0202
	WM_MOUSEMOVE                      = 0x0200
	WM_CTLCOLOREDIT                   = 0x0133
	EN_CHANGE                         = 0x0300
	EN_KILLFOCUS                      = 0x0200
	EN_SETFOCUS                       = 0x0100
	EM_SETLIMITTEXT                   = 0x00C5
	EM_SETSEL                         = 0x00B1
	WM_CTLCOLORSTATIC                 = 0x0138
	WM_SETFONT                        = 0x0030
	WM_APP                            = 0x8000
	WS_CAPTION                        = 0x00C00000
	WS_SYSMENU                        = 0x00080000
	WS_MINIMIZEBOX                    = 0x00020000
	WS_VISIBLE                        = 0x10000000
	WS_CHILD                          = 0x40000000
	WS_TABSTOP                        = 0x00010000
	ES_AUTOHSCROLL                    = 0x0080
	ES_PASSWORD                       = 0x0020
	ES_NUMBER                         = 0x2000
	ES_CENTER                         = 0x0001
	SW_SHOW                           = 5
	SW_HIDE                           = 0
	SW_RESTORE                        = 9
	CS_VREDRAW                        = 1
	CS_HREDRAW                        = 2
	DT_LEFT                           = 0
	DT_CENTER                         = 1
	DT_RIGHT                          = 2
	DT_VCENTER                        = 4
	DT_SINGLELINE                     = 0x20
	DT_NOPREFIX                       = 0x800
	DT_WORDBREAK                      = 0x10
	TRANSPARENT                       = 1
	PS_SOLID                          = 0
	SRCCOPY                           = 0x00CC0020
	GMEM_MOVEABLE                     = 0x0002
	CF_UNICODETEXT                    = 13
	MOD_NOREPEAT                      = 0x4000
	VK_RETURN                         = 0x0D
	VK_ESCAPE                         = 0x1B
	VK_F2                             = 0x71
	VK_F12                            = 0x7B
	VK_INSERT                         = 0x2D
	VK_DELETE                         = 0x2E
	VK_HOME                           = 0x24
	VK_END                            = 0x23
	VK_NUMPAD0                        = 0x60
	VK_NUMPAD9                        = 0x69
	VK_CONTROL                        = 0x11
	VK_SHIFT                          = 0x10
	VK_MENU                           = 0x12
	KEYEVENTF_KEYUP                   = 0x0002
	INPUT_KEYBOARD                    = 1
	MB_OK                             = 0
	MB_ICONWARNING                    = 0x30
	MB_YESNO                          = 4
	MB_ICONQUESTION                   = 0x20
	IDYES                             = 6
	HWND_TOP                          = 0
	SWP_NOZORDER                      = 0x0004
	SWP_NOACTIVATE                    = 0x0010
	PROCESS_QUERY_LIMITED_INFORMATION = 0x1000
)

type RECT struct{ Left, Top, Right, Bottom int32 }
type POINT struct{ X, Y int32 }

// Win32 INPUT/KEYBDINPUT are 32-byte and 24-byte structs on 64-bit Windows.
// SendInput is the standard documented Windows API for synthesized key events.
type KEYBDINPUT struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}
type INPUT struct {
	Type uint32
	_    uint32
	Ki   KEYBDINPUT
	_    [8]byte // Fill Win64 INPUT union to 32 bytes (40 total).
}

// Compile-time layout assertion: Windows SendInput rejects the wrong cbSize.
var _ [40 - unsafe.Sizeof(INPUT{})]byte
var _ [unsafe.Sizeof(INPUT{}) - 40]byte

type WNDCLASSEX struct {
	Size, Style                        uint32
	WndProc                            uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	IconSmall                          uintptr
}
type MSG struct {
	HWnd           uintptr
	Message        uint32
	_              uint32
	WParam, LParam uintptr
	Time           uint32
	Pt             POINT
	Private        uint32
}
type PAINTSTRUCT struct {
	Hdc       uintptr
	Erase     int32
	Rc        RECT
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

type Config struct {
	User               string `json:"user"`
	Salt               string `json:"salt"`
	Hash               string `json:"hash"`
	KeyStart           uint32 `json:"key_start"`
	KeyStop            uint32 `json:"key_stop"`
	KeyRefund          uint32 `json:"key_refund"`
	KeyQueue           uint32 `json:"key_queue"`
	KeyRefresh         uint32 `json:"key_refresh"`
	KeyCancel          uint32 `json:"key_cancel"`
	QueueMs            int    `json:"queue_ms"`
	RefreshMs          int    `json:"refresh_ms"`
	RefundTries        int    `json:"refund_tries"`
	UpdateURL          string `json:"update_url,omitempty"`
	TraderFreeMain     int    `json:"trader_free_main,omitempty"`
	TraderFreeBackpack int    `json:"trader_free_backpack,omitempty"`
	TraderStackRoom    int    `json:"trader_stack_room,omitempty"`
	TraderScrap        int    `json:"trader_scrap,omitempty"`
	TraderVerified     bool   `json:"trader_verified,omitempty"`
	AuthURL            string `json:"auth_url,omitempty"`
}

type hit struct {
	r  RECT
	id string
}
type Activity struct {
	at  string
	msg string
}

type App struct {
	lastInputProblem                                    string
	autoEngine                                          automation.Engine
	lastRustFocus                                       bool
	foregroundDesc                                      string
	lastFocusDiagnostic                                 time.Time
	inputReported                                       bool
	hwnd, userEdit, passEdit, updateEdit, adminNameEdit uintptr
	editBrush                                           uintptr
	traderEdits                                         [4]uintptr
	traderEditSyncing                                   bool
	fonts                                               map[string]uintptr
	cfg                                                 Config
	authed                                              bool
	hotkeysRegistered                                   bool
	page                                                string
	running                                             bool
	refunding                                           bool
	refundLeft                                          int
	pendingKey                                          string
	queueAttempts, refreshAttempts, refundAttempts      int
	started                                             time.Time
	queueAt, refreshAt, refundAt                        time.Time
	lastStatusDraw                                      time.Time
	focus                                               bool
	rustExists                                          bool
	rustWnd                                             uintptr
	hits                                                []hit
	hovered                                             string
	notice                                              string
	noticeAt                                            time.Time
	logs                                                []Activity
	configPath                                          string
	licenseID                                           string
	rememberLogin                                       bool
	authPending, adminPINPending, onlinePIN             bool
	authGen                                             int64
	authNotice                                          string
	authLocalPinned                                     bool
	activeSessionToken                                  string
	sessionChecking                                     bool
	lastSessionCheck                                    time.Time
	isAdmin, ownerLocal                                 bool
	adminIssuedCode, adminMessage                       string
	updateManifest                                      updates.Manifest
	updateChecking, updateDownloading, updateAvailable  bool
	updateStatus, updateError                           string
	lastUpdateCheck                                     time.Time
}

var a = &App{page: "stacker", fonts: map[string]uintptr{}}

var u32 = syscall.NewLazyDLL("user32.dll")
var g32 = syscall.NewLazyDLL("gdi32.dll")
var k32 = syscall.NewLazyDLL("kernel32.dll")

func proc(d *syscall.LazyDLL, name string) *syscall.LazyProc { return d.NewProc(name) }

var (
	RegisterClassEx           = proc(u32, "RegisterClassExW")
	CreateWindowEx            = proc(u32, "CreateWindowExW")
	DefWindowProc             = proc(u32, "DefWindowProcW")
	ShowWindow                = proc(u32, "ShowWindow")
	UpdateWindow              = proc(u32, "UpdateWindow")
	GetMessage                = proc(u32, "GetMessageW")
	TranslateMessage          = proc(u32, "TranslateMessage")
	DispatchMessage           = proc(u32, "DispatchMessageW")
	PostQuitMessage           = proc(u32, "PostQuitMessage")
	InvalidateRect            = proc(u32, "InvalidateRect")
	BeginPaint                = proc(u32, "BeginPaint")
	EndPaint                  = proc(u32, "EndPaint")
	SetTimer                  = proc(u32, "SetTimer")
	KillTimer                 = proc(u32, "KillTimer")
	LoadCursor                = proc(u32, "LoadCursorW")
	GetModuleHandle           = proc(k32, "GetModuleHandleW")
	GetWindowText             = proc(u32, "GetWindowTextW")
	SetWindowText             = proc(u32, "SetWindowTextW")
	RegisterHotKey            = proc(u32, "RegisterHotKey")
	UnregisterHotKey          = proc(u32, "UnregisterHotKey")
	MessageBox                = proc(u32, "MessageBoxW")
	CreateFont                = proc(g32, "CreateFontW")
	CreateSolidBrush          = proc(g32, "CreateSolidBrush")
	CreatePen                 = proc(g32, "CreatePen")
	SelectObject              = proc(g32, "SelectObject")
	DeleteObject              = proc(g32, "DeleteObject")
	SetBkMode                 = proc(g32, "SetBkMode")
	SetTextColor              = proc(g32, "SetTextColor")
	SetBkColor                = proc(g32, "SetBkColor")
	DrawText                  = proc(u32, "DrawTextW")
	RoundRect                 = proc(g32, "RoundRect")
	Rectangle                 = proc(g32, "Rectangle")
	CreateCompatibleDC        = proc(g32, "CreateCompatibleDC")
	CreateCompatibleBitmap    = proc(g32, "CreateCompatibleBitmap")
	DeleteDC                  = proc(g32, "DeleteDC")
	BitBlt                    = proc(g32, "BitBlt")
	GetForegroundWindow       = proc(u32, "GetForegroundWindow")
	GetWindowThreadProcessId  = proc(u32, "GetWindowThreadProcessId")
	OpenProcess               = proc(k32, "OpenProcess")
	QueryFullProcessImageName = proc(k32, "QueryFullProcessImageNameW")
	CloseHandle               = proc(k32, "CloseHandle")
	EnumWindows               = proc(u32, "EnumWindows")
	IsWindowVisible           = proc(u32, "IsWindowVisible")
	SetForegroundWindow       = proc(u32, "SetForegroundWindow")
	IsIconic                  = proc(u32, "IsIconic")
	KeybdEvent                = proc(u32, "keybd_event")
	SendInput                 = proc(u32, "SendInput")
	MapVirtualKey             = proc(u32, "MapVirtualKeyW")
	OpenClipboard             = proc(u32, "OpenClipboard")
	EmptyClipboard            = proc(u32, "EmptyClipboard")
	SetClipboardData          = proc(u32, "SetClipboardData")
	CloseClipboard            = proc(u32, "CloseClipboard")
	GlobalAlloc               = proc(k32, "GlobalAlloc")
	GlobalLock                = proc(k32, "GlobalLock")
	GlobalUnlock              = proc(k32, "GlobalUnlock")
	GlobalFree                = proc(k32, "GlobalFree")
	RtlMoveMemory             = proc(syscall.NewLazyDLL("ntdll.dll"), "RtlMoveMemory")
	SetProcessDPIAware        = proc(u32, "SetProcessDPIAware")
	SetWindowPos              = proc(u32, "SetWindowPos")
	SetFocus                  = proc(u32, "SetFocus")
	SendMessage               = proc(u32, "SendMessageW")
)

func p16(s string) *uint16     { return syscall.StringToUTF16Ptr(s) }
func color(hex uint32) uintptr { return uintptr((hex&0xff)<<16 | (hex & 0xff00) | (hex>>16)&0xff) }

const (
	colBG      = 0x08040F
	colSide    = 0x10081A
	colPanel   = 0x180E25
	colPanel2  = 0x251334
	colField   = 0x0B0714
	colText    = 0xF9F5FF
	colMuted   = 0xB6AAC8
	colFaint   = 0x81728F
	colOrange  = 0xCB30FA
	colOrange2 = 0x7C19BC
	colGreen   = 0x3CED77
	colRed     = 0xFF688C
	colLine    = 0x4E2367
	colBlue    = 0xA963FF
)

func rect(x, y, w, h int) RECT { return RECT{int32(x), int32(y), int32(x + w), int32(y + h)} }
func inside(r RECT, x, y int) bool {
	return x >= int(r.Left) && x < int(r.Right) && y >= int(r.Top) && y < int(r.Bottom)
}
func lo16(v uintptr) int { return int(uint16(v & 0xffff)) }
func hi16(v uintptr) int { return int(uint16((v >> 16) & 0xffff)) }

func defaultConfig() Config {
	return Config{KeyStart: 0x75, KeyStop: 0x76, KeyRefund: 0x77, KeyQueue: 0x67, KeyRefresh: 0x68, KeyCancel: 0x69, QueueMs: 800, RefreshMs: 2400, RefundTries: 25}
}
func configFilename() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "RustTCStudio", "config.json")
}
func loadConfig() {
	a.configPath = configFilename()
	a.cfg = defaultConfig()
	data, err := os.ReadFile(a.configPath)
	if err == nil {
		_ = json.Unmarshal(data, &a.cfg)
	}
	if a.cfg.KeyStart == 0 {
		a.cfg.KeyStart = 0x75
	}
	if a.cfg.KeyStop == 0 {
		a.cfg.KeyStop = 0x76
	}
	if a.cfg.KeyRefund == 0 {
		a.cfg.KeyRefund = 0x77
	}
	if a.cfg.KeyQueue == 0 {
		a.cfg.KeyQueue = 0x67
	}
	if a.cfg.KeyRefresh == 0 {
		a.cfg.KeyRefresh = 0x68
	}
	if a.cfg.KeyCancel == 0 {
		a.cfg.KeyCancel = 0x69
	}
	if a.cfg.QueueMs < 300 || a.cfg.QueueMs > 3000 {
		a.cfg.QueueMs = 800
	}
	if a.cfg.RefreshMs < 1000 || a.cfg.RefreshMs > 5500 {
		a.cfg.RefreshMs = 2400
	}
	if a.cfg.RefundTries < 1 || a.cfg.RefundTries > 60 {
		a.cfg.RefundTries = 25
	}
}
func loadAuthServer() {
	// A packaged auth_server.txt can override a previously saved URL during updates.
	exe, err := os.Executable()
	if err != nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "auth_server.txt"))
	if err != nil {
		return
	}
	v := strings.TrimSpace(string(data))
	if _, err := authclient.New(v); err == nil {
		a.authLocalPinned = true
		if a.cfg.AuthURL != v {
			a.cfg.AuthURL = v
			saveConfig()
		}
	}
}
func saveConfig() {
	_ = os.MkdirAll(filepath.Dir(a.configPath), 0700)
	data, err := json.MarshalIndent(a.cfg, "", "  ")
	if err != nil {
		return
	}
	temp := a.configPath + ".tmp"
	if err = os.WriteFile(temp, data, 0600); err == nil {
		_ = os.Rename(temp, a.configPath)
	}
}
func addLog(t string) {
	a.logs = append([]Activity{{time.Now().Format("15:04:05"), t}}, a.logs...)
	if len(a.logs) > 25 {
		a.logs = a.logs[:25]
	}
}
func toast(s string) { a.notice = s; a.noticeAt = time.Now(); refreshUI() }
func refreshUI() {
	if a.hwnd != 0 {
		InvalidateRect.Call(a.hwnd, 0, 0)
	}
}
func showMessage(s, title string, flags uintptr) int {
	r, _, _ := MessageBox.Call(a.hwnd, uintptr(unsafe.Pointer(p16(s))), uintptr(unsafe.Pointer(p16(title))), flags)
	return int(r)
}

func editValue(h uintptr) string {
	buf := make([]uint16, 1024)
	GetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
func setEdit(h uintptr, s string) { SetWindowText.Call(h, uintptr(unsafe.Pointer(p16(s)))) }

// The 4-digit owner PIN is intentionally ONLY a convenience unlock for the
// private installation. It is not internet authentication. The owner signing
// key must also be present beside the executable; player packages NEVER ship it.
const ownerPIN = "2436"

func privateOwnerKey() (ed25519.PrivateKey, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exe), "OWNER_PRIVATE_KEY.txt"))
	if err != nil {
		return nil, err
	}
	return license.ReadPrivateKey(string(data))
}

func hasPrivateOwnerKey() bool {
	_, err := privateOwnerKey()
	return err == nil
}

func issueAdminCode(role string) {
	if !a.authed || !a.isAdmin {
		return
	}
	key, err := privateOwnerKey()
	if err != nil {
		a.adminMessage = "Private owner signing key not found. Owner Manager functions unavailable here."
		toast(a.adminMessage)
		return
	}
	name := strings.TrimSpace(editValue(a.adminNameEdit))
	if !license.ValidUsername(name) {
		a.adminMessage = "Enter a username with 2-24 visible characters."
		toast(a.adminMessage)
		return
	}
	code, claim, err := license.SignRole(key, name, role)
	if err != nil {
		a.adminMessage = "Unable to sign code: " + err.Error()
		toast(a.adminMessage)
		return
	}
	a.adminIssuedCode = code
	if copyClipboard(code) {
		a.adminMessage = "Issued " + claim.Role + " code for " + claim.Username + "; copied to clipboard."
	} else {
		a.adminMessage = "Code created. Clipboard busy; use COPY LAST CODE."
	}
	addLog("Access issued: " + claim.Username + " (" + claim.Role + ")")
	refreshUI()
}

// Vending inputs are native EDIT controls so the user can click, select and type
// numbers directly. No data comes from the Rust process or from memory scanning.
// Each value is bounded; input confirmation becomes invalid when anything changes.
const traderControlBase = 201

var traderMaxima = [4]int{30, 40, 30000, 500000}
var traderFieldsY = [4]int{440, 492, 544, 596}

func traderValues() [4]int {
	return [4]int{a.cfg.TraderFreeMain, a.cfg.TraderFreeBackpack, a.cfg.TraderStackRoom, a.cfg.TraderScrap}
}

func setTraderValue(index, v int) {
	switch index {
	case 0:
		a.cfg.TraderFreeMain = v
	case 1:
		a.cfg.TraderFreeBackpack = v
	case 2:
		a.cfg.TraderStackRoom = v
	case 3:
		a.cfg.TraderScrap = v
	}
}

func syncTraderEdits() {
	a.traderEditSyncing = true
	values := traderValues()
	for i, edit := range a.traderEdits {
		if edit != 0 {
			setEdit(edit, fmt.Sprint(values[i]))
		}
	}
	a.traderEditSyncing = false
}

func commitTraderEdit(i int, normalize bool) error {
	if i < 0 || i >= len(a.traderEdits) || a.traderEdits[i] == 0 {
		return nil
	}
	n, err := trader.ParseNumber(editValue(a.traderEdits[i]), traderMaxima[i])
	if err != nil {
		return fmt.Errorf("field %d: %w", i+1, err)
	}
	if traderValues()[i] != n {
		setTraderValue(i, n)
		a.cfg.TraderVerified = false
		saveConfig()
	}
	if normalize {
		a.traderEditSyncing = true
		setEdit(a.traderEdits[i], fmt.Sprint(n))
		a.traderEditSyncing = false
	}
	return nil
}

func commitAllTraderEdits() error {
	// Validate every field first so invalid text cannot partially alter the plan.
	var values [4]int
	for i, edit := range a.traderEdits {
		if edit == 0 {
			return fmt.Errorf("numeric field unavailable")
		}
		n, err := trader.ParseNumber(editValue(edit), traderMaxima[i])
		if err != nil {
			return fmt.Errorf("field %d: %w", i+1, err)
		}
		values[i] = n
	}
	old := traderValues()
	for i, v := range values {
		setTraderValue(i, v)
	}
	if values != old {
		a.cfg.TraderVerified = false
	}
	syncTraderEdits()
	saveConfig()
	return nil
}

func showEdits() {
	// Player only enters a signed access code. Username is assigned by owner.
	ShowWindow.Call(a.userEdit, SW_HIDE)
	if a.updateEdit != 0 {
		if a.authed && a.page == "updates" {
			ShowWindow.Call(a.updateEdit, SW_SHOW)
			SetWindowPos.Call(a.updateEdit, 0, 300, 452, 892, 43, SWP_NOZORDER|SWP_NOACTIVATE)
		} else {
			ShowWindow.Call(a.updateEdit, SW_HIDE)
		}
	}
	if a.adminNameEdit != 0 {
		if a.authed && a.isAdmin && a.page == "admin" {
			ShowWindow.Call(a.adminNameEdit, SW_SHOW)
			SetWindowPos.Call(a.adminNameEdit, 0, 297, 426, 548, 48, SWP_NOZORDER|SWP_NOACTIVATE)
		} else {
			ShowWindow.Call(a.adminNameEdit, SW_HIDE)
		}
	}
	for i, edit := range a.traderEdits {
		if edit == 0 {
			continue
		}
		if a.authed && a.page == "vending" {
			SetWindowPos.Call(edit, 0, 563, uintptr(traderFieldsY[i]+4), 91, 23, SWP_NOZORDER|SWP_NOACTIVATE)
			ShowWindow.Call(edit, SW_SHOW)
		} else {
			ShowWindow.Call(edit, SW_HIDE)
		}
	}
	if a.authed {
		ShowWindow.Call(a.passEdit, SW_HIDE)
	} else {
		ShowWindow.Call(a.passEdit, SW_SHOW)
		SetWindowPos.Call(a.passEdit, 0, 800, 468, 326, 48, SWP_NOZORDER|SWP_NOACTIVATE)
	}
}
func login() {
	if a.authPending {
		return
	}
	accessCode := strings.TrimSpace(editValue(a.passEdit))
	if len(accessCode) == 6 && onlyDigits(accessCode) {
		beginPINLogin(accessCode)
		return
	}
	if subtle.ConstantTimeCompare([]byte(accessCode), []byte(ownerPIN)) == 1 {
		if !hasPrivateOwnerKey() {
			toast("OWNER PIN UNAVAILABLE: This copy has no private owner key.")
			return
		}
		a.isAdmin, a.ownerLocal = true, true
		a.cfg.User = "Owner:Unknown"
		a.licenseID = "LOCAL-OWNER"
	} else {
		claim, err := license.Verify(accessCode)
		if err != nil {
			toast("ACCESS DENIED: Invalid owner-issued login code.")
			return
		}
		a.isAdmin = claim.Role == license.RoleAdmin
		a.ownerLocal = false
		a.cfg.User = claim.Username
		a.licenseID = claim.Identifier
	}
	a.authGen++
	a.authPending = false
	a.onlinePIN = false
	a.activeSessionToken = ""
	a.authed = true
	if a.rememberLogin {
		stored := accessCode
		if a.ownerLocal {
			stored = ownerSession
		}
		if err := rememberCredential(stored); err != nil {
			toast("Windows could not save the remembered login.")
		}
	} else {
		clearRememberedCredential()
	}
	a.cfg.Salt, a.cfg.Hash = "", ""
	saveConfig()
	setEdit(a.passEdit, "")
	a.page = "dashboard"
	showEdits()
	SetFocus.Call(a.hwnd)
	registerHotkeys()
	addLog("Signed in: " + a.cfg.User)
	refreshUI()
}
func signOut() {
	a.authGen++
	a.authPending = false
	a.onlinePIN = false
	a.activeSessionToken = ""
	clearRememberedCredential()
	a.rememberLogin = false
	stopStack()
	unregisterHotkeys()
	a.authed = false
	a.licenseID = ""
	a.isAdmin, a.ownerLocal = false, false
	a.adminIssuedCode = ""
	a.page = "dashboard"
	setEdit(a.passEdit, "")
	showEdits()
	SetFocus.Call(a.passEdit)
	refreshUI()
}

func vkName(v uint32) string {
	if v >= 0x70 && v <= 0x7B {
		return fmt.Sprintf("F%d", v-0x70+1)
	}
	if v >= VK_NUMPAD0 && v <= VK_NUMPAD9 {
		return fmt.Sprintf("NUMPAD %d", v-VK_NUMPAD0)
	}
	if v >= 0x41 && v <= 0x5A {
		return string(rune(v))
	}
	if v >= 0x30 && v <= 0x39 {
		return string(rune(v))
	}
	switch v {
	case VK_INSERT:
		return "INSERT"
	case VK_DELETE:
		return "DELETE"
	case VK_HOME:
		return "HOME"
	case VK_END:
		return "END"
	}
	return fmt.Sprintf("0x%02X", v)
}
func validVK(v uint32) bool {
	if v >= 0x71 && v <= 0x7B {
		return true
	}
	if v >= VK_NUMPAD0 && v <= VK_NUMPAD9 {
		return true
	}
	if v >= 0x41 && v <= 0x5A {
		return true
	}
	if v >= 0x30 && v <= 0x39 {
		return true
	}
	return v == VK_INSERT || v == VK_DELETE || v == VK_HOME || v == VK_END
}
func controlKeys() []uint32 { return []uint32{a.cfg.KeyStart, a.cfg.KeyStop, a.cfg.KeyRefund} }
func allKeys() []uint32 {
	return []uint32{a.cfg.KeyStart, a.cfg.KeyStop, a.cfg.KeyRefund, a.cfg.KeyQueue, a.cfg.KeyRefresh, a.cfg.KeyCancel}
}
func unregisterHotkeys() {
	if !a.hotkeysRegistered {
		return
	}
	for i := 1; i <= 3; i++ {
		UnregisterHotKey.Call(a.hwnd, uintptr(i))
	}
	a.hotkeysRegistered = false
}
func registerHotkeys() {
	unregisterHotkeys()
	if !a.authed {
		return
	}
	for i, key := range controlKeys() {
		r, _, _ := RegisterHotKey.Call(a.hwnd, uintptr(i+1), MOD_NOREPEAT, uintptr(key))
		if r == 0 {
			addLog("Could not register " + vkName(key) + " (already in use)")
		}
	}
	a.hotkeysRegistered = true
}
func syncHotkeyFocus() {
	fg, _, _ := GetForegroundWindow.Call()
	should := a.authed && (fg == a.hwnd || a.focus)
	if should && !a.hotkeysRegistered {
		registerHotkeys()
	} else if !should && a.hotkeysRegistered {
		unregisterHotkeys()
	}
}
func updateKey(which string, v uint32) {
	if !validVK(v) {
		toast("Choose F2–F12, Numpad, A–Z, 0–9, Insert, Delete, Home or End.")
		return
	}
	// Key assignments are unique across controls and in-game actions.
	for _, key := range allKeys() {
		if key == v {
			toast("That key is already assigned. Choose another.")
			return
		}
	}
	switch which {
	case "start":
		a.cfg.KeyStart = v
	case "stop":
		a.cfg.KeyStop = v
	case "refund":
		a.cfg.KeyRefund = v
	case "queue":
		a.cfg.KeyQueue = v
	case "refresh":
		a.cfg.KeyRefresh = v
	case "cancel":
		a.cfg.KeyCancel = v
	}
	a.pendingKey = ""
	saveConfig()
	registerHotkeys()
	addLog("Updated " + which + " hotkey to " + vkName(v))
	toast("Saved. If a Rust action key changed, reinstall the F1 binds.")
}
func rustBindCommands() string {
	key := func(v uint32) string {
		if v >= VK_NUMPAD0 && v <= VK_NUMPAD9 {
			return fmt.Sprintf("keypad%d", v-VK_NUMPAD0)
		}
		return strings.ToLower(strings.ReplaceAll(vkName(v), " ", ""))
	}
	return rustbind.Commands(key(a.cfg.KeyQueue), key(a.cfg.KeyRefresh), key(a.cfg.KeyCancel), TCID)
}
func copyClipboard(s string) bool {
	r, _, _ := OpenClipboard.Call(a.hwnd)
	if r == 0 {
		return false
	}
	defer CloseClipboard.Call()
	utf := syscall.StringToUTF16(s)
	size := uintptr(len(utf) * 2)
	h, _, _ := GlobalAlloc.Call(GMEM_MOVEABLE, size)
	if h == 0 {
		return false
	}
	p, _, _ := GlobalLock.Call(h)
	if p == 0 {
		GlobalFree.Call(h)
		return false
	}
	RtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&utf[0])), size)
	GlobalUnlock.Call(h)
	EmptyClipboard.Call()
	r, _, _ = SetClipboardData.Call(CF_UNICODETEXT, h)
	if r == 0 {
		GlobalFree.Call(h)
		return false
	}
	return true
}

func rustExecutableName(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, _, _ := OpenProcess.Call(PROCESS_QUERY_LIMITED_INFORMATION, 0, uintptr(pid))
	if h != 0 {
		defer CloseHandle.Call(h)
		buf := make([]uint16, 512)
		sz := uint32(len(buf))
		r, _, _ := QueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&sz)))
		if r != 0 && sz > 0 {
			path := syscall.UTF16ToString(buf[:sz])
			// Windows paths always use backslashes, even when cross-compiled.
			if at := strings.LastIndexAny(path, `\/`); at >= 0 {
				return path[at+1:]
			}
			return path
		}
	}
	// QueryFullProcessImageName can fail on some Windows privilege/process
	// combinations. Toolhelp resolves only the matching foreground PID; it
	// never broadens input delivery to an arbitrary window title.
	snapshot, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return ""
	}
	defer syscall.CloseHandle(snapshot)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	if syscall.Process32First(snapshot, &pe) != nil {
		return ""
	}
	for {
		if pe.ProcessID == pid {
			return syscall.UTF16ToString(pe.ExeFile[:])
		}
		if syscall.Process32Next(snapshot, &pe) != nil {
			break
		}
	}
	return ""
}
func isRustExecutable(name string) bool {
	return strings.EqualFold(name, "RustClient.exe") || strings.EqualFold(name, "Rust.exe")
}
func processIsRust(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var pid uint32
	GetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return isRustExecutable(rustExecutableName(pid))
}
func rustForeground() bool {
	hwnd, _, _ := GetForegroundWindow.Call()
	if hwnd == 0 {
		a.foregroundDesc = "No foreground window"
		return false
	}
	var pid uint32
	GetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	exe := rustExecutableName(pid)
	if exe == "" {
		a.foregroundDesc = fmt.Sprintf("Unknown process (PID %d)", pid)
		return false
	}
	a.foregroundDesc = fmt.Sprintf("%s (PID %d)", exe, pid)
	return isRustExecutable(exe)
}
func findRustWindow() uintptr {
	found := uintptr(0)
	cb := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		visible, _, _ := IsWindowVisible.Call(hwnd)
		if visible == 0 {
			return 1
		}
		if processIsRust(hwnd) {
			found = hwnd
			return 0
		}
		return 1
	})
	EnumWindows.Call(cb, 0)
	return found
}

// sendKeyTransition sends ONE key event. The key stays down until a later
// timer tick releases it, giving games that sample input once per frame a
// meaningful chance to see the press. This does not bypass game input rules.
func sendKeyTransition(vk uint32, release bool) bool {
	sc, _, _ := MapVirtualKey.Call(uintptr(vk), 0)
	if sc == 0 {
		a.lastInputProblem = "Windows could not map in-game key " + vkName(vk)
		return false
	}
	flags := uint32(0)
	if release {
		flags = KEYEVENTF_KEYUP
	}
	input := INPUT{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{Vk: uint16(vk), Scan: uint16(sc), Flags: flags}}
	sent, _, err := SendInput.Call(1, uintptr(unsafe.Pointer(&input)), unsafe.Sizeof(INPUT{}))
	if sent != 1 {
		a.lastInputProblem = fmt.Sprintf("Windows refused %s for %s (sent %d/1): %v", func() string {
			if release {
				return "key-up"
			}
			return "key-down"
		}(), vkName(vk), sent, err)
		return false
	}
	a.lastInputProblem = ""
	return true
}
func automationKeys() automation.Keys {
	return automation.Keys{Queue: a.cfg.KeyQueue, Refresh: a.cfg.KeyRefresh, Cancel: a.cfg.KeyCancel}
}
func automationTiming() automation.Timing {
	return automation.DefaultTiming(time.Duration(a.cfg.QueueMs)*time.Millisecond, time.Duration(a.cfg.RefreshMs)*time.Millisecond)
}

// On logout, Stop and window shutdown, try to release immediately so no
// synthesized key stays pressed. If Windows refuses the release we log it.
func releaseAutomationKey() {
	if !a.autoEngine.Held() {
		return
	}
	ev, ok := a.autoEngine.Next(time.Now(), false, automationKeys(), automationTiming())
	if ok && ev.Up {
		if sendKeyTransition(ev.Key, true) {
			a.autoEngine.Confirm(ev, time.Now())
		} else {
			addLog("Could not release in-game key: " + a.lastInputProblem)
		}
	}
}
func startStack() {
	if !a.authed || a.refunding {
		return
	}
	if a.running {
		stopStack()
		return
	}
	a.running = true
	a.autoEngine.Start(time.Now())
	a.started = time.Now()
	a.lastInputProblem = ""
	a.inputReported = false
	addLog("Auto stacking started; waiting for Rust foreground")
	refreshUI()
}
func stopStack() {
	if a.running || a.refunding || a.autoEngine.Held() {
		a.running = false
		a.refunding = false
		a.refundLeft = 0
		a.autoEngine.Stop()
		releaseAutomationKey()
		addLog("Automation stopped; Rust's existing craft queue continues")
		refreshUI()
	}
}

// refund begins a bounded batch of in-game TC cancellation sweeps.
// F8 must never display a modal dialog or steal focus from Rust.
// RustPilot does not inspect the craft queue: this is an attempt, not a verified refund.
func refund() {
	if !a.authed || a.refunding {
		return
	}
	// Remember whether F8 was pressed inside Rust BEFORE stopping the stacker.
	// StopStack is safe here: it releases held keys and never switches windows.
	inGame := rustForeground()
	stopStack()
	h := findRustWindow()
	if h == 0 {
		addLog("Refund not started: Rust window not detected")
		toast("Rust not detected. Join a server first.")
		return
	}
	a.rustWnd = h
	a.refunding = true
	a.refundLeft = a.cfg.RefundTries
	a.autoEngine.StartRefund(time.Now(), a.cfg.RefundTries)
	// The toolbar button can be used from the app; return to Rust in that case.
	// Never change window focus if the user's F8 hotkey originated in Rust.
	if !inGame {
		SetForegroundWindow.Call(h)
	}
	addLog(fmt.Sprintf("TC refund started in-game: up to %d sweeps across quantity 1–10 queues. Keep inventory space free; excess wood can drop.", a.cfg.RefundTries))
	refreshUI()
}
func tick() {
	pollUpdateEvents()
	if !a.updateChecking && !a.updateDownloading && a.cfg.UpdateURL != "" && time.Since(a.lastUpdateCheck) > 6*time.Hour {
		checkForUpdate(false)
	}
	if !a.authed {
		releaseAutomationKey()
		return
	}
	now := time.Now()
	a.focus = rustForeground()
	syncHotkeyFocus()
	if !a.focus {
		a.autoEngine.Pause(now)
		if a.lastRustFocus && a.running {
			addLog("Rust lost focus; active process: " + a.foregroundDesc)
		}
		if a.running && now.Sub(a.lastFocusDiagnostic) > 8*time.Second {
			a.lastFocusDiagnostic = now
			addLog("Waiting for RustClient.exe; foreground is " + a.foregroundDesc)
		}
	} else if !a.lastRustFocus && a.running {
		addLog("Rust focused — auto crafting resumed")
	}
	a.lastRustFocus = a.focus

	if !a.running && !a.refunding && !a.autoEngine.Held() {
		return
	}
	ev, ok := a.autoEngine.Next(now, a.focus, automationKeys(), automationTiming())
	if !ok {
		redrawStatusAtMost(now)
		return
	}
	if !sendKeyTransition(ev.Key, ev.Up) {
		if !ev.Up {
			a.running = false
			a.refunding = false
			a.autoEngine.Stop()
		}
		addLog("Windows input error: " + a.lastInputProblem)
		redrawStatusAtMost(now)
		return
	}
	a.autoEngine.Confirm(ev, now)
	if ev.Up {
		// Counts indicate fully dispatched keypresses, NOT successful Rust crafts.
		switch ev.Kind {
		case automation.Queue:
			a.queueAttempts++
		case automation.Refresh:
			a.refreshAttempts++
		case automation.Refund:
			a.refundAttempts++
			a.refundLeft = a.autoEngine.Remaining()
			if !a.autoEngine.Refunding() {
				a.refunding = false
				addLog("TC refund sweeps dispatched; check the in-game queue and inventory/ground. Success is not directly detectable.")
			}
		}
		if !a.inputReported {
			addLog("Timed keypress delivered to Windows; confirm TC queue in Rust")
			a.inputReported = true
		}
	}
	redrawStatusAtMost(now)
}

// The crafting timer is fast, but the dashboard needs fewer repaints.
// Limit status updates to ~4 FPS to avoid needless GDI work on the UI thread.
func redrawStatusAtMost(now time.Time) {
	if now.Sub(a.lastStatusDraw) >= 250*time.Millisecond {
		a.lastStatusDraw = now
		refreshUI()
	}
}

func createFont(size, weight int, face string) uintptr {
	r, _, _ := CreateFont.Call(uintptr(int32(-size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 0, 0, uintptr(unsafe.Pointer(p16(face))))
	return r
}
func pickFont(hdc uintptr, name string) {
	if font := a.fonts[name]; font != 0 {
		SelectObject.Call(hdc, font)
	}
}
func fillRound(hdc uintptr, r RECT, radius int, c uint32) {
	b, _, _ := CreateSolidBrush.Call(color(c))
	pen, _, _ := CreatePen.Call(PS_SOLID, 1, color(c))
	ob, _, _ := SelectObject.Call(hdc, b)
	op, _, _ := SelectObject.Call(hdc, pen)
	RoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	SelectObject.Call(hdc, ob)
	SelectObject.Call(hdc, op)
	DeleteObject.Call(b)
	DeleteObject.Call(pen)
}
func fillRect(hdc uintptr, r RECT, c uint32) { fillRound(hdc, r, 1, c) }
func outline(hdc uintptr, r RECT, radius int, c uint32) {
	pen, _, _ := CreatePen.Call(PS_SOLID, 1, color(c))
	op, _, _ := SelectObject.Call(hdc, pen) // use null brush
	nullBrush, _, _ := g32.NewProc("GetStockObject").Call(5)
	ob, _, _ := SelectObject.Call(hdc, nullBrush)
	RoundRect.Call(hdc, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(radius), uintptr(radius))
	SelectObject.Call(hdc, ob)
	SelectObject.Call(hdc, op)
	DeleteObject.Call(pen)
}
func drawText(hdc uintptr, s string, r RECT, c uint32, font string, flags uint32) {
	if s == "" {
		return
	}
	pickFont(hdc, font)
	SetBkMode.Call(hdc, TRANSPARENT)
	SetTextColor.Call(hdc, color(c))
	DrawText.Call(hdc, uintptr(unsafe.Pointer(p16(s))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), uintptr(flags|DT_NOPREFIX))
}
func txt(hdc uintptr, s string, x, y, w, h int, c uint32, font string) {
	drawText(hdc, s, rect(x, y, w, h), c, font, DT_LEFT|DT_VCENTER|DT_SINGLELINE)
}
func button(hdc uintptr, id, label string, x, y, w, h int, accent bool) {
	c := uint32(colPanel2)
	fg := uint32(colText)
	if accent {
		c = colOrange
		fg = colBG
	}
	if a.hovered == id {
		if accent {
			c = 0xFF9B65
		} else {
			c = 0x22344E
		}
	}
	r := rect(x, y, w, h)
	fillRound(hdc, r, 12, c)
	if !accent {
		outline(hdc, r, 12, colLine)
	}
	drawText(hdc, label, r, fg, "button", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	a.hits = append(a.hits, hit{r, id})
}
func sectionTitle(hdc uintptr, title, subtitle string) {
	txt(hdc, title, 304, 68, 720, 47, colText, "title")
	txt(hdc, subtitle, 304, 114, 750, 27, colMuted, "regular")
}
func card(hdc uintptr, x, y, w, h int) {
	fillRound(hdc, rect(x, y, w, h), 20, colPanel)
	outline(hdc, rect(x, y, w, h), 20, colLine)
}
func pill(hdc uintptr, s string, x, y, w int, active bool) {
	c := uint32(colField)
	fg := uint32(colMuted)
	if active {
		c = 0x173B34
		fg = colGreen
	}
	fillRound(hdc, rect(x, y, w, 28), 13, c)
	drawText(hdc, s, rect(x, y, w, 28), fg, "smallBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
}
func drawApp(hdc uintptr) {
	a.hits = a.hits[:0]
	fillRect(hdc, rect(0, 0, width, height), colBG)
	if !a.authed {
		drawLogin(hdc)
		return
	}
	drawSide(hdc)
	switch a.page {
	case "stacker", "dashboard":
		drawDashboard(hdc)
	case "hotkeys":
		drawHotkeys(hdc)
	case "settings":
		drawSettings(hdc)
	case "setup":
		drawSetup(hdc)
	case "about":
		drawAbout(hdc)
	}
	if a.pendingKey != "" {
		drawKeyOverlay(hdc)
	}
	if a.notice != "" && time.Since(a.noticeAt) < 6*time.Second {
		fillRound(hdc, rect(322, 676, 735, 42), 12, 0x28344A)
		txt(hdc, a.notice, 345, 680, 695, 32, colText, "small")
	}
}
func drawLogin(hdc uintptr) {
	// Branded, local-only sign in screen.
	fillRound(hdc, rect(60, 54, 980, 642), 25, colSide)
	fillRound(hdc, rect(60, 54, 398, 642), 25, 0x121F31)
	fillRound(hdc, rect(118, 111, 84, 84), 20, colOrange)
	txt(hdc, "TC", 137, 119, 58, 60, colBG, "logo")
	txt(hdc, "RUST", 118, 230, 276, 61, colText, "hero")
	txt(hdc, "TC STUDIO", 118, 287, 286, 51, colOrange, "title")
	txt(hdc, "Smart queue automation.", 118, 363, 306, 30, colMuted, "regular")
	txt(hdc, "Designed for Rust wood farming.", 118, 392, 316, 29, colMuted, "regular")
	fillRound(hdc, rect(118, 511, 278, 75), 15, colField)
	txt(hdc, "NO SUBSCRIPTION", 138, 525, 250, 26, colGreen, "smallBold")
	txt(hdc, "Standalone • Offline • Local", 138, 553, 250, 23, colMuted, "small")
	txt(hdc, "WELCOME", 512, 148, 435, 61, colText, "title")
	if a.cfg.User == "" {
		txt(hdc, "Create a local profile", 512, 210, 441, 31, colMuted, "regular")
		txt(hdc, "DISPLAY NAME", 520, 307, 295, 24, colMuted, "smallBold")
		fillRound(hdc, rect(512, 336, 301, 52), 12, colField)
		txt(hdc, "PASSWORD (6+ CHARACTERS)", 520, 383, 298, 26, colMuted, "smallBold")
		fillRound(hdc, rect(512, 409, 301, 53), 12, colField)
		button(hdc, "login", "CREATE PROFILE & ENTER", 518, 490, 293, 53, true)
	} else {
		txt(hdc, "Welcome back, "+a.cfg.User, 512, 210, 453, 36, colMuted, "regular")
		txt(hdc, "YOUR PROFILE", 520, 310, 289, 24, colMuted, "smallBold")
		txt(hdc, a.cfg.User, 524, 337, 289, 32, colOrange, "cardTitle")
		txt(hdc, "PASSWORD", 520, 359, 289, 26, colMuted, "smallBold")
		fillRound(hdc, rect(512, 384, 301, 55), 12, colField)
		button(hdc, "login", "SIGN IN", 518, 466, 293, 55, true)
	}
	txt(hdc, "Credentials stay on this PC. No online account required.", 507, 617, 473, 25, colFaint, "small")
	if a.notice != "" && time.Since(a.noticeAt) < 6*time.Second {
		txt(hdc, a.notice, 520, 565, 574, 26, colRed, "smallBold")
	}
}
func drawSide(hdc uintptr) {
	fillRect(hdc, rect(0, 0, 264, height), colSide)
	fillRound(hdc, rect(27, 31, 54, 54), 14, colOrange)
	txt(hdc, "TC", 39, 39, 42, 38, colBG, "logoSmall")
	txt(hdc, "RUST TC", 98, 26, 147, 36, colText, "cardTitle")
	txt(hdc, "STUDIO / V1.0", 99, 61, 139, 25, colOrange, "smallBold")
	fillRect(hdc, rect(27, 112, 206, 1), colLine)
	items := []struct{ id, label, code string }{{"dashboard", "Dashboard", "▣"}, {"hotkeys", "Hotkeys", "⌘"}, {"settings", "Settings", "⚙"}, {"setup", "Rust setup", "<>"}, {"about", "About", "i"}}
	for i, item := range items {
		y := 142 + i*66
		r := rect(17, y, 230, 52)
		if a.page == item.id {
			fillRound(hdc, r, 12, colPanel2)
			fillRound(hdc, rect(17, y+8, 4, 35), 2, colOrange)
		}
		txt(hdc, item.code, 40, y+7, 33, 37, func() uint32 {
			if a.page == item.id {
				return colOrange
			}
			return colMuted
		}(), "navIcon")
		txt(hdc, item.label, 87, y+7, 157, 38, func() uint32 {
			if a.page == item.id {
				return colText
			}
			return colMuted
		}(), "regularBold")
		a.hits = append(a.hits, hit{r, "page:" + item.id})
	}
	fillRect(hdc, rect(24, 601, 216, 1), colLine)
	txt(hdc, "SIGNED IN AS", 30, 617, 209, 24, colFaint, "smallBold")
	txt(hdc, a.cfg.User, 30, 647, 214, 31, colText, "regularBold")
	button(hdc, "logout", "SIGN OUT", 29, 691, 203, 37, false)
}
func drawDashboard(hdc uintptr) {
	sectionTitle(hdc, "Stacking dashboard", "Queue wood through Tool Cupboards while you farm.")
	statusText := "IDLE"
	active := false
	if a.refunding {
		statusText = "REFUNDING"
	} else if a.running {
		if a.focus {
			statusText = "STACKING"
			active = true
		} else {
			statusText = "WAITING FOR RUST"
		}
	}
	pill(hdc, statusText, 874, 72, 190, active)
	card(hdc, rectx(304), 165, 760, 154)
	fillRound(hdc, rect(328, 188, 60, 60), 17, func() uint32 {
		if a.running {
			return 0x173B34
		}
		return 0x352C29
	}())
	txt(hdc, func() string {
		if a.running {
			return "ON"
		}
		return "OFF"
	}(), 340, 198, 48, 40, func() uint32 {
		if a.running {
			return colGreen
		}
		return colOrange
	}(), "cardTitle")
	txt(hdc, "AUTO QUEUE STATUS", 410, 184, 410, 33, colFaint, "smallBold")
	txt(hdc, func() string {
		if a.refunding {
			return "Returning queued wood..."
		}
		if a.running && a.focus {
			return "Active in Rust"
		}
		if a.running {
			return "Paused — Rust not focused"
		}
		return "Ready when you are"
	}(), 410, 218, 620, 43, colText, "cardTitle")
	txt(hdc, "No memory reading. The game decides if there is enough wood.", 331, 272, 692, 28, colMuted, "small")
	xs := []int{304, 564, 824}
	titles := []string{"QUEUE ATTEMPTS", "REFRESH ATTEMPTS", "CANCEL ATTEMPTS"}
	vals := []int{a.queueAttempts, a.refreshAttempts, a.refundAttempts}
	for i := 0; i < 3; i++ {
		card(hdc, xs[i], 336, 240, 126)
		txt(hdc, titles[i], xs[i]+18, 351, 205, 25, colFaint, "smallBold")
		txt(hdc, fmt.Sprint(vals[i]), xs[i]+18, 388, 210, 57, colText, "stat")
	}
	card(hdc, 304, 479, 760, 168)
	txt(hdc, "QUICK CONTROLS", 328, 496, 410, 30, colText, "cardTitle")
	button(hdc, "start", func() string {
		if a.running {
			return "PAUSE STACKING"
		}
		return "START STACKING"
	}(), 329, 537, 231, 58, true)
	button(hdc, "stop", "STOP SENDING", 579, 537, 217, 58, false)
	button(hdc, "refund", "REFUND WOOD", 816, 537, 225, 58, false)
	txt(hdc, "Start  "+vkName(a.cfg.KeyStart)+"     •     Stop  "+vkName(a.cfg.KeyStop)+"     •     Refund  "+vkName(a.cfg.KeyRefund), 331, 603, 697, 26, colMuted, "small")
	txt(hdc, func() string {
		if a.focus {
			return "RUST FOCUSED  •  KEY INPUTS ACTIVE"
		}
		return "RUST NOT FOCUSED  •  KEY INPUTS PAUSED"
	}(), 307, 661, 742, 27, func() uint32 {
		if a.focus {
			return colGreen
		}
		return colOrange
	}(), "smallBold")
	txt(hdc, "Stopping does NOT pause already queued Rust crafts.", 307, 690, 756, 23, colFaint, "small")
}
func rectx(x int) int { return x }
func hotkeyRow(hdc uintptr, x, y, w int, title, desc, which string, key uint32) {
	card(hdc, x, y, w, 104)
	txt(hdc, title, x+20, y+13, w-165, 34, colText, "regularBold")
	txt(hdc, desc, x+20, y+48, w-165, 35, colMuted, "small")
	fillRound(hdc, rect(x+w-157, y+18, 135, 32), 10, colField)
	drawText(hdc, vkName(key), rect(x+w-157, y+18, 135, 32), colOrange, "smallBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	button(hdc, "key:"+which, "REMAP", x+w-157, y+60, 135, 30, false)
}
func drawHotkeys(hdc uintptr) {
	sectionTitle(hdc, "Custom hotkeys", "Click Remap, then press the new key on your keyboard.")
	txt(hdc, "APP CONTROLS", 306, 156, 347, 27, colOrange, "smallBold")
	hotkeyRow(hdc, 304, 190, 366, "Start / Pause", "Toggle the stacking loop", "start", a.cfg.KeyStart)
	hotkeyRow(hdc, 697, 190, 366, "Stop", "Stop sending inputs", "stop", a.cfg.KeyStop)
	hotkeyRow(hdc, 304, 310, 366, "Refund wood", "Cancel queued TCs in-game; no popup", "refund", a.cfg.KeyRefund)
	txt(hdc, "IN-GAME BIND KEYS", 306, 439, 500, 29, colOrange, "smallBold")
	hotkeyRow(hdc, 304, 473, 240, "Queue TC", "Craft from wood", "queue", a.cfg.KeyQueue)
	hotkeyRow(hdc, 565, 473, 240, "Refresh TC", "Reset TC timer", "refresh", a.cfg.KeyRefresh)
	hotkeyRow(hdc, 826, 473, 240, "Cancel TC", "Wood refund", "cancel", a.cfg.KeyCancel)
	fillRound(hdc, rect(306, 598, 754, 83), 12, colPanel2)
	txt(hdc, "REMEMBER", 322, 608, 150, 22, colOrange, "smallBold")
	drawText(hdc, "After changing the IN-GAME bind keys, open Rust setup and paste the updated F1 commands.", rect(324, 636, 704, 39), colMuted, "small", DT_WORDBREAK)
}
func drawKeyOverlay(hdc uintptr) {
	fillRound(hdc, rect(334, 237, 702, 250), 24, 0x25354A)
	fillRound(hdc, rect(351, 252, 668, 221), 22, colPanel)
	txt(hdc, "PRESS ANY SUPPORTED KEY", 409, 287, 556, 53, colText, "title")
	txt(hdc, "F2–F12 • Numpad • A–Z • 0–9 • Insert • Delete • Home • End", 409, 357, 572, 30, colMuted, "regular")
	txt(hdc, "Press ESC to cancel", 409, 403, 566, 27, colOrange, "smallBold")
}
func settingRow(hdc uintptr, x, y int, title, desc string, val int, unit, typ string) {
	card(hdc, x, y, 760, 110)
	txt(hdc, title, x+20, y+14, 425, 35, colText, "regularBold")
	txt(hdc, desc, x+20, y+52, 445, 43, colMuted, "small")
	button(hdc, typ+":minus", "−", x+493, y+35, 52, 44, false)
	fillRound(hdc, rect(x+556, y+35, 121, 44), 10, colField)
	drawText(hdc, fmt.Sprintf("%d %s", val, unit), rect(x+556, y+35, 121, 44), colOrange, "regularBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	button(hdc, typ+":plus", "+", x+688, y+35, 52, 44, false)
}
func drawSettings(hdc uintptr) {
	sectionTitle(hdc, "Automation settings", "Saved automatically to your local profile.")
	settingRow(hdc, 304, 172, "Queue attempt interval", "Try to queue another TC as wood becomes available.", a.cfg.QueueMs, "ms", "queueMs")
	settingRow(hdc, 304, 298, "Craft refresh interval", "How often to reset the active TC timer.", a.cfg.RefreshMs, "ms", "refreshMs")
	settingRow(hdc, 304, 424, "Refund attempts", "Number of in-game cancellation sweeps (TC batch sizes 1–10).", a.cfg.RefundTries, "tries", "refundTries")
	card(hdc, 304, 557, 760, 110)
	pill(hdc, "SAFETY", 328, 575, 94, true)
	txt(hdc, "Auto-pauses when Rust loses focus. No game memory access,", 444, 566, 600, 30, colMuted, "small")
	txt(hdc, "no cheat bypass, and no background game inputs.", 444, 594, 590, 30, colMuted, "small")
	txt(hdc, "TC crafting can finish after you stop or alt-tab.", 328, 628, 650, 25, colOrange, "smallBold")
}
func drawSetup(hdc uintptr) {
	sectionTitle(hdc, "Rust bind setup", "One-time setup inside Rust. No AutoHotkey installation needed.")
	card(hdc, 304, 168, 760, 108)
	txt(hdc, "01  OPEN RUST", 326, 182, 714, 27, colOrange, "smallBold")
	drawText(hdc, "Join a server, then press F1 to open the Rust console.", rect(326, 218, 690, 43), colText, "regular", DT_WORDBREAK)
	card(hdc, 304, 292, 760, 244)
	txt(hdc, "02  COPY EACH COMMAND", 326, 306, 527, 29, colOrange, "smallBold")
	lines := strings.Split(strings.ReplaceAll(rustBindCommands(), "\r\n", "\n"), "\n")
	for i, line := range lines {
		y := 345 + i*44
		display := line
		if len(display) > 68 {
			display = display[:66] + "..."
		}
		fillRound(hdc, rect(321, y, 603, 34), 8, colField)
		txt(hdc, display, 331, y+2, 581, 27, colMuted, "mono")
		button(hdc, fmt.Sprintf("copyline:%d", i), "COPY", 941, y, 98, 34, false)
	}
	card(hdc, 304, 551, 760, 126)
	txt(hdc, "03  PASTE, ENTER & SAVE", 327, 566, 700, 29, colOrange, "smallBold")
	drawText(hdc, "Paste each copied command in Rust's F1 console, press Enter, then copy the next one. The final writecfg saves your binds. Keep Num Lock ON for Numpad keys.", rect(327, 604, 704, 56), colText, "regular", DT_WORDBREAK)
}
func drawAbout(hdc uintptr) {
	sectionTitle(hdc, "About Rust TC Studio", "A lightweight companion app for ordinary Rust crafting binds.")
	card(hdc, 304, 174, 760, 132)
	txt(hdc, "RUST TC STUDIO  /  VERSION "+appVersion, 330, 187, 705, 36, colOrange, "cardTitle")
	txt(hdc, "Offline profile • Self-contained Windows executable • Local settings", 329, 226, 710, 36, colText, "regular")
	txt(hdc, "1000 WOOD", 329, 265, 205, 28, colGreen, "smallBold")
	txt(hdc, "PER TOOL CUPBOARD", 471, 265, 352, 28, colMuted, "smallBold")
	card(hdc, 304, 324, 760, 303)
	txt(hdc, "WHAT THIS APP CAN DO", 330, 345, 700, 38, colText, "cardTitle")
	info := []string{
		"• Queue TC crafting attempts using normal in-game craft commands.",
		"• Periodically refresh the crafting queue while Rust is active.",
		"• Cancel TC tasks in bursts when you choose to refund wood.",
		"• Remap hotkeys and adjust intervals, saved on your device.",
		"• Store a password-protected LOCAL profile; no server or network login.",
	}
	for i, s := range info {
		txt(hdc, s, 330, 392+i*37, 709, 30, colMuted, "regular")
	}
	txt(hdc, "LIMITATIONS", 329, 650, 200, 30, colOrange, "smallBold")
	drawText(hdc, "No inventory/wood detection. Attempt counters are not confirmed TC counts. Craft completion cannot be guaranteed prevented on accelerated servers or when unfocused. Follow your server rules.", rect(329, 680, 726, 48), colMuted, "small", DT_WORDBREAK)
}
func paint(hwnd uintptr) {
	var ps PAINTSTRUCT
	hdc, _, _ := BeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	mem, _, _ := CreateCompatibleDC.Call(hdc)
	bmp, _, _ := CreateCompatibleBitmap.Call(hdc, width, height)
	old, _, _ := SelectObject.Call(mem, bmp)
	v2DrawApp(mem)
	BitBlt.Call(hdc, 0, 0, width, height, mem, 0, 0, SRCCOPY)
	SelectObject.Call(mem, old)
	DeleteObject.Call(bmp)
	DeleteDC.Call(mem)
	EndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}
func handleClick(x, y int) {
	if a.pendingKey != "" {
		return
	}
	for i := len(a.hits) - 1; i >= 0; i-- {
		h := a.hits[i]
		if inside(h.r, x, y) {
			action(h.id)
			return
		}
	}
}
func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
func currentTradePlan() trader.Plan {
	return trader.Calculate(trader.Inputs{Verified: a.cfg.TraderVerified, FreeMainSlots: a.cfg.TraderFreeMain, FreeBackpackSlots: a.cfg.TraderFreeBackpack, FreeSpaceInMainStacks: a.cfg.TraderStackRoom, Scrap: a.cfg.TraderScrap, FragmentsPerTrade: 250, ScrapPerTrade: 25, FragmentStackSize: 1000})
}
func action(id string) {
	switch {
	case id == "login":
		login()
	case id == "rememberLogin":
		a.rememberLogin = !a.rememberLogin
		refreshUI()
	case id == "logout":
		signOut()
	case id == "clearLogs":
		a.logs = nil
		refreshUI()
	case strings.HasPrefix(id, "page:"):
		if a.page == "vending" {
			_ = commitAllTraderEdits()
		}
		next := strings.TrimPrefix(id, "page:")
		if next == "admin" && !a.isAdmin {
			return
		}
		a.page = next
		if next == "vending" {
			a.cfg.TraderVerified = false
		} // never reuse stale game state
		showEdits()
		refreshUI()
	case id == "issuePIN":
		issueOnlinePIN()
	case id == "revokePIN":
		revokeOnlinePIN()
	case id == "issuePlayer":
		issueAdminCode(license.RolePlayer)
	case id == "issueAdmin":
		issueAdminCode(license.RoleAdmin)
	case id == "copyAdminCode":
		if a.isAdmin && a.adminIssuedCode != "" {
			if copyClipboard(a.adminIssuedCode) {
				toast("Last issued code copied to clipboard.")
			}
		}
	case id == "saveFeed":
		feed := strings.TrimSpace(editValue(a.updateEdit))
		if err := updates.HTTPS(feed); err != nil {
			toast("Feed must be an HTTPS URL, such as your release update.json.")
			return
		}
		a.cfg.UpdateURL = feed
		a.updateAvailable = false
		a.lastUpdateCheck = time.Time{}
		a.updateStatus = "Feed saved. Checking for releases..."
		saveConfig()
		addLog("Signed update feed configured")
		if a.updateChecking {
			a.updateStatus = "Feed saved; previous check must finish first."
		} else {
			checkForUpdate(true)
		}
	case id == "sidebarCheckUpdate":
		a.page = "updates"
		showEdits()
		checkForUpdate(true)
		refreshUI()
	case id == "checkUpdate":
		checkForUpdate(true)
	case id == "installUpdate":
		startDownloadUpdate()
	case id == "trader:confirm":
		if err := commitAllTraderEdits(); err != nil {
			toast("Enter valid numbers for all four fields (" + err.Error() + ").")
			return
		}
		a.cfg.TraderVerified = !a.cfg.TraderVerified
		saveConfig()
		refreshUI()
	case id == "trader:reset":
		a.cfg.TraderVerified = false
		a.cfg.TraderFreeMain = 0
		a.cfg.TraderFreeBackpack = 0
		a.cfg.TraderStackRoom = 0
		a.cfg.TraderScrap = 0
		syncTraderEdits()
		saveConfig()
		refreshUI()
	case id == "start":
		startStack()
	case id == "stop":
		stopStack()
	case id == "refund":
		refund()
	case strings.HasPrefix(id, "key:"):
		a.pendingKey = strings.TrimPrefix(id, "key:")
		refreshUI()
	case strings.HasPrefix(id, "copyline:"):
		idx := 0
		_, err := fmt.Sscanf(strings.TrimPrefix(id, "copyline:"), "%d", &idx)
		lines := strings.Split(strings.ReplaceAll(rustBindCommands(), "\r\n", "\n"), "\n")
		if err == nil && idx >= 0 && idx < len(lines) {
			if copyClipboard(lines[idx]) {
				toast(fmt.Sprintf("Copied command %d of 4 — paste into Rust F1 and press Enter.", idx+1))
			} else {
				toast("Clipboard unavailable. Copy commands manually.")
			}
		}
	case strings.Contains(id, ":"):
		if a.page == "vending" {
			if err := commitAllTraderEdits(); err != nil {
				toast("Correct the numeric entry first (" + err.Error() + ").")
				return
			}
		}
		p := strings.SplitN(id, ":", 2)
		dir := 1
		if p[1] == "minus" {
			dir = -1
		}
		switch p[0] {
		case "traderMain":
			a.cfg.TraderFreeMain = clampInt(a.cfg.TraderFreeMain+dir, 0, 30)
			a.cfg.TraderVerified = false
		case "traderBag":
			a.cfg.TraderFreeBackpack = clampInt(a.cfg.TraderFreeBackpack+dir, 0, 40)
			a.cfg.TraderVerified = false
		case "traderScrap":
			a.cfg.TraderScrap = clampInt(a.cfg.TraderScrap+dir*25, 0, 500000)
			a.cfg.TraderVerified = false
		case "traderStack":
			a.cfg.TraderStackRoom = clampInt(a.cfg.TraderStackRoom+dir*250, 0, 30000)
			a.cfg.TraderVerified = false
		case "queueMs":
			a.cfg.QueueMs += dir * 100
			if a.cfg.QueueMs < 300 {
				a.cfg.QueueMs = 300
			}
			if a.cfg.QueueMs > 3000 {
				a.cfg.QueueMs = 3000
			}
		case "refreshMs":
			a.cfg.RefreshMs += dir * 200
			if a.cfg.RefreshMs < 1000 {
				a.cfg.RefreshMs = 1000
			}
			if a.cfg.RefreshMs > 5500 {
				a.cfg.RefreshMs = 5500
			}
		case "refundTries":
			a.cfg.RefundTries += dir * 5
			if a.cfg.RefundTries < 1 {
				a.cfg.RefundTries = 1
			}
			if a.cfg.RefundTries > 60 {
				a.cfg.RefundTries = 60
			}
		}
		saveConfig()
		refreshUI()
	}
}
func hotkey(id int) {
	if !a.authed {
		return
	}
	focused := rustForeground()
	fg, _, _ := GetForegroundWindow.Call()
	if !focused && fg != a.hwnd {
		return
	}
	switch id {
	case 1:
		startStack()
	case 2:
		stopStack()
	case 3:
		refund()
	}
}
func wndproc(hwnd uintptr, msg uintptr, wparam, lparam uintptr) uintptr {
	switch msg {
	case WM_CREATE:
		return 0
	case WM_CLOSE:
		stopStack()
		KillTimer.Call(hwnd, 1)
		unregisterHotkeys() // normal shutdown
		u32.NewProc("DestroyWindow").Call(hwnd)
		return 0
	case WM_DESTROY:
		PostQuitMessage.Call(0)
		return 0
	case WM_PAINT:
		paint(hwnd)
		return 0
	case WM_ERASEBKGND:
		return 1
	case WM_TIMER:
		tick()
		return 0
	case WM_HOTKEY:
		hotkey(int(wparam))
		return 0
	case WM_KEYDOWN:
		if a.pendingKey != "" {
			v := uint32(wparam)
			if v == VK_ESCAPE {
				a.pendingKey = ""
				refreshUI()
			} else {
				updateKey(a.pendingKey, v)
			}
			return 0
		}
		if !a.authed && uint32(wparam) == VK_RETURN {
			login()
			return 0
		}
	case WM_COMMAND:
		id := lo16(wparam)
		if id >= traderControlBase && id < traderControlBase+len(a.traderEdits) && !a.traderEditSyncing {
			i := id - traderControlBase
			switch hi16(wparam) {
			case EN_CHANGE:
				a.cfg.TraderVerified = false
				refreshUI()
			case EN_KILLFOCUS:
				// Keep incorrect text visible so it can be corrected; confirmation rejects it.
				if commitTraderEdit(i, true) == nil {
					refreshUI()
				}
			case EN_SETFOCUS:
				// Clicking a value selects it for one-step replacement by typing.
				SendMessage.Call(a.traderEdits[i], EM_SETSEL, 0, ^uintptr(0))
			}
			return 0
		}
	case WM_MOUSEMOVE:
		x, y := lo16(lparam), hi16(lparam)
		newHover := ""
		for _, h := range a.hits {
			if inside(h.r, x, y) {
				newHover = h.id
				break
			}
		}
		if newHover != a.hovered {
			a.hovered = newHover
			refreshUI()
		}
		return 0
	case WM_LBUTTONUP:
		handleClick(lo16(lparam), hi16(lparam))
		return 0
	case WM_CTLCOLOREDIT:
		SetTextColor.Call(wparam, color(colText))
		SetBkColor.Call(wparam, color(colField))
		return a.editBrush
	}
	r, _, _ := DefWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
func main() {
	// Win32 window ownership, WM_TIMER, hotkey registration and GetMessage must
	// all use the SAME OS thread. Without this, Go may migrate this goroutine,
	// leaving the window hung with "Not Responding" in Task Manager.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	loadConfig()
	loadUpdateFeed()
	loadAuthServer()
	// Unified public edition: launch directly into the stacker.
	// Existing custom hotkey/settings config stays intact.
	a.authed = true
	a.isAdmin = false
	a.ownerLocal = false
	a.cfg.User = "RustPilot"
	a.page = "stacker"
	initImageCache()
	SetProcessDPIAware.Call()
	inst, _, _ := GetModuleHandle.Call(0)
	cursor, _, _ := LoadCursor.Call(0, 32512)
	wc := WNDCLASSEX{Size: uint32(unsafe.Sizeof(WNDCLASSEX{})), Style: CS_VREDRAW | CS_HREDRAW, WndProc: syscall.NewCallback(wndproc), Instance: inst, Cursor: cursor, ClassName: p16(className)}
	if r, _, _ := RegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		panic("could not register window class")
	}
	wr := rect(0, 0, width, height)
	u32.NewProc("AdjustWindowRect").Call(uintptr(unsafe.Pointer(&wr)), WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX, 0)
	hwnd, _, _ := CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16(className))), uintptr(unsafe.Pointer(p16(appTitle+"  •  v"+appVersion))), WS_CAPTION|WS_SYSMENU|WS_MINIMIZEBOX, 110, 65, uintptr(wr.Right-wr.Left), uintptr(wr.Bottom-wr.Top), 0, 0, inst, 0)
	if hwnd == 0 {
		panic("could not create window")
	}
	a.hwnd = hwnd
	// Prefer a dark title bar. Unsupported Windows builds safely ignore this.
	var darkMode int32 = 1
	syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute").Call(hwnd, 20, uintptr(unsafe.Pointer(&darkMode)), 4)
	initAppIcon(hwnd)
	a.fonts["hero"] = createFont(53, 800, "Segoe UI")
	a.fonts["mini"] = createFont(11, 600, "Segoe UI")
	a.fonts["tiny"] = createFont(10, 500, "Segoe UI")
	a.fonts["title"] = createFont(33, 750, "Segoe UI")
	a.fonts["cardTitle"] = createFont(21, 700, "Segoe UI")
	a.fonts["stat"] = createFont(42, 750, "Segoe UI")
	a.fonts["logo"] = createFont(34, 800, "Segoe UI")
	a.fonts["logoSmall"] = createFont(20, 800, "Segoe UI")
	a.fonts["regularBold"] = createFont(16, 700, "Segoe UI")
	a.fonts["regular"] = createFont(16, 400, "Segoe UI")
	a.fonts["button"] = createFont(13, 700, "Segoe UI")
	a.fonts["smallBold"] = createFont(12, 700, "Segoe UI")
	a.fonts["small"] = createFont(12, 400, "Segoe UI")
	a.fonts["large"] = createFont(27, 750, "Segoe UI")
	a.fonts["mono"] = createFont(11, 500, "Consolas")
	a.fonts["navIcon"] = createFont(19, 700, "Segoe UI Symbol")
	a.editBrush, _, _ = CreateSolidBrush.Call(color(colField))
	a.userEdit, _, _ = CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16("EDIT"))), uintptr(unsafe.Pointer(p16(""))), WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL, 800, 404, 326, 46, hwnd, 111, inst, 0)
	a.passEdit, _, _ = CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16("EDIT"))), uintptr(unsafe.Pointer(p16(""))), WS_CHILD|WS_VISIBLE|WS_TABSTOP|ES_AUTOHSCROLL|ES_PASSWORD, 800, 468, 326, 48, hwnd, 112, inst, 0)
	a.adminNameEdit, _, _ = CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16("EDIT"))), uintptr(unsafe.Pointer(p16(""))), WS_CHILD|WS_TABSTOP|ES_AUTOHSCROLL, 297, 426, 548, 48, hwnd, 114, inst, 0)
	a.updateEdit, _, _ = CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16("EDIT"))), uintptr(unsafe.Pointer(p16(""))), WS_CHILD|WS_TABSTOP|ES_AUTOHSCROLL, 300, 452, 892, 43, hwnd, 113, inst, 0)
	SendMessage.Call(a.userEdit, WM_SETFONT, a.fonts["regular"], 1)
	SendMessage.Call(a.passEdit, WM_SETFONT, a.fonts["regular"], 1)
	SendMessage.Call(a.updateEdit, WM_SETFONT, a.fonts["regular"], 1)
	SendMessage.Call(a.adminNameEdit, WM_SETFONT, a.fonts["regular"], 1)
	for i := range a.traderEdits {
		edit, _, _ := CreateWindowEx.Call(0, uintptr(unsafe.Pointer(p16("EDIT"))), uintptr(unsafe.Pointer(p16(""))), WS_CHILD|WS_TABSTOP|ES_AUTOHSCROLL|ES_NUMBER|ES_CENTER, 563, uintptr(traderFieldsY[i]+4), 91, 23, hwnd, uintptr(traderControlBase+i), inst, 0)
		a.traderEdits[i] = edit
		if edit == 0 {
			panic("unable to create vending numeric input")
		}
		SendMessage.Call(edit, WM_SETFONT, a.fonts["regularBold"], 1)
		SendMessage.Call(edit, EM_SETLIMITTEXT, 6, 0)
	}
	syncTraderEdits()
	setEdit(a.updateEdit, a.cfg.UpdateURL)
	showEdits()
	if a.authed {
		registerHotkeys()
		SetFocus.Call(hwnd)
	} else {
		SetFocus.Call(a.passEdit)
	}
	ShowWindow.Call(hwnd, SW_SHOW)
	UpdateWindow.Call(hwnd)
	// Application has launched successfully. Only now remove exact-name
	// updater backups; keep all saved settings and unrelated files intact.
	if executable, err := os.Executable(); err == nil {
		if count, err := housekeeping.CleanupInstalled(executable, time.Now()); err == nil && count > 0 {
			addLog(fmt.Sprintf("Removed %d obsolete update backup(s)", count))
		}
	}
	if a.configPath != "" {
		housekeeping.CleanupHelpers(filepath.Dir(a.configPath), time.Now())
	}
	SetTimer.Call(hwnd, 1, 30, 0)
	if a.cfg.UpdateURL != "" {
		checkForUpdate(false)
	}
	var m MSG
	for {
		r, _, _ := GetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		TranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		DispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
	for _, font := range a.fonts {
		DeleteObject.Call(font)
	}
	DeleteObject.Call(a.editBrush)
}
