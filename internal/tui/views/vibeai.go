package views

import (
	"fmt"
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/simone-vibes/vibez/internal/tui/styles"
)

type VibeAIQuestion struct {
	Question    string
	Kind        string
	Options     []string
	Left, Right string
	Steps       int
	Default     string
}

type VibeAITrack struct {
	ID                              string
	Artist, Title, Album, Year, Why string
	Favorite                        bool
}

// VibeAISubmitMsg asks the model to run an agent turn. Fresh starts a new
// session (a new request); otherwise Text is a reaction within the session.
type VibeAISubmitMsg struct {
	Text  string
	Fresh bool
}

// VibeAIAnswersMsg carries raw answers in the agent's format:
// "" = default, "N" = option number, anything else = the user's own answer.
type VibeAIAnswersMsg struct{ Answers []string }

// VibeAILaterMsg reports an answer to a mid-stream question: Raw is the option
// number the user typed ("1".."N"); esc dismisses the question without a msg.
type VibeAILaterMsg struct{ Raw string }

type VibeAICancelMsg struct{}

type VibeAIAction int

const (
	VibeAIPlay VibeAIAction = iota
	VibeAIQueue
	VibeAISave
	VibeAIStream
)

// VibeAIActionMsg carries play/queue/save/stream commands. For Play, Index is
// the ordinal of a feed track (0-based across every portion).
type VibeAIActionMsg struct {
	Action VibeAIAction
	Index  int
}

// VibeAIRateMsg carries the owner's rating of a feed track: Index is its
// ordinal across every portion, Rating is 1 (more), -1 (less) or 0 (cleared).
type VibeAIRateMsg struct {
	Index, Rating int
}

type vibeAIScreen int

const (
	aiIntro vibeAIScreen = iota
	aiFeed
	aiFailed
)

type vibeAIRowKind int

const (
	rowDivider vibeAIRowKind = iota
	rowTrack
	rowAgent
	rowUser
	rowQuestion
	rowDone
	rowError
)

type vibeAITrackState int

const (
	trackUpcoming vibeAITrackState = iota
	trackPlaying
	trackPlayed
	trackSkipped
)

type vibeAIRow struct {
	kind   vibeAIRowKind
	text   string
	n      int
	track  VibeAITrack
	state  vibeAITrackState
	skipAt int
	rating int
	q      VibeAIQuestion
	qCur   int
	later  bool
	born   int
}

const (
	vibeAIPlanned    = 20
	vibeAITickerKeep = 4
)

var vibeAIExamples = []string{"в духе HARDSTONE PSYCHO", "вечерняя прогулка", "как Don Toliver, но не он", "бумбэп под работу"}

var thinkingBear = []string{"ʕ•ᴥ•ʔ", "ʕ·ᴥ·ʔ", "ʕ•ᴥ•ʔ", "ʕ˘ᴥ˘ʔ"}

type VibeAIModel struct {
	screen        vibeAIScreen
	width, height int
	step          int
	input         textinput.Model
	typing        bool
	example       int
	histSel       int

	query   string
	history []string

	feed     []vibeAIRow
	cursor   int
	portionN int
	open     bool
	openAt   int

	building bool
	status   string
	thinking string
	ticker   []string
	got      int

	qQueue   []VibeAIQuestion
	qAnswers []string

	memory   []string
	playlist string
	stream   bool
	notice   string
	errText  string
}

// NewVibeAI returns the vibe panel on its intro screen with the request input
// focused.
func NewVibeAI() *VibeAIModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "что хочется послушать?"
	ti.CharLimit = 200
	v := &VibeAIModel{input: ti, cursor: -1, stream: true}
	v.startTyping("")
	return v
}

func (v *VibeAIModel) SetSize(w, h int) { v.width, v.height = w, h }
func (v *VibeAIModel) SetStep(step int) { v.step = step }

// Capturing reports whether every key belongs to the panel (text entry or a
// blocking warmup question), so global shortcuts must not fire.
func (v *VibeAIModel) Capturing() bool { return v.typing || v.warmRow() >= 0 }

// Busy reports whether the agent is picking tracks or asking warmup questions.
func (v *VibeAIModel) Busy() bool { return v.building || v.warmRow() >= 0 }

func (v *VibeAIModel) Query() string { return v.query }

// Back consumes esc inside the panel; false hands the key to global shortcuts.
func (v *VibeAIModel) Back() bool {
	switch {
	case v.typing && v.screen == aiFeed:
		v.stopTyping()
		return true
	case v.screen == aiFailed && v.trackCount() > 0:
		v.screen = aiFeed
		return true
	}
	return false
}

func (v *VibeAIModel) startTyping(placeholder string) {
	v.typing = true
	v.histSel = 0
	v.input.SetValue("")
	v.input.Placeholder = placeholder
	v.input.Focus()
}

func (v *VibeAIModel) stopTyping() {
	v.typing = false
	v.input.Blur()
}

