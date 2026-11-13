package stage

import (
	"strings"
	"testing"
)

func mlEggSongs(t *testing.T, m *Model, r *recorder) {
	t.Helper()
	m.JumpPlay(threeTracks(), 0, 0)
	m.Key("m", "m")
	steps(m, 0.2)
	mlOpenSection(t, m, MusicSongs)
	plain := musicSong("s1", "Alpha", "Plainsong", 157)
	plain.Album = "Nothing Special"
	carti := musicSong("s2", "Playboi Carti", "Stop Breathing", 218)
	carti.Album = "Whole Lotta Red (Deluxe)"
	m.MusicRows(mlLastLoad(t, r).Gen, []MusicItem{plain, carti}, "")
	steps(m, 0.5)
}

func TestPaletteEggTheme(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	mlEggSongs(t, m, r)
	if m.to.a1 == wlr.a1 {
		t.Fatal("a plain song already wears the Whole Lotta Red palette")
	}
	m.Key("down", "")
	steps(m, 0.05)
	if m.to != wlr {
		t.Errorf("a song of Whole Lotta Red highlighted: theme %+v, want wlr", m.to)
	}

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	mlOpenSection(t, m2, MusicAlbums)
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{
		musicAlbum("a1", "Alpha", "Plain", 10),
		musicAlbum("a2", "Playboi Carti", "Whole Lotta Red", 24),
	}, "")
	m2.Key("right", "")
	steps(m2, 0.05)
	if m2.to != wlr {
		t.Errorf("the Whole Lotta Red tile highlighted: theme %+v, want wlr", m2.to)
	}
}

func TestPaletteEggMark(t *testing.T) {
	m, r := paletteSetup(t, 160, 45, "palette")
	mlEggSongs(t, m, r)
	if y := mlRowOf(m, "Stop Breathing"); y < 0 || !strings.Contains(rowText(m.render(), y, 0, m.w), "Stop Breathing †") {
		t.Errorf("the Whole Lotta Red row carries no † mark: %q", rowText(m.render(), y, 0, m.w))
	}
	if y := mlRowOf(m, "Plainsong"); strings.ContainsAny(rowText(m.render(), y, 0, m.w), "†✦◆") {
		t.Errorf("a plain row carries an egg mark: %q", rowText(m.render(), y, 0, m.w))
	}
	m.Key("down", "")
	steps(m, 0.5)
	if n := strings.Count(stripANSI(m.View()), "†"); n < 2 {
		t.Errorf("the detail of the Whole Lotta Red song carries no mark: %d marks on screen", n)
	}

	m2, r2 := paletteSetup(t, 160, 45, "palette")
	m2.JumpPlay(threeTracks(), 0, 0)
	m2.Key("m", "m")
	steps(m2, 0.2)
	mlOpenSection(t, m2, MusicAlbums)
	m2.MusicRows(mlLastLoad(t, r2).Gen, []MusicItem{
		musicAlbum("a1", "Alpha", "Plain", 10),
		musicAlbum("a2", "Lil Uzi Vert", "Pink Tape", 26),
	}, "")
	if y := mlRowOf(m2, "Lil Uzi"); y < 0 || !strings.Contains(rowText(m2.render(), y, 0, m2.w), "◆") {
		t.Errorf("the Lil Uzi Vert tile carries no ◆ mark: %q", rowText(m2.render(), y, 0, m2.w))
	}

	r3 := &recorder{}
	m3 := New(Options{MusicStyle: "palette", NoEggs: true, OnAction: r3.on})
	m3.SetSize(160, 45)
	mlEggSongs(t, m3, r3)
	if strings.Contains(stripANSI(m3.View()), "†") {
		t.Error("with eggs off the palette still marks Whole Lotta Red")
	}
}

func TestPaletteEggGlint(t *testing.T) {
	cover := func(m *Model) []col {
		f := m.render()
		px, py, pw, _ := m.mlPaletteGeom()
		x0 := px + 1 + m.mlListW(pw-2) + musicDetailGap
		var out []col
		for y := py + 3; y < py+3+musicCoverY; y++ {
			for x := x0; x < x0+musicCoverX; x++ {
				if c := f.at(x, y); c != nil {
					out = append(out, c.fg, c.bg)
				}
			}
		}
		return out
	}
	differs := func(a, b []col) bool {
		for i := range a {
			if i < len(b) && mlDist(a[i], b[i]) > 0.02 {
				return true
			}
		}
		return false
	}

	m, r := paletteSetup(t, 160, 45, "palette")
	mlEggSongs(t, m, r)
	m.Key("down", "")
	m.render()
	steps(m, 1)
	mid := cover(m)
	steps(m, 1.5)
	quiet := cover(m)
	if !differs(mid, quiet) {
		t.Error("no light sweeps the cover of the Whole Lotta Red song")
	}

	m.Key("up", "")
	m.render()
	steps(m, 1)
	mid = cover(m)
	steps(m, 1.5)
	quiet = cover(m)
	if differs(mid, quiet) {
		t.Error("the cover of a plain song is swept by the egg glint")
	}
}
