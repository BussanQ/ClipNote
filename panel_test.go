package main

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"clipnote.local/app/history"
	"github.com/egoist/mygo/ui"
)

func sample(t *testing.T) *panel {
	t.Helper()
	s, e := history.Open(filepath.Join(t.TempDir(), "history.json"))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	for _, text := range []string{"sixth row", "fifth row", "fourth row", "Third example", "Second example", "  First example\noriginal  "} {
		s.Add(text, now)
	}
	p := &panel{store: s, allowed: true, shortcut: "⌘J", pick: func(history.Entry) {}, close: func() {}, permission: func() {}}
	p.refresh(true)
	return p
}
func TestQuickPastePaths(t *testing.T) {
	p := sample(t)
	var picks []string
	p.pick = func(e history.Entry) { picks = append(picks, e.Text) }
	tt := ui.NewTester(p.view, width, height)
	keys := []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5}
	for _, key := range keys {
		tt.Key(ui.Cmd, key)
		tt.Frame()
	}
	want := []string{"  First example\noriginal  ", "Second example", "Third example", "fourth row", "fifth row"}
	if !slices.Equal(picks, want) {
		t.Fatalf("shortcut picks: %#v", picks)
	}
	picks = nil
	row, ok := tt.Access("Second example")
	if !ok {
		t.Fatalf("row inaccessible: %+v", tt.AccessAll())
	}
	x, y := row.Bounds.X+12, row.Bounds.Y+row.Bounds.H/2
	tt.ClickAt(x, y)
	tt.Frame()
	if len(picks) != 0 {
		t.Fatal("single click pasted")
	}
	tt.ClickAt(x, y)
	tt.Frame()
	if !slices.Equal(picks, []string{"Second example"}) {
		t.Fatalf("double click picks: %#v", picks)
	}
}
func TestSearchAndDelete(t *testing.T) {
	p := sample(t)
	var picks []string
	p.pick = func(e history.Entry) { picks = append(picks, e.Text) }
	tt := ui.NewTester(p.view, width, height)
	if err := tt.Click("搜索复制过的文本"); err != nil {
		t.Fatal(err)
	}
	tt.Type("ThIrD")
	tt.Frame()
	if len(p.results) != 1 || p.results[0].Text != "Third example" {
		t.Fatalf("search results: %+v", p.results)
	}
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	if len(picks) != 0 {
		t.Fatal("out of range shortcut pasted")
	}
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if !slices.Equal(picks, []string{"Third example"}) {
		t.Fatalf("filtered shortcut picks: %+v", picks)
	}
	if err := tt.Click("删除此记录"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if len(picks) != 1 || len(p.results) != 0 || len(p.store.Entries) != 5 {
		t.Fatal("delete removed wrong entries or triggered paste")
	}
}
func TestRender(t *testing.T) {
	dir := os.Getenv("CLIPNOTE_PREVIEWS")
	if dir == "" {
		return
	}
	for _, dark := range []bool{false, true} {
		p := sample(t)
		p.store.Entries = nil
		now := time.Date(2026, 11, 25, 10, 20, 0, 0, time.Local)
		for i, value := range []string{"下周设计评审：先确认标题与留白", "go test ./...", "会议笔记：先确认范围，再拆分任务", "https://mygo.run/docs/ui", "macOS 原生剪贴板 · 快速粘贴"} {
			p.store.Add(value, now.Add(time.Duration(i)*time.Minute))
		}
		p.refresh(true)
		tt := ui.NewTesterWith(p.view, width, height, ui.TesterOptions{Dark: dark})
		file := "preview-light.png"
		if dark {
			file = "preview-dark.png"
		}
		out, e := os.Create(filepath.Join(dir, file))
		if e != nil {
			t.Fatal(e)
		}
		if e = png.Encode(out, tt.Image()); e != nil {
			t.Fatal(e)
		}
		if e = out.Close(); e != nil {
			t.Fatal(e)
		}
	}
}

type quickPasteDesktop struct {
	desktop
	pid    int32
	target int32
	text   string
	calls  int
}

func (d *quickPasteDesktop) count() int64 { return 0 }
func (d *quickPasteDesktop) front() int32 { return d.pid }
func (d *quickPasteDesktop) paste(pid int32, text string) error {
	d.target, d.text = pid, text
	d.calls++
	return nil
}

func TestHiddenQuickPaste(t *testing.T) {
	p := sample(t)
	p.query, p.settings = "Third", true
	p.refresh(true)
	d := &quickPasteDesktop{}
	a := &app{panel: p, d: d, target: 99}
	for i, want := range []string{"  First example\noriginal  ", "Second example", "Third example", "fourth row", "fifth row"} {
		d.pid = int32(100 + i)
		if err := a.pasteHidden(i); err != nil || d.text != want || d.target != d.pid || d.calls != i+1 {
			t.Fatalf("⌘%d: text %q, target %d, calls %d, err %v", i+1, d.text, d.target, d.calls, err)
		}
	}
	p.store.Entries = nil
	if err := a.pasteHidden(0); err != nil || d.calls != 5 {
		t.Fatalf("empty history: calls %d, err %v", d.calls, err)
	}
}