// SetWorking marks the start of an agent turn without opening a portion.
func (v *VibeAIModel) SetWorking(status string) {
	v.screen = aiFeed
	v.stopTyping()
	v.building = true
	v.status = status
	v.thinking = ""
	v.ticker = nil
	v.got = 0
	v.notice = ""
}

func (v *VibeAIModel) openPortion() {
	if v.open {
		return
	}
	v.portionN++
	v.feed = append(v.feed, vibeAIRow{kind: rowDivider, n: v.portionN, born: v.step})
	v.open = true
	v.openAt = len(v.feed) - 1
}

// AddProgress turns the agent's raw trace into the live status line: lines
// without the "·" prefix replace the status, "·" lines become short thoughts.
func (v *VibeAIModel) AddProgress(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	if !strings.HasPrefix(line, "·") {
		v.status = line
		return
	}
	v.thinking = thinkingPhrase(strings.TrimSpace(strings.TrimPrefix(line, "·")))
}

func thinkingPhrase(rest string) string {
	name, args, _ := strings.Cut(rest, " ")
	switch name {
	case "describe":
		_, what, _ := strings.Cut(args, " ")
		if what == "" {
			what = args
		}
		return "читаю про " + what
	case "set_plan":
		return "наметил план"
	case "similar_artists":
		return "смотрю соседей " + args
	case "submit":
		return "проверяю в Apple Music…"
	case "ask_user":
		return "спросил тебя"
	case "ask_later":
		return "придумал вопрос"
	case "memory_add", "memory_replace":
		return "запомнил: " + args
	case "memory_remove":
		return "забыл: " + args
	case "memory_consolidate":
		return "навожу порядок в памяти"
	}
	return rest
}

// Ask queues blocking warmup questions; the first one lands in the feed as a
// question row and captures keys until it is answered.
func (v *VibeAIModel) Ask(qs []VibeAIQuestion) {
	v.qAnswers = make([]string, 0, len(qs))
	v.stopTyping()
	if len(qs) == 0 {
		v.qQueue = nil
		return
	}
	v.qQueue = append(v.qQueue[:0:0], qs[1:]...)
	v.feed = append(v.feed, vibeAIRow{kind: rowQuestion, q: qs[0], qCur: questionDefault(qs[0]), born: v.step})
}

func (v *VibeAIModel) warmRow() int {
	for i := len(v.feed) - 1; i >= 0; i-- {
		if r := v.feed[i]; r.kind == rowQuestion && !r.later {
			return i
		}
	}
	return -1
}

func (v *VibeAIModel) laterIdx() int {
	for i := len(v.feed) - 1; i >= 0; i-- {
		if r := v.feed[i]; r.kind == rowQuestion && r.later {
			return i
		}
	}
	return -1
}

func questionDefault(q VibeAIQuestion) int {
	if q.Kind == "scale" {
		n, err := strconv.Atoi(q.Default)
		if err != nil || n < 1 || n > q.Steps {
			return (q.Steps + 1) / 2
		}
		return n
	}
	if i := slices.Index(q.Options, q.Default); i >= 0 {
		return i
	}
	return 0
}

func questionDefaultShown(q VibeAIQuestion) string {
	if q.Kind == "scale" {
		return strconv.Itoa(questionDefault(q)) + " из " + strconv.Itoa(q.Steps)
	}
	return q.Default
}

func (v *VibeAIModel) warmAnswer(raw, shown string) tea.Cmd {
	i := v.warmRow()
	r := &v.feed[i]
	r.kind = rowDone
	r.text = r.q.Question + ": " + shown
	v.qAnswers = append(v.qAnswers, raw)
	if len(v.qQueue) > 0 {
		next := v.qQueue[0]
		v.qQueue = v.qQueue[1:]
		v.feed = append(v.feed, vibeAIRow{kind: rowQuestion, q: next, qCur: questionDefault(next), born: v.step})
		return nil
	}
	answers := v.qAnswers
	v.qAnswers = nil
	v.qQueue = nil
	return func() tea.Msg { return VibeAIAnswersMsg{Answers: answers} }
}

// AskLater shows one mid-stream question right after the currently playing
// track (or at the end); keys stay with the feed.
func (v *VibeAIModel) AskLater(q VibeAIQuestion) {
	if i := v.laterIdx(); i >= 0 {
		v.feed = slices.Delete(v.feed, i, i+1)
	}
	at := len(v.feed)
	for i, r := range v.feed {
		if r.kind == rowTrack && r.state == trackPlaying {
			at = i + 1
			break
		}
	}
	v.feed = append(v.feed[:at], append([]vibeAIRow{{kind: rowQuestion, q: q, qCur: 0, later: true, born: v.step}}, v.feed[at:]...)...)
}

// AddPick reports one candidate track; rejected names land in the ticker,
// accepted ones open a portion and appear in the feed right away.
func (v *VibeAIModel) AddPick(t VibeAITrack, name string, accepted bool, reason string) {
	item := "+ " + t.Artist
	if !accepted {
		item = "− " + name + ": " + reason
	} else {
		v.openPortion()
		v.feed = append(v.feed, vibeAIRow{kind: rowTrack, track: t, state: trackUpcoming, born: v.step})
		if v.cursor == -1 {
			v.cursor = 0
		}
		v.got++
	}
	v.ticker = append(v.ticker, item)
	if len(v.ticker) > vibeAITickerKeep {
		v.ticker = v.ticker[len(v.ticker)-vibeAITickerKeep:]
	}
}

