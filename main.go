package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"clipnote.local/app/history"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	imgdraw "golang.org/x/image/draw"
)

type desktop interface {
	front() int32
	count() int64
	read() (string, bool)
	canPaste() bool
	askPaste() bool
	paste(int32, string) error
}

// Optional platform metadata; the current frontmost app is only a hint
// because the pasteboard does not identify the process that wrote its contents.
type appMetadata interface {
	frontInfo() (pid int32, name, bundleID string)
	appIconPNG(bundleID string) []byte
}

func appIcons(provider appMetadata) func(string) *ui.Bitmap {
	icons := make(map[string]*ui.Bitmap)
	var order []string
	return func(id string) *ui.Bitmap {
		if bitmap, ok := icons[id]; ok {
			return bitmap
		}
		var bitmap *ui.Bitmap
		if raw := provider.appIconPNG(id); len(raw) > 0 {
			if original, err := png.Decode(bytes.NewReader(raw)); err == nil {
				thumb := image.NewRGBA(image.Rect(0, 0, 48, 48))
				src := original.Bounds()
				if src.Dx() > 0 && src.Dy() > 0 {
					side := max(src.Dx(), src.Dy())
					w, h := max(1, 48*src.Dx()/side), max(1, 48*src.Dy()/side)
					x, y := (48-w)/2, (48-h)/2
					imgdraw.ApproxBiLinear.Scale(thumb, image.Rect(x, y, x+w, y+h), original, src, draw.Src, nil)
					bitmap = ui.NewBitmap(thumb)
				}
			}
		}
		if len(order) == 64 {
			delete(icons, order[0])
			order = order[1:]
		}
		icons[id] = bitmap // Cache missing apps too; older records can outlive apps.
		order = append(order, id)
		return bitmap
	}
}

type app struct {
	d             desktop
	win           *mygo.Window
	tray          *mygo.Tray
	panel         *panel
	lastCount     int64
	target        int32
	busy          bool
	quitting      bool
	keepOpenUntil time.Time
	shortcutErr   string
	stopWatch     func()
	stopTimer     chan struct{}
}

