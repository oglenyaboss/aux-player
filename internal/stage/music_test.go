package stage

import (
	"fmt"
	"strings"
	"testing"
)

// musicSetup is a model with a recorder, in English, at the given size.
func musicSetup(t *testing.T, w, h int) (*Model, *recorder) {
	t.Helper()
	prev := lang
	SetLang("en")
	t.Cleanup(func() { SetLang(prev) })
	r := &recorder{}
	m := New(Options{OnAction: r.on, MusicStyle: "full"})
	m.SetSize(w, h)
	return m, r
}

func musicSong(id, artist, title string, dur float64) MusicItem {
	return MusicItem{ID: id, Kind: MusicSong, Title: title, Artist: artist, Sub: "2023 · album", Dur: dur}
}

func musicAlbum(id, artist, title string, count int) MusicItem {
	return MusicItem{ID: id, Kind: MusicAlbum, Title: title, Artist: artist, Sub: "2023 · album", Count: count}
}

// musicBodyText is what the list area of the screen says, without the tabs and the strip.
func musicBodyText(m *Model) string {
	f := m.render()
	var b strings.Builder
	for y := musicBodyY; y < m.h-musicMargin; y++ {
		b.WriteString(rowText(f, y, 0, m.w))
		b.WriteByte('\n')
	}
	return b.String()
}

// musicSelected is the row the selection marker sits on.
func musicSelected(m *Model) string {
	f := m.render()
	for y := 0; y < m.h; y++ {
		if c := f.at(musicMargin, y); c != nil && c.ch == '›' {
			return rowText(f, y, musicMargin, m.w-musicMargin)
		}
	}
	return ""
}

// A1: m in the player opens the screen on Favorites and asks for its rows once; esc
// closes it back to the same track and position.
func TestMusicOpenFromPlay(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 1, 42)
	if !m.Key("m", "m") {
		t.Fatal("m in the player not taken")
	}
	if !m.MusicOpen() {
		t.Fatal("m did not open the music screen")
	}
	loads := recorded[MusicLoad](r)
	if len(loads) != 1 || loads[0].Section != MusicFavorites || loads[0].Gen != 1 {
		t.Fatalf("loads = %+v, want one MusicFavorites gen 1", loads)
	}
	if !m.Key("esc", "") {
		t.Fatal("esc not taken")
	}
	if m.MusicOpen() {
		t.Fatal("esc did not close the music screen")
	}
	if m.Phase() != PhPlay {
		t.Errorf("phase = %v, want the player", m.Phase())
	}
	if m.cur != 1 || m.pos != 42 {
		t.Errorf("cur/pos = %d/%v, want 1/42", m.cur, m.pos)
	}
}

// A2: ctrl+o opens the screen from the intro; without it m still types into the input.
func TestMusicIntroKeys(t *testing.T) {
	m, _ := musicSetup(t, 110, 32)
	if !m.Key("ctrl+o", "") {
		t.Fatal("ctrl+o on the intro not taken")
	}
	if !m.MusicOpen() {
		t.Fatal("ctrl+o did not open the music screen")
	}
	m.Key("esc", "")
	if m.MusicOpen() {
		t.Fatal("esc did not close the music screen")
	}
	m.Key("m", "m")
	m.Key("m", "m")
	if got := m.Input(); got != "mm" {
		t.Errorf("input = %q, want mm typed on the intro", got)
	}
}

// A3: right switches to the next tab and asks for it with a new generation; left back
// to a tab that already has a request out asks for nothing.
func TestMusicTabSwitch(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.Key("right", "")
	loads := recorded[MusicLoad](r)
	if len(loads) != 2 || loads[1].Section != MusicPlaylists || loads[1].Gen != 2 {
		t.Fatalf("loads = %+v, want playlists gen 2 second", loads)
	}
	m.Key("left", "")
	if loads := recorded[MusicLoad](r); len(loads) != 2 {
		t.Fatalf("left reloaded favorites: %+v", loads)
	}
}

// A4: the answer to an older generation never replaces the rows of the current tab.
func TestMusicStaleRows(t *testing.T) {
	m, _ := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.Key("right", "")
	m.MusicRows(1, []MusicItem{musicSong("f1", "Alpha", "Fav One", 200)}, "")
	body := musicBodyText(m)
	if strings.Contains(body, "Fav One") {
		t.Errorf("stale rows landed in the playlists tab:\n%s", body)
	}
	if !strings.Contains(body, "· · · ·") {
		t.Errorf("the loading rows are gone:\n%s", body)
	}
}