// PortionDone settles the open portion: the divider gets its note and the
// picks are replaced by the final tracks (state survives per ID).
func (v *VibeAIModel) PortionDone(tracks []VibeAITrack, note string) {
	v.openPortion()
	v.feed[v.openAt].text = note
	prev := map[string]vibeAIRow{}
	rest := make([]vibeAIRow, 0, len(v.feed))
	for _, r := range v.feed[v.openAt+1:] {
		if r.kind == rowTrack {
			if _, ok := prev[r.track.ID]; !ok {
				prev[r.track.ID] = r
			}
		} else {
			rest = append(rest, r)
		}
	}
	fresh := make([]vibeAIRow, 0, len(tracks))
	for _, t := range tracks {
		row := vibeAIRow{kind: rowTrack, track: t, born: v.step}
		if kept, ok := prev[t.ID]; ok {
			row.state, row.skipAt, row.born = kept.state, kept.skipAt, kept.born
			row.rating = kept.rating
		}
		fresh = append(fresh, row)
	}
	out := make([]vibeAIRow, 0, len(v.feed))
	out = append(out, v.feed[:v.openAt+1]...)
	out = append(out, fresh...)
	out = append(out, rest...)
	v.feed = out
	v.building = false
	v.open = false
	v.clampCursor()
}

// SetPlaying marks the newest feed row of id as playing and releases any other
// playing row back to upcoming.
func (v *VibeAIModel) SetPlaying(id string) {
	for i := range v.feed {
		r := &v.feed[i]
		if r.kind == rowTrack && r.state == trackPlaying && r.track.ID != id {
			r.state = trackUpcoming
		}
	}
	for i := len(v.feed) - 1; i >= 0; i-- {
		r := &v.feed[i]
		if r.kind == rowTrack && r.track.ID == id {
			r.state = trackPlaying
			return
		}
	}
}

// FollowPlaying puts the cursor on the feed row of id when it has one.
func (v *VibeAIModel) FollowPlaying(id string) {
	i := v.lastTrack(id)
	if i < 0 {
		return
	}
	n := 0
	for _, r := range v.feed[:i] {
		if r.kind == rowTrack {
			n++
		}
	}
	v.cursor = n
}

// SetPlayed marks the newest feed row of id as listened to the end.
func (v *VibeAIModel) SetPlayed(id string) {
	if i := v.lastTrack(id); i >= 0 {
		v.feed[i].state = trackPlayed
	}
}

// SetSkipped marks the newest feed row of id as skipped at posSec.
func (v *VibeAIModel) SetSkipped(id string, posSec int) {
	if i := v.lastTrack(id); i >= 0 {
		v.feed[i].state = trackSkipped
		v.feed[i].skipAt = posSec
	}
}

func (v *VibeAIModel) lastTrack(id string) int {
	for i := len(v.feed) - 1; i >= 0; i-- {
		if r := v.feed[i]; r.kind == rowTrack && r.track.ID == id {
			return i
		}
	}
	return -1
}

func (v *VibeAIModel) trackCount() int {
	n := 0
	for _, r := range v.feed {
		if r.kind == rowTrack {
			n++
		}
	}
	return n
}

func (v *VibeAIModel) clampCursor() {
	n := v.trackCount()
	switch {
	case n == 0:
		v.cursor = -1
	case v.cursor < 0:
		v.cursor = 0
	case v.cursor >= n:
		v.cursor = n - 1
	}
}

func (v *VibeAIModel) dropOpenPortion() {
	if !v.open {
		return
	}
	d := v.openAt
	out := make([]vibeAIRow, 0, len(v.feed)-1)
	for i, r := range v.feed {
		if i == d || (i > d && r.kind == rowTrack) {
			continue
		}
		out = append(out, r)
	}
	v.feed = out
	v.portionN--
	v.open = false
	v.openAt = 0
}

// SetError aborts the turn: the open portion disappears and the panel shows
// either the whole feed with an error row or the failed screen.
func (v *VibeAIModel) SetError(err error) {
	v.stopTyping()
	v.building = false
	v.dropOpenPortion()
	v.clampCursor()
	v.errText = err.Error()
	if v.trackCount() == 0 {
		v.screen = aiFailed
		return
	}
	v.screen = aiFeed
	v.feed = append(v.feed, vibeAIRow{kind: rowError, text: err.Error(), born: v.step})
}

// Cancelled aborts the turn the same way but brings back the intro screen when
// the feed is empty.
func (v *VibeAIModel) Cancelled() {
	v.dropOpenPortion()
	v.building = false
	v.clampCursor()
	if i := v.warmRow(); i >= 0 {
		v.feed = slices.Delete(v.feed, i, i+1)
		v.qQueue = nil
		v.qAnswers = nil
	}
	if v.trackCount() == 0 {
		v.screen = aiIntro
		v.startTyping("")
		v.input.SetValue(v.query)
		return
	}
	v.SetNotice("отменено")
}

