package stage

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

type agentForm struct {
	t       float64
	sub     agentSub
	subT    float64
	prov    int
	url     []rune
	key     []rune
	filter  []rune
	model   int
	effort  int
	picked  string
	list    []ModelInfo
	total   int
	replied bool
	errMsg  string
	errT    float64
	saved   map[string]bool
	current string
}

type form = agentForm

func newAgentForm() *agentForm {
	return &agentForm{effort: 1, errT: -1}
}

func (a *agentForm) step(dt float64) {
	a.t += dt
	if a.sub == SubLoading && a.replied && a.t-a.subT > 1.2 {
		a.setSub(SubModel)
	}
}

func (a *agentForm) setSub(s agentSub) {
	a.sub, a.subT, a.errT = s, a.t, -1
	a.replied, a.errMsg = false, ""
}

func (a *agentForm) models() []ModelInfo {
	q := strings.ToLower(string(a.filter))
	var out []ModelInfo
	for _, mi := range a.list {
		if strings.Contains(strings.ToLower(mi.ID), q) {
			out = append(out, mi)
		}
	}
	return out
}

func (a *agentForm) reply(list []ModelInfo, total int, err string) {
	if err != "" {
		a.setSub(SubKey)
		a.errMsg = err
		return
	}
	a.list, a.total, a.replied = list, total, true
	if a.current != "" {
		if i := slices.IndexFunc(a.models(), func(mi ModelInfo) bool { return mi.ID == a.current }); i >= 0 {
			a.model = i
			return
		}
	}
	a.model = max(0, slices.IndexFunc(a.models(), func(mi ModelInfo) bool { return mi.Rec }))
}

func (a *agentForm) agentURL() string {
	if p := providers[a.prov]; p.keys != "" {
		return "https://" + p.host
	}
	u := string(a.url)
	if !strings.Contains(u, "://") {
		u = "https://" + u
	}
	return u
}

func (a *agentForm) pickedInfo() (ModelInfo, bool) {
	for _, mi := range a.list {
		if mi.ID == a.picked {
			return mi, true
		}
	}
	return ModelInfo{}, false
}

func (a *agentForm) result() (provider, url, key, model, effort string) {
	if a.picked == "" {
		return "", "", "", "", ""
	}
	provider, url, key, model = providers[a.prov].id, a.agentURL(), string(a.key), a.picked
	if mi, ok := a.pickedInfo(); ok && mi.Reasoning {
		effort = [...]string{"low", "medium", "high"}[a.effort]
	}
	return
}

type formResult int

const (
	formNone formResult = iota
	formDone
	formBack
	formSave
)

func (m *Model) formKey(a *agentForm, k, text string) formResult {
	edit := func(buf *[]rune) {
		switch {
		case k == "backspace":
			if len(*buf) > 0 {
				*buf = (*buf)[:len(*buf)-1]
			}
		case len(text) > 0:
			*buf = append(*buf, []rune(text)...)
		}
	}
	if k == "esc" {
		switch a.sub {
		case SubLoading:
			a.setSub(SubKey)
		case SubURL:
			a.setSub(SubProvider)
		case SubKey:
			if providers[a.prov].keys == "" {
				a.setSub(SubURL)
			} else {
				a.setSub(SubProvider)
			}
		case SubModel:
			a.setSub(SubKey)
		case SubEffort:
			a.setSub(SubModel)
		case SubProvider:
			return formBack
		}
		return formNone
	}
	switch a.sub {
	case SubProvider:
		switch k {
		case "up", "k":
			a.prov = max(0, a.prov-1)
		case "down", "j":
			a.prov = min(len(providers)-1, a.prov+1)
		case "enter":
			id := providers[a.prov].id
			if a.saved[id] && id != "custom" {
				a.setSub(SubLoading)
				m.send(LoadModels{Provider: id, URL: a.agentURL(), Key: ""})
			} else if providers[a.prov].keys == "" {
				a.setSub(SubURL)
			} else {
				a.setSub(SubKey)
			}
		}
	case SubURL:
		if k == "enter" {
			if len(a.url) == 0 {
				a.errT = a.t
			} else {
				a.setSub(SubKey)
			}
			return formNone
		}
		edit(&a.url)
	case SubKey:
		if k == "tab" && a.errMsg != "" {
			return formSave
		}
		if k == "enter" {
			if len(a.key) == 0 && providers[a.prov].keys != "" {
				a.errT = a.t
			} else {
				a.setSub(SubLoading)
				m.send(LoadModels{Provider: providers[a.prov].id, URL: a.agentURL(), Key: string(a.key)})
			}
			return formNone
		}
		edit(&a.key)
	case SubModel:
		list := a.models()
		switch k {
		case "up":
			a.model = max(0, a.model-1)
		case "down":
			a.model = max(0, min(len(list)-1, a.model+1))
		case "enter":
			if len(list) == 0 {
				return formNone
			}
			a.picked = list[a.model].ID
			if list[a.model].Reasoning {
				a.setSub(SubEffort)
			} else {
				return formDone
			}
		default:
			edit(&a.filter)
			a.model = 0
		}
	case SubEffort:
		switch k {
		case "left", "h":
			a.effort = max(0, a.effort-1)
		case "right", "l":
			a.effort = min(len(efforts)-1, a.effort+1)
		case "enter":
			return formDone
		}
	}
	return formNone
}