// A5: enter on a song inside Favorites plays the whole list from that row.
func TestMusicPlayFavorites(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	rows := []MusicItem{
		musicSong("s1", "Alpha", "One", 200),
		musicSong("s2", "Beta", "Two", 201),
		musicSong("s3", "Gamma", "Three", 202),
	}
	m.MusicRows(1, rows, "")
	m.Key("down", "")
	m.Key("enter", "")
	plays := recorded[MusicPlay](r)
	if len(plays) != 1 {
		t.Fatalf("plays = %+v, want one", r.acts)
	}
	p := plays[0]
	if len(p.Items) != 3 || p.Start != 1 || p.Items[1].ID != "s2" {
		t.Fatalf("play = %+v, want all three from the second", p)
	}
	if p.From == nil || p.From.Kind != MusicPlaylist || p.From.Title != "favorites" {
		t.Errorf("from = %+v, want the favorites collection", p.From)
	}
}

// A6: in Songs enter plays just the row.
func TestMusicPlaySingle(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	for range 3 {
		m.Key("right", "")
	}
	rows := []MusicItem{
		musicSong("s1", "Alpha", "One", 200),
		musicSong("s2", "Beta", "Two", 201),
	}
	m.MusicRows(4, rows, "")
	m.Key("down", "")
	m.Key("enter", "")
	plays := recorded[MusicPlay](r)
	if len(plays) != 1 {
		t.Fatalf("plays = %+v, want one", r.acts)
	}
	if p := plays[0]; len(p.Items) != 1 || p.Start != 0 || p.Items[0].ID != "s2" || p.From != nil {
		t.Fatalf("play = %+v, want the second row alone", p)
	}
}

// A7: enter on a playlist asks for its tracks, the list shows the breadcrumb, and esc
// puts the playlist list back as it was.
func TestMusicCollectionDrill(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.Key("right", "")
	pl := MusicItem{ID: "p1", Kind: MusicPlaylist, Title: "UTOPIA", Artist: "Travis Scott", Count: 2}
	m.MusicRows(2, []MusicItem{pl}, "")
	m.Key("enter", "")
	loads := recorded[MusicLoad](r)
	last := loads[len(loads)-1]
	if last.Gen != 3 || last.Parent == nil || last.Parent.Key() != "p1" {
		t.Fatalf("drill load = %+v, want parent p1 gen 3", last)
	}
	tracks := []MusicItem{musicSong("t1", "Travis Scott", "HYAENA", 222), musicSong("t2", "Travis Scott", "FE!N", 191)}
	m.MusicRows(3, tracks, "")
	frame := stripANSI(m.View())
	if !strings.Contains(frame, "playlists › UTOPIA") {
		t.Errorf("no breadcrumb in the frame:\n%s", frame)
	}
	if !strings.Contains(frame, "HYAENA") {
		t.Errorf("no tracks under the breadcrumb:\n%s", frame)
	}
	m.Key("esc", "")
	if m.mu.coll != nil {
		t.Fatal("esc did not leave the collection")
	}
	frame = stripANSI(m.View())
	if !strings.Contains(frame, "UTOPIA") || strings.Contains(frame, "HYAENA") {
		t.Errorf("the playlist list is not back:\n%s", frame)
	}
	if got := musicSelected(m); !strings.Contains(got, "UTOPIA") {
		t.Errorf("selection = %q, want the playlist row", got)
	}
	m.Key("enter", "")
	loads = recorded[MusicLoad](r)
	if last := loads[len(loads)-1]; last.Parent == nil || last.Parent.Key() != "p1" || last.Gen != 4 {
		t.Errorf("re-entering sent %+v, want p1 gen 4", last)
	}
}

