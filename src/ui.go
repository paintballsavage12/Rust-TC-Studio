//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	_ "image/png"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// Assets extracted from the user-approved purple UI concept.
//
//go:embed banner.png
var bannerPNG []byte

//go:embed app_icon.png
var iconPNG []byte

//go:embed background.png
var backgroundPNG []byte

const (
	cBlack      uint32 = 0x08040F
	cSide       uint32 = 0x100719
	cPanel      uint32 = 0x14091E
	cPanelHi    uint32 = 0x28103A
	cPanelHdr   uint32 = 0x320F46
	cBorder     uint32 = 0x5D2482
	cNeon       uint32 = 0xD535FF
	cLilac      uint32 = 0xB886E5
	cPurpleText uint32 = 0xEDBFFF
	cText       uint32 = 0xFBF7FF
	cSubtle     uint32 = 0xB2A2C1
	cGood       uint32 = 0x46EA72
	cWarn       uint32 = 0xFFB166
)

const bannerH = 167

type dibHeader struct {
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

var bannerBytes []byte
var bannerDIB dibHeader
var backgroundBytes []byte
var backgroundDIB dibHeader

func initImageCache() {
	im, _, err := image.Decode(bytes.NewReader(bannerPNG))
	if err != nil {
		return
	}
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return
	}
	bannerBytes = make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, blue, alpha := im.At(b.Min.X+x, b.Min.Y+y).RGBA()
			i := (y*w + x) * 4
			bannerBytes[i] = byte(blue >> 8)
			bannerBytes[i+1] = byte(g >> 8)
			bannerBytes[i+2] = byte(r >> 8)
			bannerBytes[i+3] = byte(alpha >> 8)
		}
	}
	bannerDIB = dibHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32, Compression: 0, SizeImage: uint32(w * h * 4)}
	backgroundBytes, backgroundDIB = rgbaDIB(backgroundPNG)
}

func rgbaDIB(src []byte) ([]byte, dibHeader) {
	im, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, dibHeader{}
	}
	r := im.Bounds()
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		return nil, dibHeader{}
	}
	buf := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			red, green, blue, alpha := im.At(r.Min.X+x, r.Min.Y+y).RGBA()
			i := (y*w + x) * 4
			buf[i], buf[i+1], buf[i+2], buf[i+3] = byte(blue>>8), byte(green>>8), byte(red>>8), byte(alpha>>8)
		}
	}
	return buf, dibHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32, SizeImage: uint32(w * h * 4)}
}

func v2Background(dc uintptr) {
	if len(backgroundBytes) == 0 {
		return
	}
	g32.NewProc("StretchDIBits").Call(dc, 0, 0, width, height, 0, 0, uintptr(backgroundDIB.Width), uintptr(-backgroundDIB.Height), uintptr(unsafe.Pointer(&backgroundBytes[0])), uintptr(unsafe.Pointer(&backgroundDIB)), 0, SRCCOPY)
}

func v2Banner(dc uintptr) {
	if len(bannerBytes) == 0 {
		fillRect(dc, rect(0, 0, width, bannerH), cPanel)
		return
	}
	g32.NewProc("StretchDIBits").Call(dc, 0, 0, width, bannerH, 0, 0, uintptr(bannerDIB.Width), uintptr(-bannerDIB.Height), uintptr(unsafe.Pointer(&bannerBytes[0])), uintptr(unsafe.Pointer(&bannerDIB)), 0, SRCCOPY)
	fillRect(dc, rect(0, bannerH-2, width, 2), cNeon)
}

func initAppIcon(hwnd uintptr) {
	// Prefer the embedded .rsrc icon group, so Explorer, Alt+Tab, and taskbar agree.
	inst, _, _ := GetModuleHandle.Call(0)
	ico, _, _ := u32.NewProc("LoadImageW").Call(inst, 1, 1, 64, 64, 0) // group 1, IMAGE_ICON
	if ico == 0 && len(iconPNG) > 0 {
		ico, _, _ = u32.NewProc("CreateIconFromResourceEx").Call(uintptr(unsafe.Pointer(&iconPNG[0])), uintptr(len(iconPNG)), 1, 0x00030000, 64, 64, 0)
	}
	if ico != 0 {
		SendMessage.Call(hwnd, 0x0080, 0, ico)
		SendMessage.Call(hwnd, 0x0080, 1, ico)
	}
}