func (v *VibeAIModel) SetPlaylist(name string) { v.playlist = name }
func (v *VibeAIModel) SetStream(on bool)       { v.stream = on }
func (v *VibeAIModel) SetNotice(s string)      { v.notice = s }
func (v *VibeAIModel) SetMemory(lines []string) {
	v.memory = append(v.memory[:0:0], lines...)
}

// HandleKey processes a key; handled=false lets the model apply its global
// shortcuts (playback, panel switching, esc-to-close).
func (v *VibeAIModel) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := msg.String()
	if v.typing {
		return v.typingKey(msg, k)
	}
	if v.warmRow() >= 0 {
		return v.warmKey(k), true
	}
	if i := v.laterIdx(); i >= 0 {
		if cmd, ok := v.laterKey(i, k); ok {
			return cmd, true
		}
	}
	switch v.screen {
	case aiFeed:
		return v.feedKey(k)
	case aiFailed:
		if k == "enter" || k == "x" {
			v.toIntro()
			return nil, true
		}
	case aiIntro:
		if k != "esc" {
			v.startTyping("")
			return nil, true
		}
	}
	return nil, false
}

func (v *VibeAIModel) toIntro() {
	v.screen = aiIntro
	v.startTyping("")
}

func (v *VibeAIModel) typingKey(msg tea.KeyPressMsg, k string) (tea.Cmd, bool) {
	switch k {
	case "enter":
		text := strings.TrimSpace(v.input.Value())
		if text == "" {
			return nil, true
		}
		switch {
		case v.warmRow() >= 0:
			v.stopTyping()
			return v.warmAnswer(text, text), true
		case v.screen == aiIntro:
			return v.startRequest(text), true
		case v.screen == aiFeed:
			v.feed = append(v.feed, vibeAIRow{kind: rowUser, text: text, born: v.step})
			v.SetWorking("")
			return func() tea.Msg { return VibeAISubmitMsg{Text: text, Fresh: false} }, true
		}
	case "esc":
		if v.warmRow() >= 0 {
			v.stopTyping()
			return nil, true
		}
		if v.screen == aiFeed {
			v.stopTyping()
			return nil, true
		}
		return nil, false
	case "tab":
		if v.screen == aiIntro {
			v.input.SetValue(vibeAIExamples[v.example%len(vibeAIExamples)])
			v.input.CursorEnd()
			v.example++
			return nil, true
		}
	case "up", "down":
		if v.screen == aiIntro && len(v.history) > 0 && (k == "down" && v.histSel < len(v.history) || k == "up" && v.histSel > 0) {
			if k == "down" {
				v.histSel++
			} else {
				v.histSel--
			}
			if v.histSel > 0 {
				v.input.SetValue(v.history[v.histSel-1])
				v.input.CursorEnd()
			}
		}
		return nil, true
	}
	var cmd tea.Cmd
	v.input, cmd = v.input.Update(msg)
	return cmd, true
}

func (v *VibeAIModel) startRequest(text string) tea.Cmd {
	v.stopTyping()
	v.query = text
	v.history = append([]string{text}, removeString(v.history, text)...)
	v.histSel = 0
	v.feed = []vibeAIRow{{kind: rowUser, text: text, born: v.step}}
	v.portionN = 0
	v.open = false
	v.openAt = 0
	v.cursor = -1
	v.building = false
	v.status = ""
	v.thinking = ""
	v.ticker = nil
	v.got = 0
	v.qQueue = nil
	v.qAnswers = nil
	v.errText = ""
	v.notice = ""
	v.screen = aiFeed
	return func() tea.Msg { return VibeAISubmitMsg{Text: text, Fresh: true} }
}

func (v *VibeAIModel) warmKey(k string) tea.Cmd {
	i := v.warmRow()
	r := &v.feed[i]
	q := r.q
	if k == "t" {
		v.startTyping("свой вариант…")
		return nil
	}
	if q.Kind == "scale" {
		switch k {
		case "left", "h":
			r.qCur = max(1, r.qCur-1)
		case "right", "l":
			r.qCur = min(q.Steps, r.qCur+1)
		case "enter", "space":
			return v.warmAnswer(strconv.Itoa(r.qCur), strconv.Itoa(r.qCur)+" из "+strconv.Itoa(q.Steps))
		case "esc":
			return v.warmAnswer("", questionDefaultShown(q))
		default:
			if d, err := strconv.Atoi(k); err == nil && d >= 1 && d <= q.Steps {
				return v.warmAnswer(k, strconv.Itoa(d)+" из "+strconv.Itoa(q.Steps))
			}
		}
		return nil
	}
	n := len(q.Options)
	switch k {
	case "left", "h":
		if n > 0 {
			r.qCur = (r.qCur - 1 + n) % n
		}
	case "right", "l":
		if n > 0 {
			r.qCur = (r.qCur + 1) % n
		}
	case "enter", "space":
		if n == 0 {
			return nil
		}
		return v.warmAnswer(strconv.Itoa(r.qCur+1), q.Options[r.qCur])
	case "esc":
		return v.warmAnswer("", questionDefaultShown(q))
	default:
		if d, err := strconv.Atoi(k); err == nil && d >= 1 && d <= n {
			return v.warmAnswer(k, q.Options[d-1])
		}
	}
	return nil
}