// A8: a on a song appends it; a on a collection appends the whole of it.
func TestMusicAppend(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.Key("ctrl+o", "")
	m.MusicRows(1, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	m.Key("a", "")
	app := recorded[MusicAppend](r)
	if len(app) != 1 || app[0].Item.ID != "s1" {
		t.Fatalf("appends = %+v, want s1", app)
	}
	m.Key("right", "")
	pl := MusicItem{ID: "p1", Kind: MusicPlaylist, Title: "Mix", Count: 4}
	m.MusicRows(2, []MusicItem{pl}, "")
	m.Key("a", "")
	app = recorded[MusicAppend](r)
	if len(app) != 2 || app[1].Item.ID != "p1" {
		t.Fatalf("appends = %+v, want the playlist too", app)
	}
}

// A9: an unplayable row never plays or queues, and says why.
func TestMusicUnplayable(t *testing.T) {
	m, r := musicSetup(t, 160, 45)
	m.Key("ctrl+o", "")
	up := MusicItem{ID: "u1", Kind: MusicSong, Title: "voice memo 03", Artist: "me", Dur: 42,
		Unplayable: "upload without catalog match — can't play on Linux"}
	m.MusicRows(1, []MusicItem{musicSong("s1", "Alpha", "One", 200), up}, "")
	m.Key("down", "")
	before := len(r.acts)
	m.Key("enter", "")
	m.Key("a", "")
	if len(r.acts) != before {
		t.Fatalf("an unplayable row sent %+v, want nothing", r.acts[before:])
	}
	if frame := stripANSI(m.View()); !strings.Contains(frame, "can't play on Linux") {
		t.Errorf("the reason is not on screen:\n%s", frame)
	}
}

// A10: / focuses the search field, letters and space go there, enter asks the host and
// esc gives the keys back.
func TestMusicSearchTyping(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.Key("ctrl+o", "")
	if !m.Key("/", "/") {
		t.Fatal("/ not taken")
	}
	if m.mu.tab != MusicSearch || !m.mu.focus {
		t.Fatalf("tab = %v focus = %v, want the search field", m.mu.tab, m.mu.focus)
	}
	if !m.Key("g", "g") || !m.Key("n", "n") {
		t.Fatal("letters in the field not taken")
	}
	m.Key("enter", "")
	loads := recorded[MusicLoad](r)
	last := loads[len(loads)-1]
	if last.Section != MusicSearch || last.Query != "gn" {
		t.Fatalf("search load = %+v, want query gn", last)
	}
	m.Key("/", "/")
	if !m.Key("space", " ") {
		t.Fatal("space in the field not taken")
	}
	if q := m.mu.tabs[MusicSearch].query; q != "gn " {
		t.Errorf("query = %q, want the space typed in", q)
	}
	m.Key("esc", "")
	if m.Key("space", " ") {
		t.Error("space outside the field must reach the host")
	}
}

// A11: the search results are grouped under dim headers, the selection starts on the
// first song and walks into the album group.
func TestMusicSearchGroups(t *testing.T) {
	m, _ := musicSetup(t, 110, 32)
	m.Key("ctrl+o", "")
	m.Key("/", "/")
	m.Key("g", "g")
	m.Key("enter", "")
	rows := []MusicItem{
		musicSong("s1", "Alpha", "Song One", 200),
		musicAlbum("a1", "Beta", "Album One", 9),
		{ID: "p1", Kind: MusicPlaylist, Title: "List One", Artist: "Gamma", Count: 12},
	}
	m.MusicRows(2, rows, "")
	body := musicBodyText(m)
	for _, h := range []string{"songs", "albums", "playlists"} {
		if !strings.Contains(body, h) {
			t.Errorf("no %q group header:\n%s", h, body)
		}
	}
	if got := musicSelected(m); !strings.Contains(got, "Song One") {
		t.Errorf("selection = %q, want the first song", got)
	}
	m.Key("down", "")
	if got := musicSelected(m); !strings.Contains(got, "Album One") {
		t.Errorf("down from the last song landed on %q, want the first album", got)
	}
}

// A12: empty and failed loads are drawn, and r asks again with a new generation.
func TestMusicEmptyAndError(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.Key("ctrl+o", "")
	m.MusicRows(1, nil, "")
	if body := musicBodyText(m); !strings.Contains(body, "nothing here") {
		t.Errorf("no empty state:\n%s", body)
	}
	m.Key("/", "/")
	m.Key("z", "z")
	m.Key("z", "z")
	m.Key("enter", "")
	m.MusicRows(2, nil, "")
	if body := musicBodyText(m); !strings.Contains(body, "no matches") {
		t.Errorf("no empty search state:\n%s", body)
	}
	m.Key("left", "")
	m.MusicRows(3, nil, "Apple Music: 503")
	body := musicBodyText(m)
	if !strings.Contains(body, "503") || !strings.Contains(body, "r — retry") {
		t.Errorf("no error with a retry hint:\n%s", body)
	}
	m.Key("r", "")
	loads := recorded[MusicLoad](r)
	last := loads[len(loads)-1]
	if last.Gen != 4 || last.Section != MusicSongs || last.Parent != nil {
		t.Fatalf("retry = %+v, want songs gen 4", last)
	}
}

// D6/R12: keys the screen does not use reach the host, and the played row is marked.
func TestMusicPlayedRowAndPassthrough(t *testing.T) {
	m, _ := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.MusicRows(1, []MusicItem{musicSong("s1", "Alpha", "One", 200), musicSong("s2", "Beta", "Two", 201)}, "")
	for _, k := range []string{"n", "p", "space", "[", "]"} {
		if m.Key(k, k) {
			t.Errorf("%q must reach the host while the screen is open", k)
		}
	}

	m.Key("enter", "")
	m.JumpPlay([]Track{{ID: "s1", Artist: "Alpha", Title: "One", Dur: 200}}, 0, 0)
	steps(m, 1)
	f := m.render()
	if got := musicSelected(m); !strings.Contains(got, "One") || !strings.HasPrefix(got, "›") {
		t.Fatalf("the selected, playing row = %q, want it marked with ›", got)
	}
	y := musicRowOf(f, m, "One")
	if y < 0 {
		t.Fatalf("the playing row is not in the list")
	}
	if c := f.at(musicMargin+5, y); c == nil || c.fg != m.th.a1 {
		t.Errorf("the selected, playing title is not in the accent color")
	}
	m.Key("down", "")
	f = m.render()
	y = musicRowOf(f, m, "One")
	if y < 0 {
		t.Fatalf("the playing row left the list")
	}
	if c := f.at(musicMargin, y); c == nil || c.ch != '♪' {
		t.Errorf("the playing row is not marked with ♪ in the marker column")
	}
	if !strings.Contains(stripANSI(m.View()), "One — Alpha") {
		t.Errorf("the now-playing strip does not name the track")
	}
}

// musicRowOf is the list row that shows the title, -1 when it is not on screen.
func musicRowOf(f *fb, m *Model, title string) int {
	for y := musicBodyY; y < m.h-musicMargin; y++ {
		if strings.Contains(rowText(f, y, musicMargin, m.w-musicMargin), title) {
			return y
		}
	}
	return -1
}

// R3: a long list scrolls with the selection, and the rows carry thumbs, titles and length.
func TestMusicScrollKeepsSelection(t *testing.T) {
	m, _ := musicSetup(t, 110, 32)
	m.Key("ctrl+o", "")
	rows := make([]MusicItem, 0, 40)
	for i := range 40 {
		rows = append(rows, musicSong(fmt.Sprintf("s%02d", i), "Artist", fmt.Sprintf("Track %02d", i), 200))
	}
	m.MusicRows(1, rows, "")
	for range 30 {
		m.Key("down", "")
	}
	f := m.render()
	body := musicBodyText(m)
	if !strings.Contains(body, "Track 30") {
		t.Errorf("the selected row is not in view:\n%s", body)
	}
	if strings.Contains(body, "Track 00") {
		t.Errorf("the list did not scroll:\n%s", body)
	}
	if !strings.Contains(body, "3:20") {
		t.Errorf("no track length in the rows:\n%s", body)
	}
	thumbs := 0
	for y := musicBodyY; y < m.h-musicMargin; y++ {
		for x := musicMargin + 2; x < musicMargin+4; x++ {
			if c := f.at(x, y); c != nil && c.pix {
				thumbs++
			}
		}
	}
	if thumbs == 0 {
		t.Error("no cover thumbnails in the rows")
	}
	if got := musicSelected(m); !strings.Contains(got, "Track 30") {
		t.Errorf("selection = %q, want Track 30", got)
	}
}

// R11: the preview shows the highlight's cover and the palette follows it, then comes back.
func TestMusicPreviewTheme(t *testing.T) {
	m, _ := musicSetup(t, 160, 45)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.MusicRows(1, []MusicItem{musicSong("s1", "Alpha", "One", 200), musicSong("s2", "Beta", "Two", 201)}, "")
	m.Step(0.05)
	first := m.to
	if first != m.themeFor(Track{ID: "s1", Artist: "Alpha", Title: "One"}) {
		t.Error("the palette did not follow the highlighted row")
	}
	f := m.render()
	pixels := 0
	for y := 4; y < m.h-10; y++ {
		for x := 95; x < m.w; x++ {
			if c := f.at(x, y); c != nil && c.pix {
				pixels++
			}
		}
	}
	if pixels < 100 {
		t.Errorf("the preview column has no big cover: %d pixels", pixels)
	}
	m.Key("down", "")
	m.Step(0.05)
	if m.to != m.themeFor(Track{ID: "s2", Artist: "Beta", Title: "Two"}) || m.to == first {
		t.Error("the palette did not move with the selection")
	}
	m.Key("esc", "")
	if m.to != m.themeFor(*m.curTrack()) {
		t.Error("the palette of the player did not come back")
	}

	m2, _ := musicSetup(t, 110, 32)
	before := m2.to
	m2.Key("ctrl+o", "")
	m2.MusicRows(1, []MusicItem{musicSong("s9", "Gamma", "Nine", 200)}, "")
	m2.Step(0.05)
	if m2.to == before {
		t.Error("the palette did not follow the highlight on the intro")
	}
	m2.Key("esc", "")
	if m2.to != before {
		t.Error("closing did not bring the palette back")
	}
}

// A13: the screen survives the model reset a JumpPlay from a MusicPlay goes through.
func TestMusicSurvivesReset(t *testing.T) {
	m, r := musicSetup(t, 110, 32)
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	m.MusicRows(1, []MusicItem{musicSong("s1", "Alpha", "One", 200)}, "")
	m.Key("enter", "")
	if plays := recorded[MusicPlay](r); len(plays) != 1 {
		t.Fatalf("plays = %+v, want one", r.acts)
	}
	m.reset()
	m.JumpPlay(threeTracks(), 0, 0)
	if !m.MusicOpen() {
		t.Fatal("the reset closed the music screen")
	}
	if body := musicBodyText(m); !strings.Contains(body, "One") {
		t.Errorf("the rows are gone after the reset:\n%s", body)
	}
}

// A14: the screen fits every size without drawing into the last column.
func TestMusicSmallFrames(t *testing.T) {
	for _, sz := range [][2]int{{160, 45}, {100, 30}, {70, 20}, {50, 15}} {
		m, _ := musicSetup(t, sz[0], sz[1])
		m.JumpPlay(threeTracks(), 0, 0)
		m.Key("m", "m")
		rows := []MusicItem{
			musicAlbum("a1", "Travis Scott", "UTOPIA", 10),
			musicSong("s1", "Frank Ocean", "Nights", 307),
			musicSong("s2", "me", "voice memo 03", 42),
		}
		rows[2].Unplayable = "upload without catalog match — can't play on Linux"
		m.MusicRows(1, rows, "")
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("%dx%d: render panicked: %v", sz[0], sz[1], p)
				}
			}()
			f := m.render()
			if f.w != sz[0] || f.h != sz[1] {
				t.Fatalf("%dx%d: frame is %dx%d", sz[0], sz[1], f.w, f.h)
			}
			for y := 0; y < f.h; y++ {
				if got := len([]rune(rowText(f, y, 0, f.w))); got != f.w {
					t.Errorf("%dx%d: row %d has %d cells", sz[0], sz[1], y, got)
				}
				for x := f.w - musicMargin; x < f.w; x++ {
					if c := f.at(x, y); c != nil && c.ch != ' ' && c.ch != 0 {
						t.Errorf("%dx%d: row %d draws %q into the right margin", sz[0], sz[1], y, string(c.ch))
					}
				}
			}
			for _, y := range []int{0, m.h - 1} {
				if row := strings.TrimSpace(rowText(f, y, 0, f.w)); row != "" {
					t.Errorf("%dx%d: row %d is not empty: %q", sz[0], sz[1], y, row)
				}
			}
		}()
	}
}