func v2Card(dc uintptr, x, y, w, h int) {
	fillRound(dc, rect(x, y, w, h), 17, cPanel)
	outline(dc, rect(x, y, w, h), 17, cBorder)
}
func v2HeaderCard(dc uintptr, x, y, w, h int, title string) {
	v2Card(dc, x, y, w, h)
	fillRound(dc, rect(x+2, y+2, w-4, 39), 13, cPanelHdr)
	// cover the rounded lower edge of the header to make it square.
	fillRect(dc, rect(x+2, y+27, w-4, 15), cPanelHdr)
	txt(dc, title, x+17, y+6, w-34, 32, cText, "regularBold")
	fillRect(dc, rect(x+13, y+42, w-26, 1), cBorder)
}
func v2Button(dc uintptr, id, label string, x, y, w, h int, accent bool) {
	r := rect(x, y, w, h)
	if accent {
		// Multiple tight borders provide a subtle neon halo without unsupported alpha APIs.
		outline(dc, rect(x-3, y-3, w+6, h+6), 14, 0x4D125E)
		outline(dc, rect(x-1, y-1, w+2, h+2), 13, cNeon)
		fillRound(dc, r, 13, 0x4C1068)
		if a.hovered == id {
			fillRound(dc, r, 13, 0x74169B)
		}
		outline(dc, r, 13, cNeon)
	} else {
		fillRound(dc, r, 12, cPanelHi)
		if a.hovered == id {
			fillRound(dc, r, 12, 0x3D1B5B)
		}
		outline(dc, r, 12, cBorder)
	}
	fg := cText
	if !accent {
		fg = cPurpleText
	}
	drawText(dc, label, r, fg, "button", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	a.hits = append(a.hits, hit{r, id})
}
func v2SmallButton(dc uintptr, id, label string, x, y, w, h int) {
	v2Button(dc, id, label, x, y, w, h, false)
}
func v2Stat(dc uintptr, x, w int, label, value, detail string) {
	v2Card(dc, x, 198, w, 88)
	fillRound(dc, rect(x+15, 216, 5, 50), 4, cNeon)
	txt(dc, label, x+30, 207, w-46, 21, cLilac, "smallBold")
	txt(dc, value, x+30, 227, w-49, 36, cText, "large")
	if detail != "" {
		txt(dc, detail, x+30, 263, w-44, 16, cSubtle, "tiny")
	}
}
func v2Nav(dc uintptr) {
	fillRect(dc, rect(0, bannerH, 245, height-bannerH), cSide)
	fillRect(dc, rect(244, bannerH, 1, height-bannerH), cBorder)
	items := []struct{ id, label, sym string }{
		{"stacker", "TC Stacker", "▣"},
		{"vending", "Vending Trader", "▤"},
		{"hotkeys", "Hotkeys", "⌘"},
		{"settings", "Settings", "⚙"},
		{"setup", "Rust Setup", "</>"},
		{"updates", "Updates", "↻"},
		{"logs", "Logs", "▤"},
		{"about", "About", "ⓘ"},
	}

	// Reserve a dedicated action row below the menu for update checks.
	// Both the owner and player menus must fit above the account footer.
	step := 47
	for i, e := range items {
		y := 192 + i*step
		r := rect(16, y, 214, 43)
		if a.page == e.id {
			fillRound(dc, r, 12, 0x35104D)
			outline(dc, r, 12, cNeon)
			fillRound(dc, rect(17, y+8, 4, 32), 2, cNeon)
		}
		shade := cSubtle
		if a.page == e.id {
			shade = cText
		}
		txt(dc, e.sym, 31, y+7, 42, 32, cLilac, "navIcon")
		txt(dc, e.label, 82, y+6, 148, 34, shade, "regularBold")
		if e.id == "updates" && a.updateAvailable {
			fillRound(dc, rect(206, y+19, 10, 10), 8, cNeon)
		}
		a.hits = append(a.hits, hit{r, "page:" + e.id})
	}
	// One-click manual update check: opens Updates, displays status and checks
	// the configured signed-release feed. The owner controls and player tools
	// share the same action; neither needs special permissions to check.
	updateLabel := "CHECK FOR UPDATES"
	if a.updateChecking {
		updateLabel = "CHECKING FOR UPDATES..."
	} else if a.updateAvailable {
		updateLabel = "UPDATE AVAILABLE"
	}
	v2Button(dc, "sidebarCheckUpdate", updateLabel, 20, 631, 205, 42, a.updateAvailable)
	fillRect(dc, rect(23, 690, 198, 1), cBorder)
	txt(dc, "RUSTPILOT", 26, 696, 190, 23, cLilac, "tiny")
	txt(dc, "AUTOMATION SUITE", 25, 717, 201, 31, cText, "regularBold")
	state := "READY"
	sc := cGood
	if a.refunding {
		state = "REFUNDING"
		sc = cWarn
	} else if a.running && a.focus {
		state = "STACKING"
	} else if a.running {
		state = "WAITING FOR RUST"
		sc = cWarn
	}
	fillRound(dc, rect(28, 762, 11, 11), 8, sc)
	txt(dc, "STATUS: "+state, 49, 751, 186, 29, sc, "smallBold")
	txt(dc, "GITHUB UPDATES  •  v"+appVersion, 25, 786, 217, 18, cSubtle, "tiny")
}
func v2PageHeading(dc uintptr, title, subtitle string) {
	txt(dc, title, 275, 180, 880, 43, cText, "title")
	txt(dc, subtitle, 278, 220, 922, 29, cSubtle, "regular")
}
func v2DrawApp(dc uintptr) {
	a.hits = a.hits[:0]
	fillRect(dc, rect(0, 0, width, height), cBlack)
	v2Background(dc)
	v2Banner(dc)
	if !a.authed {
		v2Login(dc)
	} else {
		v2Nav(dc)
		switch a.page {
		case "stacker", "dashboard":
			v2Dashboard(dc)
		case "vending":
			v2Vending(dc)
		case "hotkeys":
			v2Hotkeys(dc)
		case "settings":
			v2Settings(dc)
		case "setup":
			v2Setup(dc)
		case "updates":
			v2Updates(dc)
		case "logs":
			v2Logs(dc)
		case "about":
			v2About(dc)
		default:
			v2Dashboard(dc)
		}
	}
	if a.pendingKey != "" {
		v2KeyOverlay(dc)
	}
	if a.notice != "" && time.Since(a.noticeAt) < 6*time.Second {
		fillRound(dc, rect(281, 773, 958, 34), 9, 0x3E1956)
		outline(dc, rect(281, 773, 958, 34), 9, cBorder)
		txt(dc, a.notice, 294, 776, 930, 27, cText, "smallBold")
	}
}
func v2Login(dc uintptr) {
	v2Card(dc, 150, 225, 1030, 500)
	fillRound(dc, rect(164, 239, 545, 471), 14, 0x180B2A)
	fillRound(dc, rect(206, 304, 78, 78), 14, 0x5B197A)
	drawText(dc, "TC", rect(207, 304, 76, 76), cText, "logo", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	txt(dc, "RUST TC STUDIO", 205, 404, 475, 59, cText, "title")
	txt(dc, "OWNER-CONTROLLED ACCESS", 205, 467, 470, 32, cNeon, "cardTitle")
	txt(dc, "Your username is assigned by the owner.", 205, 530, 478, 29, cSubtle, "regular")
	txt(dc, "No account creation or password needed.", 205, 568, 474, 29, cSubtle, "regular")
	txt(dc, "ACCESS CODE LOGIN", 768, 292, 400, 55, cText, "title")
	txt(dc, "ENTER A 6-DIGIT PIN OR SIGNED CODE", 799, 399, 342, 27, cLilac, "smallBold")
	txt(dc, "Players: enter your 6-digit PIN (online).", 799, 427, 365, 29, cSubtle, "small")
	fillRound(dc, rect(794, 463, 341, 58), 9, cPanelHi)
	checkboxLabel := "☐  KEEP ME LOGGED IN ON THIS PC"
	if a.rememberLogin {
		checkboxLabel = "☑  KEEP ME LOGGED IN ON THIS PC"
	}
	v2Button(dc, "rememberLogin", checkboxLabel, 799, 532, 325, 29, false)
	loginLabel := "SIGN IN"
	if a.authPending {
		loginLabel = "VERIFYING SECURE LOGIN..."
	}
	v2Button(dc, "login", loginLabel, 799, 587, 325, 48, true)
	txt(dc, "Owner: use your PIN in the private installation.", 796, 652, 382, 27, cSubtle, "small")
	statusNote := "Offline signed codes continue to work."
	if a.cfg.AuthURL == "" {
		statusNote = "6-digit PIN service not connected yet."
	}
	if a.authNotice != "" {
		statusNote = a.authNotice
	}
	txt(dc, statusNote, 776, 676, 394, 24, cLilac, "tiny")
}
func v2Dashboard(dc uintptr) {
	// Values are deliberately truthful. Rust inventory and queue counts are not exposed.
	v2Stat(dc, 270, 308, "WOOD IN GAME", "NOT READABLE", "No inventory inspection")
	v2Stat(dc, 594, 307, "QUEUE REQUESTS", fmt.Sprint(a.queueAttempts), "Attempts; not confirmed TCs")
	v2Stat(dc, 917, 333, "REFRESH INTERVAL", fmt.Sprintf("%.1f s", float64(a.cfg.RefreshMs)/1000), "Adjust in Settings")
	runLabel := "START STACKING  (" + vkName(a.cfg.KeyStart) + ")"
	if a.running {
		runLabel = "PAUSE STACKING  (" + vkName(a.cfg.KeyStart) + ")"
	}
	v2Button(dc, "start", "▶  "+runLabel, 291, 316, 424, 69, true)
	v2Button(dc, "stop", "■  STOP  ("+vkName(a.cfg.KeyStop)+")", 747, 319, 208, 63, false)
	v2Button(dc, "refund", "⟳  REFUND  ("+vkName(a.cfg.KeyRefund)+")", 975, 319, 264, 63, false)
	v2HeaderCard(dc, 270, 414, 475, 202, "⌘   APP HOTKEYS")
	v2HeaderCard(dc, 763, 414, 487, 202, "</>   IN-GAME BIND KEYS")
	rows := []struct{ label, key, id string }{
		{"Start / Pause Stacking", vkName(a.cfg.KeyStart), "start"},
		{"Stop Automation", vkName(a.cfg.KeyStop), "stop"},
		{"Refund Wood", vkName(a.cfg.KeyRefund), "refund"},
	}
	for i, r := range rows {
		y := 462 + i*47
		txt(dc, r.label, 289, y, 245, 32, cText, "smallBold")
		fillRound(dc, rect(552, y+2, 105, 29), 8, cBlack)
		drawText(dc, r.key, rect(553, y+2, 103, 29), cPurpleText, "smallBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		v2SmallButton(dc, "key:"+r.id, "EDIT", 671, y+2, 57, 29)
	}
	binds := []struct {
		label, id string
		key       uint32
	}{
		{"Queue TC (Craft)", "queue", a.cfg.KeyQueue},
		{"Refresh Queue", "refresh", a.cfg.KeyRefresh},
		{"Cancel TC (Refund)", "cancel", a.cfg.KeyCancel},
	}
	for i, r := range binds {
		y := 462 + i*47
		txt(dc, r.label, 780, y, 231, 32, cText, "smallBold")
		fillRound(dc, rect(1022, y+2, 139, 29), 8, cBlack)
		drawText(dc, vkName(r.key), rect(1022, y+2, 139, 29), cPurpleText, "smallBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
		v2SmallButton(dc, "key:"+r.id, "EDIT", 1174, y+2, 58, 29)
	}
	v2Card(dc, 270, 629, 980, 49)
	state := "READY  •  Start stacking when Rust is focused"
	col := cSubtle
	if a.refunding {
		state = "REFUNDING  •  Check your inventory and nearby ground"
		col = cWarn
	} else if a.running && a.focus {
		state = "WINDOWS INPUT ACTIVE  •  Rust detected (check queue in game)"
		col = cGood
	} else if a.running {
		state = "NO INPUT SENT  •  Rust not detected as foreground"
		col = cWarn
	}
	if a.lastInputProblem != "" {
		state = "INPUT ERROR  •  " + a.lastInputProblem
		col = cWarn
	}
	fillRound(dc, rect(287, 647, 11, 11), 8, col)
	txt(dc, state, 312, 638, 570, 31, col, "smallBold")
	txt(dc, "Foreground: "+a.foregroundDesc, 885, 638, 337, 31, cLilac, "tiny")
	v2Card(dc, 270, 690, 563, 111)
	v2Card(dc, 847, 690, 403, 111)
	txt(dc, "SETUP & USAGE GUIDE", 286, 697, 540, 23, cPurpleText, "smallBold")
	txt(dc, "1. Copy Rust F1 binds   2. Set hotkeys", 288, 725, 520, 21, cText, "small")
	txt(dc, "3. Join a server   4. Start stacking", 288, 749, 520, 23, cText, "small")
	txt(dc, "Stopping inputs does not pause crafting.", 288, 776, 525, 17, cWarn, "tiny")
	txt(dc, "RUST F1 COMMANDS", 862, 697, 367, 23, cPurpleText, "smallBold")
	txt(dc, "4 console commands • Copy individually", 863, 724, 360, 22, cSubtle, "small")
	v2Button(dc, "page:setup", "OPEN RUST SETUP", 866, 751, 362, 35, false)
}
func v2TraderStepper(dc uintptr, label, kind string, value, y int) {
	txt(dc, label, 300, y, 194, 32, cText, "smallBold")
	v2SmallButton(dc, kind+":minus", "−", 513, y, 41, 31)
	fillRound(dc, rect(562, y, 93, 31), 8, cBlack)
	drawText(dc, fmt.Sprint(value), rect(563, y, 92, 31), cPurpleText, "regularBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	v2SmallButton(dc, kind+":plus", "+", 662, y, 40, 31)
}
func v2Vending(dc uintptr) {
	v2PageHeading(dc, "Outpost Vending Trader", "Inventory-space purchase planner  |  v"+appVersion)
	v2Card(dc, 274, 263, 951, 103)
	txt(dc, "TRADE PLANNER — MANUAL INVENTORY CHECK", 295, 274, 900, 32, cPurpleText, "cardTitle")
	drawText(dc, "RustPilot does not read Rust inventory yet. Confirm the current slot counts, partial stack room and scrap yourself. This screen calculates a conservative purchase batch; it never clicks the vending machine.", rect(296, 310, 890, 50), cSubtle, "small", DT_WORDBREAK)

	v2HeaderCard(dc, 274, 386, 456, 301, "YOUR CURRENT FREE SPACE")
	v2TraderStepper(dc, "Free main slots", "traderMain", a.cfg.TraderFreeMain, 440)
	v2TraderStepper(dc, "Free backpack slots", "traderBag", a.cfg.TraderFreeBackpack, 492)
	v2TraderStepper(dc, "Room in partial stacks", "traderStack", a.cfg.TraderStackRoom, 544)
	v2TraderStepper(dc, "Available scrap", "traderScrap", a.cfg.TraderScrap, 596)
	txt(dc, "Changes reset confirmation. Base assumption: 250 frags for 25 scrap, 1,000 per slot.", 293, 646, 418, 35, cSubtle, "tiny")

	v2HeaderCard(dc, 747, 386, 478, 301, "PLANNED SAFE PURCHASE")
	plan := currentTradePlan()
	if !a.cfg.TraderVerified {
		txt(dc, "UNVERIFIED INPUT", 768, 443, 434, 36, cWarn, "cardTitle")
		txt(dc, "Check Rust inventory and confirm the values below.", 767, 485, 428, 55, cSubtle, "regular")
	} else if plan.Ready {
		txt(dc, fmt.Sprintf("UP TO  x%d TRADES", plan.MaxBatch), 768, 440, 432, 43, cGood, "cardTitle")
		txt(dc, fmt.Sprintf("Est. metal fragments:  %d", plan.Fragments), 767, 505, 436, 30, cText, "regularBold")
		txt(dc, fmt.Sprintf("Est. scrap cost:  %d", plan.ScrapCost), 767, 541, 436, 30, cText, "regularBold")
		txt(dc, fmt.Sprintf("Backpack capacity:  %d frags", plan.BackpackCapacity), 767, 576, 436, 29, cSubtle, "smallBold")
	} else {
		txt(dc, "PAUSE — DO NOT BUY", 768, 440, 432, 43, cWarn, "cardTitle")
		drawText(dc, plan.Reason, rect(768, 498, 420, 75), cSubtle, "regular", DT_WORDBREAK)
	}
	txt(dc, "Estimate only • actual stock/prices and transfers are not detected.", 767, 640, 437, 31, cSubtle, "tiny")

	v2Card(dc, 274, 704, 951, 103)
	txt(dc, "AUTO TRADING & BACKPACK TRANSFER: NOT ENABLED", 295, 712, 875, 28, cWarn, "regularBold")
	v2Button(dc, "trader:confirm", "CONFIRM SPACE", 296, 753, 214, 37, a.cfg.TraderVerified)
	v2Button(dc, "trader:reset", "RESET", 523, 753, 124, 37, false)
	txt(dc, "Never trust old slot counts; reconfirm after each trade.", 673, 751, 514, 38, cSubtle, "small")
}

func v2KeyCard(dc uintptr, x, y, w, h int, title, description, id string, key uint32) {
	v2Card(dc, x, y, w, h)
	txt(dc, title, x+18, y+14, w-36, 33, cText, "regularBold")
	drawText(dc, description, rect(x+18, y+53, w-34, 45), cSubtle, "small", DT_WORDBREAK)
	fillRound(dc, rect(x+19, y+h-51, 127, 34), 9, cBlack)
	drawText(dc, vkName(key), rect(x+19, y+h-51, 127, 34), cPurpleText, "regularBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	v2Button(dc, "key:"+id, "REMAP", x+160, y+h-51, w-181, 34, false)
}
func v2Hotkeys(dc uintptr) {
	v2PageHeading(dc, "Custom hotkeys", "Click Remap, then press a supported key. Changes are saved automatically.")
	txt(dc, "APP CONTROLS", 281, 254, 600, 25, cNeon, "smallBold")
	v2KeyCard(dc, 274, 286, 303, 150, "Start / Pause", "Toggle TC stacking inputs", "start", a.cfg.KeyStart)
	v2KeyCard(dc, 599, 286, 302, 150, "Stop", "Stop sending inputs to Rust", "stop", a.cfg.KeyStop)
	v2KeyCard(dc, 923, 286, 305, 150, "Refund Wood", "Attempt to cancel queued TCs", "refund", a.cfg.KeyRefund)
	txt(dc, "IN-GAME BIND KEYS", 281, 454, 620, 28, cNeon, "smallBold")
	v2KeyCard(dc, 274, 485, 303, 158, "Queue TC", "Request TC crafting in Rust", "queue", a.cfg.KeyQueue)
	v2KeyCard(dc, 599, 485, 302, 158, "Refresh TC", "Request cancellation/re-queue", "refresh", a.cfg.KeyRefresh)
	v2KeyCard(dc, 923, 485, 305, 158, "Cancel TC", "Attempt wood refund", "cancel", a.cfg.KeyCancel)
	v2Card(dc, 274, 664, 955, 120)
	txt(dc, "REMEMBER", 294, 679, 180, 28, cPurpleText, "regularBold")
	drawText(dc, "Changing an IN-GAME key requires updating the Rust F1 bind commands. Changing a hotkey only updates this application.", rect(295, 707, 884, 48), cSubtle, "regular", DT_WORDBREAK)
	v2Button(dc, "page:setup", "OPEN RUST SETUP", 983, 739, 225, 36, false)
}
func v2Setting(dc uintptr, y int, title, detail string, value int, unit, typ string) {
	v2Card(dc, 274, y, 954, 128)
	txt(dc, title, 297, y+17, 615, 32, cText, "cardTitle")
	drawText(dc, detail, rect(298, y+62, 584, 50), cSubtle, "regular", DT_WORDBREAK)
	v2Button(dc, typ+":minus", "−", 919, y+40, 51, 46, false)
	fillRound(dc, rect(984, y+40, 148, 46), 9, cBlack)
	drawText(dc, fmt.Sprintf("%d %s", value, unit), rect(984, y+40, 148, 46), cPurpleText, "regularBold", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	v2Button(dc, typ+":plus", "+", 1145, y+40, 54, 46, false)
}
func v2Settings(dc uintptr) {
	v2PageHeading(dc, "Automation settings", "Customize the request intervals and refund limit for your local profile.")
	v2Setting(dc, 267, "Queue attempt interval", "How often another TC craft request is sent if Rust remains focused.", a.cfg.QueueMs, "ms", "queueMs")
	v2Setting(dc, 410, "Craft refresh interval", "Attempts to refresh the TC crafting timer. Actual behavior depends on Rust.", a.cfg.RefreshMs, "ms", "refreshMs")
	v2Setting(dc, 553, "Refund attempts", "How many cancellation requests are sent after you confirm a refund.", a.cfg.RefundTries, "tries", "refundTries")
	v2Card(dc, 274, 697, 954, 98)
	txt(dc, "IMPORTANT", 296, 707, 164, 25, cWarn, "smallBold")
	drawText(dc, "Rust crafting does not stop when you alt-tab or disable the app. This program does not read your wood count, scan memory, or bypass anti-cheat protections.", rect(295, 737, 896, 53), cSubtle, "small", DT_WORDBREAK)
}
func v2Setup(dc uintptr) {
	v2PageHeading(dc, "Rust bind setup", "Copy each command separately into the Rust F1 console, then press Enter.")
	v2Card(dc, 274, 265, 954, 90)
	txt(dc, "STEP 1  •  OPEN RUST", 296, 278, 560, 30, cPurpleText, "regularBold")
	txt(dc, "Join a server, open F1, and paste the commands below one at a time.", 296, 311, 889, 29, cSubtle, "regular")
	v2Card(dc, 274, 369, 954, 333)
	txt(dc, "STEP 2  •  INSTALL IN-GAME BINDS", 296, 380, 788, 30, cPurpleText, "regularBold")
	lines := strings.Split(strings.ReplaceAll(rustBindCommands(), "\r\n", "\n"), "\n")
	for i, line := range lines {
		y := 420 + i*67
		fillRound(dc, rect(290, y, 772, 56), 9, cBlack)
		drawText(dc, line, rect(306, y+5, 736, 49), cText, "mono", DT_WORDBREAK)
		v2Button(dc, fmt.Sprintf("copyline:%d", i), "COPY", 1081, y+8, 124, 39, false)
	}
	v2Card(dc, 274, 715, 954, 86)
	txt(dc, "STEP 3  •  TEST WITH A SMALL AMOUNT OF WOOD", 295, 725, 870, 27, cPurpleText, "regularBold")
	txt(dc, "Try F6 in Rust. Watch the craft queue. Press F7 to stop inputs, and F8 to cancel.", 295, 756, 902, 28, cSubtle, "small")
}

func v2Updates(dc uintptr) {
	v2PageHeading(dc, "Updates & releases", "Check GitHub for verified updates without downloading the app again.")
	v2Card(dc, 274, 266, 954, 126)
	txt(dc, "CURRENT VERSION", 298, 279, 405, 27, cLilac, "smallBold")
	txt(dc, "v"+appVersion, 299, 307, 395, 56, cText, "title")
	txt(dc, "SIGNED RELEASE STATUS", 724, 279, 435, 27, cLilac, "smallBold")
	status := a.updateStatus
	if status == "" {
		status = "Not checked yet"
	}
	if a.updateChecking {
		status = "Checking for a signed update..."
	}
	txt(dc, status, 724, 313, 480, 34, func() uint32 {
		if a.updateAvailable {
			return cGood
		}
		return cPurpleText
	}(), "regularBold")
	if a.updateAvailable {
		txt(dc, "Latest: v"+a.updateManifest.Version, 724, 352, 458, 26, cNeon, "smallBold")
	}
	v2Card(dc, 274, 407, 954, 156)
	txt(dc, "UPDATE FEED URL", 297, 414, 612, 29, cPurpleText, "regularBold")
	txt(dc, "Paste the HTTPS update.json link supplied by the owner. Save it once.", 298, 430, 900, 20, cSubtle, "tiny")
	fillRound(dc, rect(294, 450, 907, 49), 9, cPanelHi)
	v2Button(dc, "saveFeed", "SAVE FEED & CHECK", 957, 508, 239, 39, false)
	txt(dc, "Release signatures and download checksums are verified before installing.", 298, 514, 634, 35, cSubtle, "small")
	v2Card(dc, 274, 581, 954, 211)
	txt(dc, "INSTALL VERIFIED RELEASE", 296, 595, 805, 31, cPurpleText, "cardTitle")
	message := "Updates are checked on startup and every six hours while the app is open."
	if a.updateAvailable {
		message = "Version " + a.updateManifest.Version + " is ready. Close any game session before upgrading."
	}
	if a.updateDownloading {
		message = "Downloading now; the file is checked before your existing EXE is touched."
	}
	txt(dc, message, 298, 632, 891, 28, cSubtle, "small")
	if a.updateError != "" {
		drawText(dc, "Check detail: "+a.updateError, rect(299, 662, 885, 37), cWarn, "small", DT_WORDBREAK)
	}
	v2Button(dc, "checkUpdate", "CHECK NOW", 298, 711, 253, 51, false)
	if a.updateAvailable && !a.updateDownloading && !a.updateChecking {
		v2Button(dc, "installUpdate", "DOWNLOAD, INSTALL & RESTART", 581, 711, 608, 51, true)
	} else {
		fillRound(dc, rect(581, 711, 608, 51), 10, cPanelHi)
		label := "NO NEW UPDATE AVAILABLE"
		if a.updateDownloading {
			label = "VERIFYING DOWNLOADED RELEASE..."
		} else if a.updateChecking {
			label = "CHECKING FOR RELEASE..."
		} else if a.cfg.UpdateURL == "" {
			label = "SET A FEED URL FIRST"
		}
		drawText(dc, label, rect(581, 711, 608, 51), cSubtle, "button", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	}
}

func v2Admin(dc uintptr) {
	v2PageHeading(dc, "Owner administration", "Generate player PINs or legacy signed access codes. Manage PINs on your private server.")
	v2Card(dc, 274, 265, 954, 105)
	txt(dc, "Owner:Unknown", 297, 277, 608, 35, cText, "cardTitle")
	label := "VERIFIED ADMIN ACCESS"
	if a.ownerLocal {
		label = "LOCAL OWNER ACCESS"
	}
	txt(dc, label, 298, 314, 580, 27, cPurpleText, "smallBold")
	if !hasPrivateOwnerKey() {
		txt(dc, "Private signing key unavailable on this installation.", 670, 309, 526, 42, cWarn, "small")
	} else {
		txt(dc, "PRIVATE OWNER KEY AVAILABLE", 831, 317, 370, 28, cGood, "smallBold")
	}
	v2Card(dc, 274, 382, 954, 247)
	txt(dc, "ASSIGN A PLAYER USERNAME", 298, 389, 585, 31, cPurpleText, "regularBold")
	fillRound(dc, rect(292, 424, 562, 58), 12, cPanelHi)
	labelPIN := "GENERATE 6-DIGIT PLAYER PIN"
	if a.cfg.AuthURL == "" {
		labelPIN = "PIN SERVER REQUIRED — USE LEGACY CODE"
	}
	if a.adminPINPending {
		labelPIN = "CONTACTING AUTHENTICATION SERVER..."
	}
	v2Button(dc, "issuePIN", labelPIN, 299, 489, 574, 45, true)
	v2Button(dc, "revokePIN", "REVOKE PLAYER PIN", 899, 489, 288, 45, false)
	v2Button(dc, "issuePlayer", "LEGACY SIGNED PLAYER CODE", 299, 545, 416, 43, false)
	v2Button(dc, "issueAdmin", "LEGACY SIGNED ADMIN CODE", 739, 545, 447, 43, false)
	detail := "6-digit PINs require an HTTPS authentication server and a private owner API token."
	if a.cfg.AuthURL != "" {
		detail = "Auth server: " + a.cfg.AuthURL
	}
	drawText(dc, detail, rect(298, 594, 890, 32), cSubtle, "small", DT_WORDBREAK)
	v2Card(dc, 274, 645, 954, 156)
	txt(dc, "LAST ISSUED ACCESS", 296, 652, 688, 30, cPurpleText, "regularBold")
	label = "No code issued in this session."
	if a.adminIssuedCode != "" {
		label = "New access generated and copied. Send it privately."
	}
	txt(dc, label, 297, 684, 778, 29, cText, "regular")
	if a.adminMessage != "" {
		drawText(dc, a.adminMessage, rect(297, 713, 880, 30), cLilac, "small", DT_SINGLELINE)
	}
	if a.adminIssuedCode != "" {
		v2Button(dc, "copyAdminCode", "COPY LAST PIN/CODE", 890, 745, 292, 42, false)
	}
	txt(dc, "Never upload signing keys, owner API tokens, or login data to public GitHub.", 295, 775, 883, 21, cSubtle, "tiny")
}

func v2Profile(dc uintptr) {
	v2PageHeading(dc, "Your access profile", "This username is assigned by the owner who issued your login code.")
	v2Card(dc, 274, 267, 954, 252)
	fillRound(dc, rect(302, 296, 80, 80), 18, 0x4D1672)
	drawText(dc, "TC", rect(302, 296, 80, 80), cText, "logo", DT_CENTER|DT_VCENTER|DT_SINGLELINE)
	txt(dc, a.cfg.User, 405, 291, 705, 52, cText, "title")
	profileRole := "VERIFIED PLAYER ACCESS  •  OFFLINE"
	if a.onlinePIN {
		profileRole = "VERIFIED PLAYER ACCESS  •  ONLINE PIN"
	}
	if a.isAdmin {
		profileRole = "VERIFIED ADMIN ACCESS  •  OFFLINE"
	}
	txt(dc, profileRole, 405, 348, 621, 30, cLilac, "smallBold")
	loginNote := "Login uses an owner-issued signed access code."
	if a.onlinePIN {
		loginNote = "Login verified by the private PIN service; revoked accounts cannot reconnect."
	}
	if a.ownerLocal {
		loginNote = "Private owner installation: local PIN + owner signing key."
	}
	txt(dc, loginNote, 303, 405, 885, 29, cSubtle, "regular")
	txt(dc, "Your hotkey and timing preferences are saved on this computer.", 303, 452, 894, 25, cSubtle, "small")
	v2Button(dc, "logout", "SIGN OUT", 304, 553, 280, 53, true)
	v2Button(dc, "page:hotkeys", "EDIT HOTKEYS", 606, 553, 274, 53, false)
	v2Button(dc, "page:settings", "SETTINGS", 902, 553, 294, 53, false)
	v2Card(dc, 274, 647, 954, 135)
	txt(dc, "ACCESS DETAILS", 295, 663, 620, 33, cPurpleText, "regularBold")
	drawText(dc, "Online six-digit PINs can be revoked on the private auth service. Offline signed codes remain valid unless separately changed. Keep your sign-in secret.", rect(295, 707, 889, 57), cSubtle, "regular", DT_WORDBREAK)
}
func v2Logs(dc uintptr) {
	v2PageHeading(dc, "Activity logs", "Local session activity. Request attempts do not confirm successful in-game crafts.")
	v2Card(dc, 274, 267, 954, 518)
	txt(dc, "RECENT ACTIVITY", 295, 277, 684, 34, cPurpleText, "cardTitle")
	v2Button(dc, "clearLogs", "CLEAR LOG", 1057, 278, 142, 36, false)
	fillRect(dc, rect(295, 322, 902, 1), cBorder)
	if len(a.logs) == 0 {
		txt(dc, "No activity yet. Start a session to see status events.", 300, 355, 880, 40, cSubtle, "regular")
	}
	for i, e := range a.logs {
		if i >= 10 {
			break
		}
		y := 334 + i*43
		if i%2 == 0 {
			fillRound(dc, rect(291, y, 910, 40), 8, 0x241233)
		}
		txt(dc, e.at, 308, y+4, 113, 31, cLilac, "smallBold")
		txt(dc, e.msg, 432, y+4, 757, 31, cText, "regular")
	}
	txt(dc, "Session-only messages are cleared when you close the application.", 296, 749, 890, 25, cSubtle, "small")
}
func v2About(dc uintptr) {
	v2PageHeading(dc, "About RustPilot", "A purple-themed Windows companion for Rust gameplay utilities.")
	v2Card(dc, 274, 266, 954, 183)
	txt(dc, "RUSTPILOT", 300, 284, 902, 52, cText, "title")
	txt(dc, "STACK. TRADE. AUTOMATE.  /  VERSION "+appVersion, 300, 341, 871, 30, cPurpleText, "cardTitle")
	txt(dc, "Written in Go  •  Native Win32  •  Offline  •  Windows 10/11 64-bit", 300, 395, 897, 26, cSubtle, "regular")
	v2Card(dc, 274, 465, 954, 310)
	txt(dc, "FEATURES & LIMITATIONS", 300, 483, 875, 39, cPurpleText, "cardTitle")
	sentences := []string{
		"• Configurable app controls and in-game Rust binds",
		"• Queue, refresh, and cancellation requests while Rust has focus",
		"• TC Stacker is available; Vending Trader is a disabled preview",
		"• Does not read inventory or know the actual queued TC count",
		"• Signed GitHub updater is retained from the prior release",
		"• Stopping inputs does not pause existing crafting tasks",
	}
	for i, s := range sentences {
		txt(dc, s, 300, 535+i*36, 880, 31, cSubtle, "regular")
	}
}
func v2KeyOverlay(dc uintptr) {
	fillRound(dc, rect(338, 284, 781, 255), 24, 0x3C155A)
	fillRound(dc, rect(350, 296, 757, 231), 24, cPanel)
	outline(dc, rect(350, 296, 757, 231), 24, cNeon)
	txt(dc, "PRESS A NEW KEY", 413, 327, 630, 58, cText, "title")
	txt(dc, "F2–F12  •  Numpad  •  A–Z  •  0–9  •  Insert / Delete / Home / End", 413, 406, 663, 35, cSubtle, "regular")
	txt(dc, "ESC to cancel", 413, 466, 561, 26, cPurpleText, "regularBold")
}

// Ensure the Rust console still receives the complete command. No text is silently omitted.
var _ = syscall.StringToUTF16
