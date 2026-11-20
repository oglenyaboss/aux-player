package stage

import "slices"

// SettingsInfo is what the settings overlay shows: the active agent and the preferences.
type SettingsInfo struct {
	Provider string // active provider id; "" when none
	URL      string // active base URL (prefills the custom address)
	Model    string
	Effort   string          // "", "low", "medium", "high"
	Keys     map[string]bool // provider ids with a stored key
	Lang     string          // "ru" or "en"
	Eggs     bool
	Usage    string // ChatGPT subscription usage, e.g. "5h: 34% · week: 12%"; "" unknown
	Apple    bool   // signed in to Apple Music
	Lastfm   bool   // Last.fm connected
	Discord  bool   // Discord Rich Presence on
	Error    string // why the last settings action failed; "" none
}

type settingsUI struct {
	info    SettingsInfo
	sel     int
	confirm bool
	form    *agentForm
}

type settingsRow int

const (
	setRowProvider settingsRow = iota
	setRowKey
	setRowModel
	setRowEffort
	setRowForget
	setRowLang
	setRowEggs
)

const (
	setHeadLine settingsRow = -1
	setGapLine  settingsRow = -2
)

type settingsLine struct {
	row   settingsRow
	label string
	value string
	arrow bool
}

// OpenSettings opens the overlay over whatever is on screen, or refreshes it when it is
// already open; the onboarding owns the screen.
func (m *Model) OpenSettings(info SettingsInfo) {
	if m.ph == PhOnboard {
		return
	}
	if m.settings == nil {
		m.settings = &settingsUI{info: info}
		return
	}
	m.settings.info = info
	m.settings.clamp()
}

// SettingsOpen reports whether the settings overlay is up.
func (m *Model) SettingsOpen() bool { return m.settings != nil }

func (m *Model) settingsKey(k, text string) {
	s := m.settings
	if s.form != nil {
		m.settingsFormKey(k, text)
		return
	}
	armed := s.confirm && s.selected() == setRowForget
	s.confirm = false
	switch k {
	case "esc":
		m.settings = nil
	case "up", "k":
		s.sel = max(0, s.sel-1)
	case "down", "j":
		s.sel = min(settingsRowCount(s.lines())-1, s.sel+1)
	case "enter":
		if armed {
			m.settingsForget()
			return
		}
		m.settingsAct()
	}
}

func (m *Model) settingsFormKey(k, text string) {
	s := m.settings
	f := s.form
	switch m.formKey(f, k, text) {
	case formBack:
		s.form = nil
	case formDone:
		provider, url, key, model, effort := f.result()
		m.send(AgentSet{Provider: provider, URL: url, Key: key, Model: model, Effort: effort})
		s.info.Provider, s.info.Model, s.info.Effort = provider, model, effort
		if provider == "custom" {
			s.info.URL = url
		}
		if key != "" {
			s.setKey(provider)
		}
		m.o.NoAgent = false
		s.form = nil
		s.clamp()
	case formSave:
		id := providers[f.prov].id
		m.send(AgentSet{Provider: id, URL: f.agentURL(), Key: string(f.key)})
		s.info.Provider = id
		s.setKey(id)
		m.o.NoAgent = false
		s.form = nil
		s.clamp()
	}
}

func (m *Model) settingsAct() {
	s := m.settings
	switch s.selected() {
	case setRowProvider:
		s.form = s.formFor()
	case setRowKey:
		s.form = s.formFor()
		s.form.setSub(SubKey)
	case setRowModel:
		s.form = s.formFor()
		s.form.setSub(SubLoading)
		m.send(LoadModels{Provider: s.info.Provider, URL: s.form.agentURL(), Key: ""})
	case setRowEffort:
		order := [...]string{"low", "medium", "high"}
		i := slices.Index(order[:], s.info.Effort)
		next := order[(max(0, i)+1)%len(order)]
		s.info.Effort = next
		m.send(AgentSet{Provider: s.info.Provider, Model: s.info.Model, Effort: next})
	case setRowForget:
		s.confirm = true
	case setRowLang:
		other := "en"
		if s.info.Lang != "ru" {
			other = "ru"
		}
		SetLang(other)
		s.info.Lang = other
		m.send(LangSet{Lang: other})
	case setRowEggs:
		s.info.Eggs = !s.info.Eggs
		m.o.NoEggs = !s.info.Eggs
		m.send(Eggs{On: s.info.Eggs})
	}
}

func (m *Model) settingsForget() {
	s := m.settings
	if s.info.Provider == "" {
		return
	}
	if s.info.Keys != nil {
		s.info.Keys[s.info.Provider] = false
	}
	m.o.NoAgent = true
	m.send(AgentForget{Provider: s.info.Provider})
	s.clamp()
}

func (s *settingsUI) setKey(id string) {
	if s.info.Keys == nil {
		s.info.Keys = map[string]bool{}
	}
	s.info.Keys[id] = true
}

func (s *settingsUI) formFor() *agentForm {
	f := newAgentForm()
	f.prov = max(0, slices.IndexFunc(providers, func(p provider) bool { return p.id == s.info.Provider }))
	f.saved = s.info.Keys
	f.current = s.info.Model
	if s.info.Provider == "custom" {
		f.url = []rune(s.info.URL)
	}
	return f
}

func (s *settingsUI) lines() []settingsLine {
	out := []settingsLine{{row: setHeadLine, label: tr("агент", "agent")}}
	out = append(out, settingsLine{row: setRowProvider, label: tr("провайдер", "provider"), value: s.providerName(), arrow: true})
	if s.info.Provider != "" {
		key := tr("нет", "none")
		if s.info.Keys[s.info.Provider] {
			key = tr("сохранён", "saved")
		}
		model := s.info.Model
		if model == "" {
			model = "—"
		}
		out = append(out, settingsLine{row: setRowKey, label: tr("ключ", "key"), value: key})
		out = append(out, settingsLine{row: setRowModel, label: tr("модель", "model"), value: model})
		if s.info.Effort != "" {
			out = append(out, settingsLine{row: setRowEffort, label: tr("рассуждение", "reasoning"), value: effortLabel(s.info.Effort)})
		}
		if s.info.Keys[s.info.Provider] {
			value := ""
			if s.confirm {
				value = tr("enter ещё раз — удалить", "enter again to remove")
			}
			out = append(out, settingsLine{row: setRowForget, label: tr("удалить ключ", "remove key"), value: value})
		}
	}
	out = append(out, settingsLine{row: setGapLine})
	out = append(out, settingsLine{row: setHeadLine, label: tr("интерфейс", "interface")})
	out = append(out, settingsLine{row: setRowLang, label: tr("язык", "language"), value: langLabel(s.info.Lang)})
	eggs := tr("выкл", "off")
	if s.info.Eggs {
		eggs = tr("вкл", "on")
	}
	out = append(out, settingsLine{row: setRowEggs, label: tr("пасхалки", "easter eggs"), value: eggs})
	return out
}

func (s *settingsUI) selected() settingsRow {
	n := 0
	for _, l := range s.lines() {
		if l.row < 0 {
			continue
		}
		if n == s.sel {
			return l.row
		}
		n++
	}
	return setRowProvider
}

func (s *settingsUI) clamp() {
	s.sel = max(0, min(s.sel, settingsRowCount(s.lines())-1))
}

func settingsRowCount(lines []settingsLine) int {
	n := 0
	for _, l := range lines {
		if l.row >= 0 {
			n++
		}
	}
	return n
}

func (s *settingsUI) providerName() string {
	if s.info.Provider == "" {
		return tr("не подключён", "not connected")
	}
	for _, p := range providers {
		if p.id == s.info.Provider {
			return p.name
		}
	}
	return s.info.Provider
}

func effortLabel(e string) string {
	for i, id := range [...]string{"low", "medium", "high"} {
		if id == e && i < len(efforts) {
			return efforts[i]
		}
	}
	return e
}

func langLabel(l string) string {
	if l == "ru" {
		return "русский"
	}
	return "English"
}

func settingsFormNeed(a *agentForm) int {
	switch a.sub {
	case SubProvider:
		return len(providers) + 3
	case SubURL, SubKey:
		return 4
	case SubLoading:
		return 2
	case SubModel:
		return 10
	case SubEffort:
		return 6
	}
	return 4
}

func (m *Model) drawSettings(f *fb) {
	s := m.settings
	w := min(64, max(8, m.w-4))
	need := len(s.lines()) + 6
	if s.form != nil {
		need = settingsFormNeed(s.form) + 4
	}
	h := min(need, max(1, m.h-2))
	x, y := (m.w-w)/2, (m.h-h)/2
	bg := m.th.bg.lerp(m.th.text, 0.07)
	f.fill(x, y, w, h, bg)
	m.glowBox(f, x, y, w, h, 1)
	if h < 3 {
		return
	}
	f.textS(x+2, y+1, trunc(tr("настройки", "settings"), w-4), m.th.a1, true, false)
	if s.form != nil {
		m.drawAgentForm(f, s.form, x+2, y+2, w-4, h-4, false, 1)
		return
	}
	m.drawSettingsRows(f, x, y, w, h)
	hy := y + h - 2
	dx := m.mlCap(f, x+2, hy, "↑↓", tr("выбрать", "choose"))
	dx += 2
	dx += m.mlCap(f, x+2+dx, hy, "enter", tr("изменить", "change")) + 2
	if x+2+dx < x+w-2 {
		m.mlCap(f, x+2+dx, hy, "esc", tr("закрыть", "close"))
	}
}

func (m *Model) drawSettingsRows(f *fb, x, y, w, h int) {
	s := m.settings
	sel := 0
	for i, l := range s.lines() {
		yy := y + 3 + i
		if yy > y+h-2 {
			break
		}
		if l.row < 0 {
			if l.row == setHeadLine {
				f.textS(x+2, yy, trunc(l.label, w-4), m.th.dim.mul(0.9), true, false)
			}
			continue
		}
		if sel == s.sel {
			for xx := x + 1; xx < x+w-1; xx++ {
				f.tint(xx, yy, m.th.a1, 0.18)
			}
		}
		fg := m.th.text
		if sel == s.sel {
			fg = m.th.a1
		}
		f.textS(x+2, yy, trunc(l.label, w-4), fg, sel == s.sel, false)
		if l.arrow {
			f.text(x+w-2, yy, "›", m.th.dim)
		}
		if l.value != "" {
			vx := x + w - 3
			if l.arrow {
				vx--
			}
			f.text(max(x+2, vx-len([]rune(l.value))), yy, l.value, m.th.dim)
		}
		sel++
	}
}