func (v *VibeAIModel) laterKey(i int, k string) (tea.Cmd, bool) {
	switch {
	case k == "esc":
		v.feed = slices.Delete(v.feed, i, i+1)
		return nil, true
	default:
		q := v.feed[i].q
		d, err := strconv.Atoi(k)
		if err != nil || d < 1 || d > len(q.Options) {
			return nil, false
		}
		r := &v.feed[i]
		r.kind = rowDone
		r.later = false
		r.text = q.Question + ": " + q.Options[d-1]
		return func() tea.Msg { return VibeAILaterMsg{Raw: strconv.Itoa(d)} }, true
	}
}

func (v *VibeAIModel) feedKey(k string) (tea.Cmd, bool) {
	switch k {
	case "j", "down":
		v.moveCursor(1)
	case "k", "up":
		v.moveCursor(-1)
	case "g", "home":
		if v.trackCount() > 0 {
			v.cursor = 0
		} else {
			v.cursor = -1
		}
	case "G", "end":
		n := v.trackCount()
		if n > 0 {
			v.cursor = n - 1
		} else {
			v.cursor = -1
		}
	case "enter":
		if v.cursor >= 0 {
			i := v.cursor
			return func() tea.Msg { return VibeAIActionMsg{Action: VibeAIPlay, Index: i} }, true
		}
		return nil, true
	case "]", "[":
		want := 1
		if k == "[" {
			want = -1
		}
		if v.cursor < 0 {
			return nil, true
		}
		i := v.ordinalRow(v.cursor)
		if i < 0 {
			return nil, true
		}
		rating := want
		if v.feed[i].rating == want {
			rating = 0
		}
		v.feed[i].rating = rating
		idx := v.cursor
		return func() tea.Msg { return VibeAIRateMsg{Index: idx, Rating: rating} }, true
	case "a":
		return func() tea.Msg { return VibeAIActionMsg{Action: VibeAIQueue} }, true
	case "w":
		return func() tea.Msg { return VibeAIActionMsg{Action: VibeAISave} }, true
	case "o":
		return func() tea.Msg { return VibeAIActionMsg{Action: VibeAIStream} }, true
	case "m":
		if v.building {
			v.SetNotice("уже подбираю")
			return nil, true
		}
		v.SetWorking("")
		return func() tea.Msg { return VibeAISubmitMsg{Text: "", Fresh: false} }, true
	case "t":
		v.startTyping("скажи агенту: меньше такого, больше русского…")
	case "x":
		v.toIntro()
	default:
		return nil, false
	}
	return nil, true
}

func (v *VibeAIModel) ordinalRow(ord int) int {
	n := 0
	for i, r := range v.feed {
		if r.kind != rowTrack {
			continue
		}
		if n == ord {
			return i
		}
		n++
	}
	return -1
}

func (v *VibeAIModel) moveCursor(d int) {
	n := v.trackCount()
	if n == 0 {
		v.cursor = -1
		return
	}
	v.cursor = max(0, min(n-1, v.cursor+d))
}

func removeString(list []string, s string) []string {
	out := list[:0:0]
	for _, x := range list {
		if x != s {
			out = append(out, x)
		}
	}
	return out
}

// ── rendering ─────────────────────────────────────────────────────────────

var (
	vaiMuted  = fg(styles.ColorMuted)
	vaiText   = fg(styles.ColorFg)
	vaiBright = fg(styles.ColorGlow7)
	vaiPrim   = fg(styles.ColorPrimary)
	vaiWarm   = fg(styles.ColorAccentWarm)
	vaiGood   = fg(styles.ColorActive)
	vaiBad    = fg(styles.ColorError)
	vaiArt    = fg(styles.ColorGlow5)
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func fg(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

// View renders the active screen as exactly height lines, each no wider
// than width.
func (v *VibeAIModel) View() string {
	w, h := max(v.width, 20), max(v.height, 6)
	head := v.header(w)
	inc := v.incomingLines()
	foot := v.footer(w)
	bodyH := h - len(head) - len(inc) - len(foot)
	var body []string
	switch v.screen {
	case aiFeed:
		body = v.feedView(w, bodyH)
	case aiIntro:
		body = v.introLines(w)
		if pad := (bodyH - len(body)) / 3; pad > 0 {
			body = append(make([]string, pad), body...)
		}
	case aiFailed:
		body = v.failedLines(w)
		if pad := (bodyH - len(body)) / 3; pad > 0 {
			body = append(make([]string, pad), body...)
		}
	}
	lines := append(head, fitLines(body, bodyH)...)
	lines = append(lines, inc...)
	lines = append(lines, foot...)
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, "…")
	}
	return strings.Join(fitLines(lines, h), "\n")
}