func main() {
	dir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	mygo.App.SetName("ClipNote")
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	a := &app{stopTimer: make(chan struct{})}
	mygo.App.WhenReady(func() {
		a.d, err = newDesktop()
		if err != nil {
			log.Print(err)
			mygo.App.Quit()
			return
		}
		store, e := history.Open(filepath.Join(dir, "ClipNote", "history.json"))
		if e != nil {
			log.Printf("ClipNote history: %v", e)
			mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
			w := mygo.NewWindow(mygo.WindowOptions{Title: "ClipNote", Width: 560, Height: 510, Content: errorView})
			w.Show()
			return
		}
		a.ready(store)
	})
	mygo.App.OnWindowAllClosed(func() {
		if a.panel == nil {
			mygo.App.Quit()
		}
	})
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.win != nil {
			a.show()
		}
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnQuitCanceled(func() { a.quitting = false })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.stopWatch != nil {
			a.stopWatch()
		}
		close(a.stopTimer)
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
func (a *app) ready(s *history.Store) {
	a.panel = &panel{store: s, pick: a.pick, close: a.hide, permission: func() {
		a.keepOpenUntil = time.Now().Add(500 * time.Millisecond)
		a.panel.allowed = a.d.askPaste()
		if !a.panel.allowed {
			a.panel.notice = "请在「系统设置 → 隐私与安全性 → 辅助功能」中启用 ClipNote。"
		}
	}, shortcut: "⌘J"}
	a.panel.allowed = a.d.canPaste()
	if provider, ok := a.d.(appMetadata); ok {
		a.panel.appIcon = appIcons(provider)
	}
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "ClipNote", Width: width, Height: height, Hidden: true, Panel: true, Frameless: true,
		Transparent: true, CornerRadius: 20, DisableResize: true, Content: a.panel.content(),
	})
	a.win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			a.hide()
		}
	})
	a.win.OnBlur(func() {
		if time.Now().After(a.keepOpenUntil) {
			a.hide()
		}
	})
	if err := mygo.GlobalShortcut.Register("Cmd+J", func() {
		if a.busy {
			return
		}
		if a.win.IsVisible() {
			a.hide()
		} else {
			a.show()
		}
	}); err != nil {
		a.shortcutErr = "⌘J 注册失败，请检查快捷键冲突。可从菜单栏打开。"
		a.panel.shortcut = "菜单栏打开"
		log.Printf("ClipNote shortcut: %v", err)
	}
	var failed []string
	for i := range keyNums {
		if err := mygo.GlobalShortcut.Register(fmt.Sprintf("Cmd+%d", i+1), func() {
			if a.busy || a.quitting {
				return
			}
			if a.win.IsVisible() {
				a.panel.pickNumber(i)
				a.win.Invalidate()
				return
			}
			if err := a.pasteHidden(i); err != nil {
				a.show()
				a.panel.notice = err.Error()
				a.win.Invalidate()
			}
		}); err != nil {
			failed = append(failed, fmt.Sprintf("⌘%d", i+1))
			log.Printf("ClipNote shortcut: %v", err)
		}
	}
	if len(failed) > 0 {
		a.shortcutErr += strings.Join(failed, "、") + " 全局注册失败，请检查快捷键冲突。"
	}
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayIcon(), IconIsTemplate: true, ToolTip: "ClipNote 剪贴板历史"})
	if err == nil {
		a.tray.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开剪贴板历史", Click: func(*mygo.MenuItem, *mygo.Window) { a.show() }},
			{Label: "保留设置…", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.show()
				a.panel.settings = true
				a.panel.loadDays()
				a.win.Invalidate()
			}},
			mygo.Separator(),
			{Label: "退出 ClipNote", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Quit() }},
		}))
	} else {
		log.Printf("ClipNote menu bar: %v", err)
	}
	// Poll through MyGo's event loop with its native clipboard watcher. Do
	// not import whatever was already on the clipboard before launch.
	a.lastCount = a.d.count()
	a.stopWatch = mygo.Clipboard.OnChanged(a.capture)
	a.prune()
	go func() {
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-a.stopTimer:
				return
			case <-tick.C:
				a.win.Update(a.prune)
			}
		}
	}()
	if !mygo.App.WasOpenedAtLogin() {
		a.show()
	}
}
func (a *app) prune() {
	if err := a.panel.store.Commit(time.Now(), func(*history.Store) {}); err != nil {
		a.panel.notice = "无法清理本地历史：请检查磁盘和文件权限。"
	}
	a.panel.refresh(false)
}
func (a *app) capture() {
	count := a.d.count()
	if count == a.lastCount {
		return
	}
	var appName, bundleID string
	if provider, ok := a.d.(appMetadata); ok && !a.win.IsVisible() {
		pid, name, id := provider.frontInfo()
		if pid > 0 && pid != int32(os.Getpid()) && a.d.front() == pid {
			appName, bundleID = name, id
		}
	}
	text, ok := a.d.read()
	if a.d.count() != count {
		return
	}
	a.lastCount = count
	if ok && history.Valid(text) {
		now := time.Now()
		if err := a.panel.store.Commit(now, func(s *history.Store) { s.AddFrom(text, now, appName, bundleID) }); err != nil {
			a.panel.notice = "无法保存本地历史：请检查磁盘和文件权限。"
		}
		a.panel.refresh(false)
		a.win.Invalidate()
	}
}
func (a *app) show() {
	if a.busy {
		return
	}
	if a.win.IsVisible() {
		return
	}
	a.target = a.d.front()
	a.capture()
	a.panel.notice = a.shortcutErr
	a.prune()
	a.panel.query = ""
	a.panel.settings = false
	a.panel.allowed = a.d.canPaste()
	a.panel.refresh(true)
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	w, h := a.win.Size()
	a.win.SetPosition(d.WorkArea.X+max(0, (d.WorkArea.Width-w)/2), d.WorkArea.Y+max(0, (d.WorkArea.Height-h)/4))
	a.panel.search.Focus()
	a.win.Show()
	a.win.Invalidate()
}
func (a *app) hide() { a.win.Hide() }

// Hidden shortcuts use the app at keypress and the unfiltered history.
func (a *app) pasteHidden(index int) error {
	pid := a.d.front()
	a.capture()
	a.prune()
	entries := a.panel.store.Entries
	if index >= len(entries) {
		return nil
	}
	return a.d.paste(pid, entries[index].Text)
}

func (a *app) pick(entry history.Entry) {
	if a.busy {
		return
	}
	a.panel.allowed = a.d.canPaste()
	if !a.panel.allowed {
		a.panel.notice = "开启辅助功能后，再用快捷键或双击粘贴。"
		return
	}
	pid := a.target
	if pid <= 0 || pid == int32(os.Getpid()) || a.d.front() != pid {
		a.panel.notice = "找不到原应用。请回到要粘贴的位置，再按 ⌘J。"
		return
	}
	a.busy = true
	a.hide()
	// A nonactivating panel keeps the previous app and its insertion point.
	// Let AppKit release the panel's keyboard focus before posting to that PID.
	time.AfterFunc(115*time.Millisecond, func() {
		a.win.Update(func() {
			a.busy = false
			if a.quitting {
				return
			}
			if err := a.d.paste(pid, entry.Text); err != nil {
				log.Printf("ClipNote paste: %v", err)
				a.show()
				a.panel.notice = err.Error()
			}
		})
	})
}
func trayIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 36, 36))
	ink := image.NewUniform(color.NRGBA{A: 255})
	rect := func(x1, y1, x2, y2 int) {
		r := image.Rect(x1, y1, x2, y2)
		draw.Draw(img, r, ink, image.Point{}, draw.Src)
	}
	rect(8, 7, 11, 31)
	rect(25, 7, 28, 31)
	rect(9, 28, 27, 31)
	rect(8, 7, 28, 10)
	rect(13, 4, 23, 12)
	rect(14, 16, 22, 18)
	rect(14, 22, 22, 24)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		panic(fmt.Sprint(err))
	}
	return b.Bytes()
}
