package stage

import "strings"

// IGOR: flat pink, a black-and-white cut-out, a typewritten credit line. The pink of
// the cover bleeds into the screen so the portrait floats on it, the credit types
// itself under the cover, and the pink drains out along the story of the album:
// bright at IGOR'S THEME, nearly grey by ARE WE STILL FRIENDS?
func init() {
	eggRules = append(eggRules, eggRule{artist: "Tyler, The Creator", album: "IGOR", e: egg{
		theme: &igorTheme, bg: igorBg, cover: igorCover, ink: igorInk, over: igorOver,
		knob: '●', knobCol: hex(0x161616)}})
}

var igorTheme = theme{
	bg:   hex(0xefa7bf),
	a1:   hex(0x161616),
	a2:   hex(0x6e5560),
	a3:   hex(0x161616),
	text: hex(0x161616),
	dim:  hex(0x5e4650),
	bad:  hex(0xb0203a),
}

var (
	igorPinkC = hex(0xefa7bf)
	igorAsh   = hex(0xcbc6c8)
	igorType  = hex(0x1e1e1e)
)

var igorTracks = []string{"IGOR'S THEME", "EARFQUAKE", "I THINK",
	"EXACTLY WHAT YOU RUN FROM YOU END UP CHASING", "RUNNING OUT OF TIME", "NEW MAGIC WAND",
	"A BOY IS A GUN*", "PUPPET", "WHAT'S GOOD", "GONE, GONE / THANK YOU",
	"I DON'T LOVE YOU ANYMORE", "ARE WE STILL FRIENDS?"}

var igorCredit = []string{"ALL SONGS WRITTEN, PRODUCED AND ARRANGED BY", "TYLER OKONMA"}

// igorPink is the pink of the current track: it drains toward ash along the album.
func igorPink(m *Model) col {
	k := foldTitle(m.curTrack().Title)
	for i, t := range igorTracks {
		if foldTitle(t) == k {
			return igorPinkC.lerp(igorAsh, 0.8*float64(i)/float64(len(igorTracks)-1))
		}
	}
	return igorPinkC
}

func igorBg(m *Model, c col, x, y int) col {
	return igorPink(m).mul(0.985 + 0.03*hash(x, y, 63))
}

// igorCover: the pink paper of the cover takes the pink of the screen, so only the
// cut-out portrait stands on it.
func igorCover(m *Model, at func(px, py int) col, px, py int) col {
	c := at(px, py)
	h, s, v := toHSV(c)
	if (h > 315 || h < 10) && s > 0.15 && s < 0.5 && v > 0.7 {
		return igorPink(m).mul(v / 0.94)
	}
	return c
}

// igorInk: the title in flat black.
func igorInk(m *Model, c col, u, v float64) col { return igorTheme.text }

// igorOver types the credit line of the cover under the cover, a key at a time, the
// cursor blinking a while after the last letter.
func igorOver(m *Model, f *fb, g playGeom) {
	y := g.cy + g.ch + 1
	if !g.side || g.mini || g.split || y+len(igorCredit) >= m.h-2 {
		return
	}
	n := int((m.trackT - 1.5) * 16)
	if n < 0 {
		return
	}
	for i, line := range igorCredit {
		rs := []rune(line)
		if len(rs) > g.cw {
			rs = []rune(trunc(line, g.cw))
		}
		x := g.cx + (g.cw-len(rs))/2
		shown := min(n, len(rs))
		f.text(x, y+i, string(rs[:shown]), igorType)
		typing := n <= len(rs)
		if typing || i == len(igorCredit)-1 && m.trackT < 1.5+float64(len(strings.Join(igorCredit, "")))/16+3 {
			if typing || int(m.trackT*2)%2 == 0 {
				f.put(x+shown, y+i, '█', igorType)
			}
		}
		n -= len(rs)
		if n < 0 {
			return
		}
	}
}