func fitLines(lines []string, h int) []string {
	if h <= 0 {
		return nil
	}
	if len(lines) > h {
		return lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines
}

func (v *VibeAIModel) header(w int) []string {
	left := " " + RenderGlowTitle("✦ vibe", v.step)
	if v.screen == aiFeed {
		left += vaiMuted.Render("  ·  ") + vaiBright.Render(v.query)
	}
	var chips []string
	if v.screen == aiFeed {
		if v.stream {
			chips = append(chips, vaiGood.Render("● поток"))
		} else {
			chips = append(chips, vaiMuted.Render("○ поток"))
		}
		if v.playlist != "" {
			chips = append(chips, vaiWarm.Render("♫ плейлист"))
		}
		played, skipped := 0, 0
		for _, r := range v.feed {
			if r.kind != rowTrack {
				continue
			}
			switch r.state {
			case trackPlayed:
				played++
			case trackSkipped:
				skipped++
			}
		}
		if played+skipped > 0 {
			chips = append(chips, vaiGood.Render(fmt.Sprintf("✓%d", played))+" "+vaiBad.Render(fmt.Sprintf("↷%d", skipped)))
		}
	}
	return []string{spread(left, strings.Join(chips, "   ")+" ", w), v.rule(w)}
}

// rule draws a shimmering gradient divider that drifts with the glow step.
func (v *VibeAIModel) rule(w int) string {
	var sb strings.Builder
	for i := range w {
		t := float64((i+v.step)%max(w, 1)) / float64(max(w, 1))
		if t > 0.5 {
			t = 1 - t
		}
		sb.WriteString(fg(styles.LerpColor(styles.ColorGlow2, styles.ColorPrimary, t*2)).Render("━"))
	}
	return sb.String()
}

func pulse(step int) float64 {
	t := float64(step%30) / 15
	if t > 1 {
		t = 2 - t
	}
	return t
}

func (v *VibeAIModel) introLines(w int) []string {
	lines := []string{"", "", ""}
	bear := styles.BearStyle.Render(thinkingBear[(v.step/12)%len(thinkingBear)])
	lines = append(lines, "   "+bear+"  "+vaiBright.Bold(true).Render("что включаем?"), "")

	boxW := min(w-6, 76)
	v.input.SetWidth(max(boxW-6, 10))
	field := vaiMuted.Render(v.input.Value())
	border := styles.ColorSurface
	if v.typing {
		border = styles.LerpColor(styles.ColorGlow3, styles.ColorPrimary, pulse(v.step))
		field = v.input.View()
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Width(boxW).Padding(0, 1).
		Render(vaiPrim.Render("› ") + field)
	for _, l := range strings.Split(box, "\n") {
		lines = append(lines, "   "+l)
	}
	lines = append(lines, "")
	if len(v.history) > 0 {
		lines = append(lines, "", "   "+vaiWarm.Render("недавние"))
		for i, q := range head(v.history, 5) {
			bar, qSt := "  ", vaiText
			if v.histSel == i+1 {
				bar, qSt = vaiPrim.Render("▌ "), vaiBright.Bold(true)
			}
			lines = append(lines, "   "+bar+qSt.Render(short(q, 44)))
		}
	}
	lines = append(lines, "", "   "+vaiWarm.Render("что я о тебе знаю"))
	if len(v.memory) == 0 {
		lines = append(lines, "     "+vaiMuted.Render("пока ничего — узнаю по ходу"))
	}
	for _, s := range head(v.memory, 6) {
		lines = append(lines, "     "+vaiPrim.Render("·")+" "+vaiText.Render(s))
	}
	return lines
}

func head(s []string, n int) []string { return s[:min(n, len(s))] }

func (v *VibeAIModel) feedView(w, h int) []string {
	var lines []string
	curLine := 0
	num := 0
	artW := min(20, max(w/7, 12))
	titleW := min(32, max(w/4, 14))
	for i, r := range v.feed {
		fresh := v.step-r.born < 10
		if r.kind == rowTrack {
			num++
			if num-1 == v.cursor {
				curLine = len(lines)
			}
		}
		switch r.kind {
		case rowDivider:
			noteSt := vaiWarm
			if fresh {
				noteSt = fg(styles.LerpColor(styles.ColorGlow7, styles.ColorAccentWarm, float64(v.step-r.born)/10))
			}
			s := " " + vaiMuted.Render("── ") + vaiPrim.Bold(true).Render(strconv.Itoa(r.n))
			if r.text != "" {
				s += vaiMuted.Render(" · ") + noteSt.Italic(true).Render(r.text)
			}
			s += " " + vaiMuted.Render(strings.Repeat("─", max(w-lipgloss.Width(s)-2, 0)))
			lines = append(lines, "", s)
		case rowAgent:
			st := vaiText.Italic(true)
			if fresh {
				st = vaiBright.Italic(true)
			}
			lines = append(lines, " "+vaiPrim.Render("✦")+" "+st.Render(r.text))
		case rowUser:
			lines = append(lines, "   "+vaiWarm.Render("› ")+vaiBright.Render(r.text))
		case rowDone:
			lines = append(lines, "   "+vaiGood.Render("✓ ")+vaiMuted.Render(r.text))
		case rowError:
			lines = append(lines, "   "+vaiBad.Render("✗ "+r.text))
		case rowQuestion:
			lines = append(lines, v.questionLines(i, r, w)...)
		case rowTrack:
			lines = append(lines, v.trackLines(r, num, artW, titleW, w)...)
		}
	}
	top := 0
	if curLine >= h-3 {
		top = curLine - (h - 3)
	}
	if v.warmRow() >= 0 {
		top = max(top, len(lines)-h)
	}
	if top > 0 && top < len(lines) {
		lines = lines[top:]
	}
	return lines
}

func (v *VibeAIModel) questionLines(i int, r vibeAIRow, w int) []string {
	q := r.q
	glow := styles.LerpColor(styles.ColorAccentWarm, styles.ColorGlow7, pulse(v.step))
	n := len(q.Options)
	if q.Kind == "scale" {
		n = q.Steps
	}
	hint := keys("←→", "выбор", fmt.Sprintf("1-%d", n), "ответ", "enter", "ок", "t", "свой вариант", "esc", "по умолчанию")
	if r.later {
		hint = keys(fmt.Sprintf("1-%d", n), "ответ", "esc", "убрать") + vaiMuted.Render("   · музыка не ждёт")
	} else if v.warmRow() != i {
		hint = keys("←→", "выбор", "enter", "ок", "esc", "по умолчанию")
	}
	return []string{"", " " + fg(glow).Bold(true).Render("?") + " " + vaiBright.Bold(true).Render(q.Question),
		"     " + v.questionOpts(r, q), "     " + hint, ""}
}

func (v *VibeAIModel) questionOpts(r vibeAIRow, q VibeAIQuestion) string {
	if q.Kind == "scale" {
		return v.sliderView(q, r.qCur)
	}
	var parts []string
	for i, o := range q.Options {
		label := fmt.Sprintf(" %d  %s ", i+1, o)
		if i == r.qCur {
			parts = append(parts, lipgloss.NewStyle().Background(styles.ColorPrimary).Foreground(styles.ColorBg).Bold(true).Render(label))
		} else {
			parts = append(parts, lipgloss.NewStyle().Background(styles.ColorSurface).Foreground(styles.ColorFg).Render(label))
		}
	}
	return strings.Join(parts, " ")
}

func (v *VibeAIModel) sliderView(q VibeAIQuestion, val int) string {
	var sb strings.Builder
	for i := range q.Steps {
		if i+1 == val {
			sb.WriteString(vaiBright.Bold(true).Render("●"))
		} else {
			sb.WriteString(fg(styles.LerpColor(styles.ColorProgress, styles.ColorPrimary, float64(i)/float64(max(q.Steps-1, 1)))).Render("━━"))
		}
	}
	return vaiMuted.Render(q.Left+"  ") + sb.String() + vaiMuted.Render("  "+q.Right)
}

func (v *VibeAIModel) trackLines(r vibeAIRow, num, artW, titleW, w int) []string {
	sel := num-1 == v.cursor
	bar := "  "
	if sel {
		bar = vaiPrim.Render("▌ ")
	}
	numSt, aSt, tSt := vaiMuted, vaiArt, vaiText.Bold(true)
	glyph := "   "
	metaParts := strings.Join(nonEmpty(r.track.Album, r.track.Year), " · ")
	meta := vaiMuted.Render(metaParts)
	switch r.state {
	case trackPlaying:
		glyph = vaiPrim.Render(eq(v.step))
		numSt, aSt, tSt = vaiPrim, fg(styles.ColorPrimary).Bold(true), vaiBright.Bold(true)
	case trackPlayed:
		glyph = vaiGood.Faint(true).Render(" ✓ ")
		aSt, tSt = vaiMuted, vaiMuted
	case trackSkipped:
		glyph = vaiBad.Render(" ↷ ")
		aSt, tSt = vaiMuted, vaiMuted.Strikethrough(true)
		meta = vaiBad.Faint(true).Render("скип на " + mmss(r.skipAt))
	}
	if v.step-r.born < 10 && r.state == trackUpcoming {
		aSt = fg(styles.LerpColor(styles.ColorGlow7, styles.ColorGlow5, float64(v.step-r.born)/10)).Bold(true)
	}
	rate := "  "
	switch r.rating {
	case 1:
		rate = vaiGood.Render("▲ ")
	case -1:
		rate = vaiBad.Render("▼ ")
	}
	fav := "  "
	if r.track.Favorite {
		fav = fg(styles.ColorLove).Render("★ ")
	}
	line := " " + bar + numSt.Render(fmt.Sprintf("%02d", num)) + " " + glyph + " " + rate + fav +
		aSt.Render(padTo(r.track.Artist, artW)) + "  " + tSt.Render(padTo(r.track.Title, titleW)) + "  " + meta
	out := []string{line}
	if sel && r.track.Why != "" {
		out = append(out, strings.Repeat(" ", 15)+vaiMuted.Render("↳ ")+fg(styles.ColorSubtle).Italic(true).Render(r.track.Why))
	}
	return out
}

func nonEmpty(vals ...string) []string {
	var out []string
	for _, s := range vals {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (v *VibeAIModel) failedLines(w int) []string {
	lines := []string{"", "  " + styles.BearStyle.Render("ʕ•̀ᴥ•́ʔ") + "  " + vaiBad.Render("агент споткнулся"), ""}
	for _, l := range wrapText(v.errText, w-6) {
		lines = append(lines, "    "+vaiMuted.Render(l))
	}
	return append(lines, "", "  "+vaiMuted.Render("enter — новый запрос"))
}

func (v *VibeAIModel) incomingLines() []string {
	if !v.building || v.warmRow() >= 0 {
		return nil
	}
	spin := vaiPrim.Render(spinnerFrames[v.step%len(spinnerFrames)])
	thought := v.thinking
	if thought == "" {
		thought = v.status
	}
	next := v.portionN + 1
	if v.open {
		next = v.portionN
	}
	headLine := " " + spin + " " + vaiBright.Render("подбираю порцию "+strconv.Itoa(next)) +
		vaiMuted.Render(fmt.Sprintf("  ·  %d из %d  ·  ", v.got, vibeAIPlanned)) + vaiText.Italic(true).Render(thought)
	var tk []string
	for _, s := range v.ticker {
		if strings.HasPrefix(s, "+") {
			tk = append(tk, vaiGood.Render("+")+" "+vaiText.Render(strings.TrimPrefix(s, "+ ")))
		} else {
			tk = append(tk, vaiBad.Render("−")+" "+vaiMuted.Render(strings.TrimPrefix(s, "− ")))
		}
	}
	return []string{"", headLine, "   " + strings.Join(tk, vaiMuted.Render("   "))}
}

func wrapText(s string, w int) []string {
	if w < 8 {
		w = 8
	}
	wrapped := ansi.Wordwrap(s, w, "")
	return strings.Split(wrapped, "\n")
}

func (v *VibeAIModel) footer(w int) []string {
	notice := ""
	if v.notice != "" {
		notice = fg(styles.ColorActive).Render("✓ " + v.notice)
	}
	if v.typing && v.screen == aiFeed {
		v.input.SetWidth(max(w-6, 10))
		return []string{notice, " " + vaiWarm.Render("› ") + v.input.View()}
	}
	var pairs []string
	switch {
	case v.screen == aiIntro:
		pairs = []string{"enter", "поехали", "↑↓", "недавние", "tab", "пример", "esc", "закрыть"}
	case v.warmRow() >= 0:
		pairs = []string{"←→", "выбор", "enter", "ок", "t", "свой вариант", "esc", "по умолчанию"}
	case v.screen == aiFailed:
		pairs = []string{"enter", "новый запрос", "esc", "закрыть"}
	default:
		stream := "поток"
		if v.stream {
			stream = "выкл поток"
		}
		save := "в плейлист"
		if v.playlist != "" {
			save = "в ♫ плейлист"
		}
		pairs = []string{"enter", "играть отсюда", "]", "больше", "[", "меньше", "n", "скип", "space", "пауза", "t", "сказать", "m", "ещё", "a", "в очередь", "w", save, "o", stream, "x", "новый"}
	}
	return append([]string{notice}, wrapHints(pairs, w)...)
}

func wrapHints(pairs []string, w int) []string {
	maxW := max(w-1, 10)
	type hint struct {
		s string
		w int
	}
	hints := make([]hint, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		s := styles.KeyName.Render(pairs[i]) + vaiMuted.Render(" "+pairs[i+1])
		hints = append(hints, hint{s, lipgloss.Width(s)})
	}
	sep := vaiMuted.Render("  ·  ")
	sepW := lipgloss.Width(sep)
	var lines []string
	line, lineW := "", 0
	for i, hi := range hints {
		if lineW > 0 && lineW+sepW+hi.w > maxW {
			lines = append(lines, " "+line)
			line, lineW = hi.s, hi.w
			continue
		}
		if i > 0 {
			line += sep
			lineW += sepW
		}
		line += hi.s
		lineW += hi.w
	}
	if line != "" {
		lines = append(lines, " "+line)
	}
	return lines
}

func keys(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, styles.KeyName.Render(pairs[i])+vaiMuted.Render(" "+pairs[i+1]))
	}
	return strings.Join(parts, vaiMuted.Render("  ·  "))
}

func spread(left, right string, w int) string {
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func eq(f int) string {
	frames := []string{"▁▃▅", "▃▅▂", "▅▂▆", "▂▆▃", "▆▃▁", "▃▁▅"}
	return frames[(f/2)%len(frames)]
}

func padTo(s string, n int) string {
	s = short(s, n)
	return s + strings.Repeat(" ", max(n-lipgloss.Width(s), 0))
}

func short(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func mmss(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }
