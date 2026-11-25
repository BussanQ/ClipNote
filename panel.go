package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"clipnote.local/app/history"
	"github.com/egoist/mygo/ui"
)

const width, height = 620, 572

type panel struct {
	appIcon                       func(string) *ui.Bitmap
	store                         *history.Store
	pick                          func(history.Entry)
	close                         func()
	permission                    func()
	allowed, settings             bool
	shortcut, query, days, notice string
	results                       []history.Entry
	selected                      int
	search                        ui.Handle
	list                          ui.ListState
}

func (p *panel) refresh(reset bool) {
	p.results = p.store.Search(p.query)
	if reset {
		p.list.Choose(0)
		p.list.ScrollTo(0, ui.Start)
		p.selected = 0
	}
}
func (p *panel) loadDays() { p.days = strconv.Itoa(p.store.Days) }
func (p *panel) saveDays() {
	n, err := strconv.Atoi(strings.TrimSpace(p.days))
	if err != nil || n < 1 || n > history.MaxDays {
		p.notice = "请输入 1–365 之间的整数。"
		return
	}
	if err = p.store.Commit(time.Now(), func(s *history.Store) { s.Days = n }); err != nil {
		p.notice = "保留设置保存失败。"
		return
	}
	p.settings = false
	p.notice = ""
	p.refresh(true)
	p.search.Focus()
}
func (p *panel) delete(id uint64) {
	if err := p.store.Commit(time.Now(), func(s *history.Store) { s.Delete(id) }); err != nil {
		p.notice = "这条记录未能删除。"
		return
	}
	p.refresh(false)
}
func preview(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "（仅空白字符）"
	}
	if utf8.RuneCountInString(text) <= 180 {
		return text
	}
	r := []rune(text)
	return string(r[:180]) + "…"
}

type palette struct {
	bg, card, sub, border, text, muted, accent, accentTint, danger ui.Color
	selectionBorder, control, brand, ok ui.Color
}

func colors(dark bool) palette {
	if dark {
		return palette{
			bg: ui.Hex("#1e1b25"), card: ui.Hex("#292530"), sub: ui.Hex("#241f2c"),
			border: ui.Hex("#3e3749"), text: ui.Hex("#f4f0fa"), muted: ui.Hex("#bab0c8"),
			accent: ui.Hex("#baa6ff"), accentTint: ui.Hex("#352b4c"), danger: ui.Hex("#ffadaf"),
			selectionBorder: ui.Hex("#635287"), control: ui.Hex("#9f94af"),
			brand: ui.Hex("#7959e3"), ok: ui.Hex("#70c5a1"),
		}
	}
	return palette{
		bg: ui.Hex("#f9f8fc"), card: ui.Hex("#ffffff"), sub: ui.Hex("#f2eff7"),
		border: ui.Hex("#e3deed"), text: ui.Hex("#242232"), muted: ui.Hex("#696278"),
		accent: ui.Hex("#6c52bc"), accentTint: ui.Hex("#eee9fa"), danger: ui.Hex("#a13950"),
		selectionBorder: ui.Hex("#d8ccf2"), control: ui.Hex("#797187"),
		brand: ui.Hex("#7254d9"), ok: ui.Hex("#30846b"),
	}
}

func lineIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">`+body+`</svg>`))
}

var (
	clipIcon = lineIcon(`<rect x="8" y="3" width="8" height="4" rx="1.5"/><path d="M8 5H6a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-2M8 12h8m-8 4h6"/>`)
	searchIcon = lineIcon(`<circle cx="10.5" cy="10.5" r="5.8"/><path d="m15 15 4.5 4.5"/>`)
	settingsIcon = lineIcon(`<path d="M4 6h16M4 12h16M4 18h16M9 4v4m6 2v4m-5 2v4"/>`)
	clockIcon = lineIcon(`<circle cx="12" cy="12" r="8"/><path d="M12 7.5V12l3 1.5"/>`)
	trashIcon = lineIcon(`<path d="M5 7h14M9 7V4h6v3M7 7l1 13h8l1-13m-6 3v7m2-7v7"/>`)
	textIcon = lineIcon(`<rect x="5" y="3" width="14" height="18" rx="2"/><path d="M8.5 8h7m-7 4h7m-7 4h4"/>`)
	linkIcon = lineIcon(`<path d="M10 14a4 4 0 0 0 5.7 0l3-3a4 4 0 0 0-5.7-5.7l-1.7 1.7m2.7 3a4 4 0 0 0-5.7 0l-3 3a4 4 0 0 0 5.7 5.7l1.7-1.7"/>`)
	closeIcon = lineIcon(`<path d="m7 7 10 10M17 7 7 17"/>`)
)

func title(c *ui.Context, s string, cl ui.Color) {
	ui.Text(c, s).FontSize(22).FontWeight(750).TextColor(cl).SingleLine()
}
func text(c *ui.Context, s string, size float32, cl ui.Color) ui.Element {
	return ui.Text(c, s).FontSize(size).TextColor(cl)
}
func pill(c *ui.Context, s string, col palette) ui.Element {
	return text(c, s, 10, col.muted).FontWeight(650).SingleLine().Padding(3, 6).
		Radius(5).Background(col.card).Border(1, col.border)
}
func action(c *ui.Context, label string, col palette) ui.Element {
	return ui.Button(c, label).Height(30).Radius(8).PaddingX(11).FontSize(12).
		Background(col.card).TextColor(col.text).Border(1, col.border).Cursor(ui.CursorPointer)
}
func iconButton(c *ui.Context, icon *ui.SVG, label string, size float32, col palette, destructive bool) ui.Element {
	btn := ui.Button(c, "").Size(size, size).Padding(0).Radius(7).Border(0, ui.Transparent).
		AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).Label(label).Tooltip(label)
	ink, bg := col.control, ui.Transparent
	if btn.Hovered() {
		ink, bg = col.accent, col.accentTint
		if destructive {
			ink = col.danger
		}
	}
	btn.Background(bg).Children(func() { ui.Icon(c, icon).FontSize(17).TextColor(ink) })
	return btn
}
func (p *panel) content() *ui.Content { return ui.View(p.view) }

var keyNums = []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5}

func (p *panel) pickNumber(index int) {
	if !p.settings && index < len(p.results) {
		p.pick(p.results[index])
	}
}

func (p *panel) view(c *ui.Context) {
	col := colors(c.Theme().Dark)
	t := *c.Theme().WithAccent(col.accent)
	t.Radius, t.SurfaceHover, t.Selection = 11, col.sub, col.accentTint
	c.SetTheme(&t)
	c.Root().Background(col.bg)
	if c.Shortcut(0, ui.KeyEscape) {
		if p.settings {
			p.settings = false
			p.notice = ""
			p.search.Focus()
		} else {
			p.close()
			return
		}
	}
	for i, k := range keyNums {
		if c.Shortcut(ui.Cmd, k) && !c.ShortcutRepeated() {
			p.pickNumber(i)
		}
	}
	ui.Column(c).Fill().Children(func() {
		p.header(c, col)
		if p.settings {
			ui.Column(c).Grow(1).MinHeight(0).PaddingX(22).PaddingY(10).Children(func() {
				p.settingsView(c, col)
			})
		} else {
			p.resultsView(c, col)
		}
		if p.notice != "" {
			ui.Column(c).PaddingX(22).PaddingY(4).Children(func() {
				text(c, p.notice, 12, col.danger).MaxLines(2).Padding(7, 10).Radius(7).Background(col.sub)
			})
		}
		p.footer(c, col)
	})
}

func (p *panel) header(c *ui.Context, col palette) {
	ui.Row(c).Height(64).PaddingX(22).Gap(11).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(34, 34).Radius(9).Background(col.brand).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.Icon(c, clipIcon).FontSize(21).TextColor(ui.Hex("#ffffff"))
		})
		title(c, "ClipNote", col.text)
		ui.Box(c).Grow(1)
		pill(c, p.shortcut, col).Tooltip("呼出 / 收起剪贴板")
		ui.Box(c).Size(1, 16).Background(col.border)
		icon, label := settingsIcon, "保留设置"
		if p.settings {
			icon, label = closeIcon, "返回历史"
		}
		if iconButton(c, icon, label, 29, col, false).Clicked() {
			p.settings = !p.settings
			p.loadDays()
			p.notice = ""
			if !p.settings {
				p.search.Focus()
			}
		}
	})
}

func (p *panel) footer(c *ui.Context, col palette) {
	ui.Box(c).Height(1).Background(col.border)
	ui.Row(c).Height(42).PaddingX(22).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Size(7, 7).Radius(4).Background(col.ok)
		text(c, "正在记录", 11, col.muted).SingleLine()
		ui.Box(c).Grow(1)
		if !p.settings && !p.allowed {
			if action(c, "开启权限", col).Tooltip("原位粘贴需要 macOS 辅助功能权限").Clicked() && p.permission != nil {
				p.permission()
			}
		}
		if !p.settings {
			keyHint(c, "⌘ 1–5", "粘贴", col)
			keyHint(c, "↑ ↓", "选择", col)
			keyHint(c, "↵", "粘贴", col)
		}
		label := "关闭"
		if p.settings {
			label = "返回"
		}
		keyHint(c, "esc", label, col)
	})
}

func keyHint(c *ui.Context, key, label string, col palette) {
	ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
		pill(c, key, col)
		text(c, label, 11, col.muted).SingleLine()
	})
}

func (p *panel) resultsView(c *ui.Context, col palette) {
	ui.Column(c).PaddingX(22).Children(func() {
		field := ui.Row(c).Height(45).PaddingX(12).Gap(8).AlignItems(ui.Center).
			Background(col.card).Radius(11).Border(1, col.border)
		field.Children(func() {
			ui.Icon(c, searchIcon).FontSize(18).TextColor(col.accent)
			in := ui.TextInputBase(c, &p.query).Bind(&p.search).Grow(1).MinWidth(0).
				Height(36).Padding(0).Background(ui.Transparent).TextColor(col.text).FontSize(14).
				Placeholder("搜索复制过的文本…").Label("搜索复制过的文本").Search()
			if in.Focused() {
				field.Border(1, col.selectionBorder)
			}
			if in.Changed() {
				p.refresh(true)
			}
			if p.query != "" && iconButton(c, closeIcon, "清空搜索", 25, col, false).Clicked() {
				p.query = ""
				p.refresh(true)
				p.search.Focus()
			}
			if in.Submitted() && p.selected >= 0 && p.selected < len(p.results) {
				p.pick(p.results[p.selected])
			}
			// The list itself handles arrow keys when focused. Starting
			// from search, keep typing focused here while moving selection.
			if in.Shortcut(0, ui.KeyUp) {
				p.move(-1)
			}
			if in.Shortcut(0, ui.KeyDown) {
				p.move(1)
			}
		})
		ui.Row(c).Height(44).AlignItems(ui.Center).Gap(8).Children(func() {
			label := "最近复制"
			if strings.TrimSpace(p.query) != "" {
				label = "搜索结果"
			}
			text(c, label, 12, col.text).FontWeight(700)
			text(c, strconv.Itoa(len(p.results)), 11, col.muted).FontWeight(650)
			ui.Box(c).Grow(1)
			ui.Icon(c, clockIcon).FontSize(14).TextColor(col.control)
			text(c, fmt.Sprintf("保留 %d 天", p.store.Days), 11, col.muted)
		})
	})
	if len(p.results) == 0 {
		ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Center).Justify(ui.Center).Gap(8).Children(func() {
			emptyIcon, heading, detail := clipIcon, "还没有复制记录", "复制一段纯文本，ClipNote 会把它留在这里。"
			if strings.TrimSpace(p.query) != "" {
				emptyIcon, heading, detail = searchIcon, "没找到匹配的文本", "换个关键词，或清空搜索。"
			}
			ui.Row(c).Size(44, 44).Radius(13).Background(col.accentTint).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Icon(c, emptyIcon).FontSize(22).TextColor(col.accent)
			})
			text(c, heading, 16, col.text).FontWeight(700)
			text(c, detail, 12, col.muted)
		})
		return
	}
	var deleteID uint64
	p.list.Selected = &p.selected
	p.list.Key = func(i int) any { return p.results[i].ID }
	p.list.Label = func(i int) string { return preview(p.results[i].Text) }
	list := ui.List(c, &p.list, len(p.results), func(i int) {
		e := p.results[i]
		bg, border := col.bg, ui.Transparent
		if p.selected == i {
			bg, border = col.accentTint, col.selectionBorder
		}
		ui.Row(c).Height(59).PaddingX(15).Gap(12).AlignItems(ui.Center).Radius(11).
			Background(bg).Border(1, border).Children(func() {
			p.entryIcon(c, e, col)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				text(c, preview(e.Text), 14, col.text).SingleLine().FontWeight(650)
				text(c, entryCaption(e, time.Now()), 11, col.muted).SingleLine()
			})
			if i < 5 {
				pill(c, fmt.Sprintf("⌘ %d", i+1), col)
			}
			if iconButton(c, trashIcon, "删除此记录", 28, col, true).Clicked() {
				deleteID = e.ID
			}
		})
	}).Grow(1).MinHeight(0).PaddingX(12).PaddingY(2).Gap(3).Background(col.bg)
	if deleteID != 0 {
		p.delete(deleteID)
	} else if list.Submitted() && p.selected >= 0 && p.selected < len(p.results) {
		p.pick(p.results[p.selected])
	}
}

func (p *panel) move(delta int) {
	if len(p.results) == 0 {
		return
	}
	p.list.Selected = &p.selected
	i := max(0, min(p.selected+delta, len(p.results)-1))
	p.list.Choose(i)
	p.list.ScrollIntoView(i)
}

func (p *panel) entryIcon(c *ui.Context, e history.Entry, col palette) {
	ui.Row(c).Size(30, 30).Radius(8).Border(1, col.border).Background(col.card).
		AlignItems(ui.Center).Justify(ui.Center).Children(func() {
		if p.appIcon != nil && e.AppID != "" {
			if bitmap := p.appIcon(e.AppID); bitmap != nil {
				ui.Image(c, bitmap).Size(24, 24)
				return
			}
		}
		icon := textIcon
		if strings.HasPrefix(e.Text, "https://") || strings.HasPrefix(e.Text, "http://") {
			icon = linkIcon
		}
		ui.Icon(c, icon).FontSize(17).TextColor(col.accent)
	})
}

func entryCaption(e history.Entry, now time.Time) string {
	when := e.Copied.In(now.Location())
	yesterday := now.AddDate(0, 0, -1)
	var stamp string
	switch {
	case when.Year() == now.Year() && when.YearDay() == now.YearDay():
		stamp = "今天 " + when.Format("15:04")
	case when.Year() == yesterday.Year() && when.YearDay() == yesterday.YearDay():
		stamp = "昨天 " + when.Format("15:04")
	case when.Year() == now.Year():
		stamp = when.Format("01-02 15:04")
	default:
		stamp = when.Format("2006-01-02 15:04")
	}
	name := e.AppName
	if name == "" {
		name = "纯文本"
	}
	return name + "  ·  " + stamp
}

func (p *panel) settingsView(c *ui.Context, col palette) {
	ui.Column(c).Grow(1).Gap(13).Padding(19).Radius(12).Border(1, col.border).Background(col.card).Children(func() {
		text(c, "历史保留", 17, col.text).FontWeight(700)
		text(c, "默认 7 天，可调整为 1–365 天。缩短天数后，超期记录会立即删除。", 13, col.muted).MaxLines(2)
		ui.Row(c).Gap(9).AlignItems(ui.Center).Children(func() {
			entry := ui.TextInputBase(c, &p.days).Width(78).Height(36).PaddingX(10).Radius(7).
				Border(1, col.border).Background(col.sub).TextColor(col.text).Label("保留天数")
			text(c, "天", 13, col.muted)
			if action(c, "保存", col).Clicked() || entry.Submitted() {
				p.saveDays()
			}
		})
		text(c, "历史文件仅存本机。不记录图片、文件或被来源应用标记为机密的剪贴板内容。没有机密标记的密码或令牌仍可能被记录。", 12, col.muted).MaxLines(3)
		if !p.allowed {
			if action(c, "开启直接粘贴权限", col).Clicked() {
				p.permission()
			}
		}
	})
}

var errorView = ui.View(func(c *ui.Context) {
	col := colors(c.Theme().Dark)
	c.Root().Background(col.bg)
	ui.Column(c).Padding(26).Gap(10).Fill().Children(func() {
		title(c, "ClipNote 无法读取历史", col.text)
		text(c, "原历史文件没有被覆盖。请检查 ~/Library/Application Support/ClipNote/history.json 后重新打开。", 14, col.muted).MaxLines(3)
	})
})