func (m *Model) drawAgentForm(f *fb, a *agentForm, x, y, w, maxH int, intro bool, al float64) int {
	key := func(x, y int, k, label string, al float64) int {
		n := f.fade(x, y, k, m.th.a1, al, true)
		return n + 1 + f.fade(x+n+1, y, label, m.th.dim, al, false) + 3
	}
	p := providers[a.prov]
	switch a.sub {
	case SubProvider:
		top := y
		if intro {
			f.fade(x, y, trunc(tr("агент — сердце AUX: подбирает музыку под настроение,", "the agent is the heart of AUX: it picks music for your mood,"), w), m.th.text, al, false)
			f.fade(x, y+1, trunc(tr("учится на твоих + и − и ведёт поток дальше", "learns from your + and − and keeps the stream going"), w), m.th.text, al, false)
			top = y + 3
		}
		f.fade(x, top, tr("откуда брать модель", "where the model comes from"), m.th.dim, al, false)
		for i, pr := range providers {
			if i == a.prov {
				f.fade(x, top+1+i, "›", m.th.a1, al, true)
				f.fade(x+2, top+1+i, pr.name, m.th.text, al, false)
			} else {
				f.fade(x+2, top+1+i, pr.name, m.th.dim, al, false)
			}
			if len([]rune(pr.name))+len([]rune(pr.host))+4 <= w {
				f.fade(x+w-len([]rune(pr.host)), top+1+i, pr.host, m.th.dim, al*0.7, false)
			}
		}
		ky := top + 1 + len(providers)
		dx := key(x, ky, "↑↓", tr("выбрать", "choose"), al)
		dx += key(x+dx, ky, "enter", tr("дальше", "continue"), al)
		if intro {
			key(x+dx, ky, "tab", tr("без агента", "skip the agent"), al*0.5)
		}
		return ky - y + 2
	case SubURL:
		f.fade(x, y, tr("адрес OpenAI-совместимого API", "OpenAI-compatible API address"), m.th.dim, al, false)
		f.fade(x, y+1, "›", m.th.a1, al, true)
		url := a.url
		if n := max(0, w-6); len(url) > n {
			url = url[len(url)-n:]
		}
		n := f.fade(x+2, y+1, string(url), m.th.text, al, false)
		f.fade(x+2+n, y+1, "▎", m.th.a1, al*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		if a.errT >= 0 && a.t-a.errT < 1.6 && len(a.url) == 0 {
			need := tr("нужен адрес", "address needed")
			f.fade(x+w-len([]rune(need)), y+1, need, m.th.bad, al, false)
		}
		dx := key(x, y+2, "enter", tr("дальше", "continue"), al)
		key(x+dx, y+2, "esc", tr("назад", "back"), al)
		return 4
	case SubKey:
		title, right := tr("ключ ", "key ")+p.name, p.keys
		if p.keys == "" {
			title, right = tr("ключ, если нужен", "key, if needed"), ""
		}
		f.fade(x, y, title, m.th.dim, al, false)
		if len([]rune(title))+len([]rune(right))+2 <= w {
			f.fade(x+w-len([]rune(right)), y, right, m.th.dim, al*0.7, false)
		}
		f.fade(x, y+1, "›", m.th.a1, al, true)
		masked := []rune(string(a.key[:min(6, len(a.key))]) + strings.Repeat("•", max(0, len(a.key)-6)))
		if n := max(0, w-6); len(masked) > n {
			masked = masked[len(masked)-n:]
		}
		n := f.fade(x+2, y+1, string(masked), m.th.text, al, false)
		f.fade(x+2+n, y+1, "▎", m.th.a1, al*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		switch {
		case a.errMsg != "":
			f.fade(x+w-len([]rune(trunc(a.errMsg, w))), y+1, trunc(a.errMsg, w), m.th.bad, al, false)
		case a.errT >= 0 && a.t-a.errT < 1.6 && len(a.key) == 0:
			need := tr("нужен ключ", "key needed")
			f.fade(x+w-len([]rune(need)), y+1, need, m.th.bad, al, false)
		}
		dx := key(x, y+2, "enter", tr("проверить", "verify"), al)
		dx += key(x+dx, y+2, "esc", tr("назад", "back"), al)
		if a.errMsg != "" && a.saved != nil {
			key(x+dx, y+2, "tab", tr("сохранить всё равно", "save anyway"), al)
		}
		return 4
	case SubLoading:
		m.wave(f, x, y, tr("спрашиваю список моделей", "fetching models"), m.th.a1)
		return 2
	case SubModel:
		list := a.models()
		f.fade(x, y, tr("модель", "model"), m.th.dim, al, false)
		cnt := fmt.Sprintf(tr("%d с инструментами из %d", "%d with tools of %d"), len(a.list), a.total)
		f.fade(x+w-len([]rune(cnt)), y, cnt, m.th.dim, al*0.7, false)
		f.fade(x, y+1, "/", m.th.a1, al, true)
		f.fade(x+1, y+1, string(a.filter), m.th.text, al, false)
		f.fade(x+1+len(a.filter), y+1, "▎", m.th.a1, al*(0.35+0.65*(0.5+0.5*math.Sin(m.t*6))), false)
		if len(a.filter) == 0 {
			f.fade(x+2+len(a.filter), y+1, tr("поиск", "search"), m.th.dim, al*0.5, false)
		}
		rows := max(2, min(6, maxH-7))
		start := max(0, min(a.model-rows/2, len(list)-rows))
		if len(list) == 0 {
			f.fade(x, y+2, tr("ничего не нашлось", "nothing found"), m.th.dim, al, false)
		}
		for k := 0; k < rows && start+k < len(list); k++ {
			idx, mi := start+k, list[start+k]
			id, idc := mi.ID, m.th.dim
			if idx == a.model {
				f.fade(x, y+2+k, "›", m.th.a1, al, true)
				idc = m.th.text
			}
			if mi.Rec {
				mark := tr("рекомендуем", "recommended")
				id = trunc(mi.ID, w-2-len([]rune(mark))-2)
				f.fade(x+w-len([]rune(mark)), y+2+k, mark, m.th.a1, al*0.8, false)
			}
			f.fade(x+2, y+2+k, id, idc, al, false)
		}
		dx := key(x, y+2+rows, "enter", tr("выбрать", "choose"), al)
		key(x+dx, y+2+rows, "esc", tr("назад", "back"), al)
		return rows + 4
	case SubEffort:
		short := a.picked
		if i := strings.LastIndex(short, "/"); i >= 0 {
			short = short[i+1:]
		}
		f.fade(x, y, tr("рассуждение", "reasoning"), m.th.dim, al, false)
		f.fade(x+w-len([]rune(short)), y, short, m.th.dim, al*0.7, false)
		ux, sx := x, x
		for i, e := range efforts {
			if i == a.effort {
				sx = ux
				f.fade(ux, y+1, e, m.th.a1, al, true)
			} else {
				f.fade(ux, y+1, e, m.th.dim, al, false)
			}
			ux += len([]rune(e)) + 3
		}
		f.fade(sx, y+2, strings.Repeat("▔", len([]rune(efforts[a.effort]))), m.th.a1, al*0.8, false)
		ex := [...]string{
			tr("быстрее, подборки проще", "faster, simpler picks"),
			tr("баланс скорости и точности", "balanced"),
			tr("точнее, но дольше", "sharper, but slower"),
		}[a.effort]
		f.fade(x, y+3, ex, m.th.dim, al, false)
		dx := key(x, y+4, "←→", tr("выбрать", "choose"), al)
		key(x+dx, y+4, "enter", tr("готово", "done"), al)
		return 6
	}
	return 0
}
