// promo renders the AUX demo video: the real stage, frame by frame, cut to the music.
//
//	go run ./lab/promo -dry                  writes the timeline for mix.py
//	go run ./lab/promo -out v.mp4 -audio a.wav   renders the film
//	go run ./lab/promo -sheet 0,5.5,16       stills of the film at those seconds
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/oglenyaboss/aux-player/internal/stage"
)

const fps = 60

type entry struct {
	stage.Track
	Key, Cover string
}

type segment struct {
	Key     string  `json:"key"`
	In      float64 `json:"in"`
	Beats   int     `json:"beats"`
	BPM     float64 `json:"bpm"`
	Pos     float64 `json:"pos"`
	Warm    float64 `json:"warm"`
	EggBPM  float64 `json:"eggBpm"`
	Caption string  `json:"caption"`
	Start   int     `json:"start"`
	Frames  int     `json:"frames"`
}

type edit struct {
	DropBPM     float64   `json:"dropBpm"`
	PrerollBars int       `json:"prerollBars"`
	Tail        float64   `json:"tail"`
	ScanRate    float64   `json:"scanRate"`
	Query       string    `json:"query"`
	Say         string    `json:"say"`
	URL         string    `json:"url"`
	EndCaption  string    `json:"endCaption"`
	Segments    []segment `json:"segments"`
}

type sfx struct {
	T    float64 `json:"t"`
	Kind string  `json:"kind"`
}

type timeline struct {
	Drop     int       `json:"drop"`
	End      int       `json:"end"`
	Last     int       `json:"last"`
	Beat     float64   `json:"beat"`
	Segments []segment `json:"segments"`
	Sfx      []sfx     `json:"sfx"`
}

func frameOf(t float64) int { return int(math.Round(t * fps)) }

func (e *edit) layout() (drop, last, end int) {
	beat := 60 / e.DropBPM
	drop = frameOf(float64(e.PrerollBars*4) * beat)
	f := drop
	for i := range e.Segments {
		s := &e.Segments[i]
		s.Start = f
		s.Frames = frameOf(float64(s.Beats) * 60 / s.BPM)
		f += s.Frames
	}
	return drop, f, f + frameOf(e.Tail)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func loadJSON(path string, v any) {
	data, err := os.ReadFile(path) //nolint:gosec // G304: paths come from the flags of a local tool
	must(err)
	must(json.Unmarshal(data, v))
}

func stock(m *stage.Model, es []entry) {
	for _, e := range es {
		f, err := os.Open(e.Cover)
		must(err)
		img, _, err := image.Decode(f)
		must(err)
		must(f.Close())
		m.SetCover(e.Track.Key(), img)
	}
}

func tracksOf(es []entry) []stage.Track {
	ts := make([]stage.Track, len(es))
	for i, e := range es {
		ts[i] = e.Track
	}
	return ts
}

// film is the cursor over the timeline: one call of frame per frame in order.
type film struct {
	e      edit
	es     []entry
	tracks []stage.Track
	drop   int
	last   int
	end    int
	beat   float64
	log    []sfx
	pre    *preroll
	seg    int
	m      *stage.Model
	endM   *stage.Model
	cells  [rows * cols]cell
	glyphs atlas
}

func (f *film) sound(frame int, kind string) {
	f.log = append(f.log, sfx{float64(frame) / fps, kind})
}

func (f *film) beatFrame(n float64) int { return frameOf(n * f.beat) }

func (f *film) openSegment(i int) {
	s := f.e.Segments[i]
	m := stage.New(stage.Options{SelfPlay: true, Slots: 5})
	m.SetSize(cols, rows)
	stock(m, f.es)
	m.JumpPlay(f.tracks, i, s.Pos-s.Warm)
	for t := 0.0; t < s.Warm-1e-9; t += 1.0 / fps {
		m.Step(1.0 / fps)
		m.View()
	}
	f.m = m
}

func (f *film) frame(n int) {
	switch {
	case n < f.drop:
		f.pre.frame(n, &f.cells)
	case n < f.last:
		for f.seg+1 < len(f.e.Segments) && n >= f.e.Segments[f.seg+1].Start {
			f.seg++
			f.openSegment(f.seg)
		}
		s := f.e.Segments[f.seg]
		if f.seg == 0 && f.m == nil {
			f.m = f.pre.m
			f.pre.land(s)
		}
		if f.seg == 0 {
			f.sayScene(n-s.Start, s)
		}
		rate := 1.0
		if s.EggBPM > 0 {
			rate = s.BPM / s.EggBPM
		}
		f.m.Step(rate / fps)
		parseANSI(f.m.View(), &f.cells)
	default:
		f.endFrame(n)
	}
}

// sayScene types the owner's word into the player so that enter lands in the
// silence before the beat comes back.
func (f *film) sayScene(k int, s segment) {
	beat := 60 / s.BPM
	enter := frameOf(float64(s.Beats)*beat - 0.62)
	open := enter - frameOf(2*beat)
	word := []rune(f.e.Say)
	switch {
	case k == open:
		f.m.Key("/", "/")
	case k == enter:
		f.m.Key("enter", "")
		f.m.NextGathering(f.e.Say)
	case k > open && k < enter:
		step := (enter - open - 12) / len(word)
		if i := (k - open) / step; (k-open)%step == 0 && i >= 1 && i <= len(word) {
			f.m.Key("", string(word[i-1]))
		}
	}
}

func (f *film) endFrame(n int) {
	k := n - f.last
	if f.endM == nil {
		f.endM = stage.New(stage.Options{SelfPlay: true, Slots: 5})
		f.endM.SetSize(cols, rows)
	}
	m := f.endM
	typeAt := frameOf(1.6)
	url := []rune(f.e.URL)
	if k >= typeAt {
		i := (k - typeAt) / 2
		if (k-typeAt)%2 == 0 && i < len(url) {
			m.Key("", string(url[i]))
		}
	}
	m.Step(1.0 / fps)
	parseANSI(m.View(), &f.cells)
	if fade := f.end - n; fade < 30 {
		dim(&f.cells, float64(fade)/30)
	}
}

func dim(c *[rows * cols]cell, k float64) {
	for i := range c {
		for j := range 3 {
			c[i].fg[j] = uint8(float64(c[i].fg[j]) * k)
			c[i].bg[j] = uint8(float64(c[i].bg[j]) * k)
		}
	}
}

func newFilm(e edit, es []entry) *film {
	f := &film{e: e, es: es, tracks: tracksOf(es), beat: 60 / e.DropBPM}
	f.drop, f.last, f.end = f.e.layout()
	f.pre = newPreroll(f)
	return f
}

func savePNG(path string, rgb []byte) {
	im := image.NewNRGBA(image.Rect(0, 0, frameW, frameH))
	for i := range frameW * frameH {
		im.Pix[i*4], im.Pix[i*4+1], im.Pix[i*4+2], im.Pix[i*4+3] = rgb[i*3], rgb[i*3+1], rgb[i*3+2], 255
	}
	w, err := os.Create(path) //nolint:gosec // G304: paths come from the flags of a local tool
	must(err)
	must(png.Encode(w, im))
	must(w.Close())
}

func encoder(out, audio string, crf int, preset string) (*exec.Cmd, io.WriteCloser) {
	args := []string{"-v", "error", "-y", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", fmt.Sprintf("%dx%d", frameW, frameH),
		"-r", strconv.Itoa(fps), "-i", "pipe:0"}
	if audio != "" {
		args = append(args, "-i", audio, "-c:a", "aac", "-b:a", "256k", "-ar", "48000", "-shortest")
	}
	args = append(args, "-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuv420p", "-c:v", "libx264",
		"-preset", preset, "-crf", strconv.Itoa(crf), "-colorspace", "bt709", "-color_primaries", "bt709",
		"-color_trc", "bt709", "-movflags", "+faststart", out)
	cmd := exec.Command("ffmpeg", args...) //nolint:gosec // G204: the arguments come from the flags of a local tool
	cmd.Stderr = os.Stderr
	w, err := cmd.StdinPipe()
	must(err)
	must(cmd.Start())
	return cmd, w
}

func main() {
	editPath := flag.String("edit", "lab/promo/edit.json", "")
	tracksPath := flag.String("tracks", "lab/cache/promo/tracks.json", "")
	atlasPath := flag.String("atlas", "lab/cache/promo/atlas.bin", "")
	timelinePath := flag.String("timeline", "lab/cache/promo/timeline.json", "")
	dry := flag.Bool("dry", false, "")
	sheet := flag.String("sheet", "", "")
	sheetOut := flag.String("sheet-out", "lab/cache/promo/shot", "")
	out := flag.String("out", "", "")
	audio := flag.String("audio", "", "")
	crf := flag.Int("crf", 17, "")
	preset := flag.String("preset", "slow", "")
	flag.Parse()
	stage.SetLang("ru")

	var e edit
	loadJSON(*editPath, &e)
	var es []entry
	loadJSON(*tracksPath, &es)
	byKey := map[string]entry{}
	for _, x := range es {
		byKey[x.Key] = x
	}
	ordered := make([]entry, 0, len(e.Segments))
	for _, s := range e.Segments {
		ordered = append(ordered, byKey[s.Key])
	}
	f := newFilm(e, ordered)

	a, err := loadAtlas(*atlasPath)
	must(err)
	f.glyphs = a
	src := make([]byte, srcW*srcH*3)
	rgb := make([]byte, frameW*frameH*3)

	want := map[int]bool{}
	stop := f.end
	if *sheet != "" {
		stop = 0
		for v := range strings.SplitSeq(*sheet, ",") {
			t, _ := strconv.ParseFloat(v, 64)
			want[frameOf(t)] = true
			stop = max(stop, frameOf(t)+1)
		}
	}
	var cmd *exec.Cmd
	var w io.WriteCloser
	if *out != "" {
		cmd, w = encoder(*out, *audio, *crf, *preset)
	}
	for n := 0; n < stop; n++ {
		f.frame(n)
		switch {
		case want[n]:
			raster(&f.cells, a, src)
			f.shoot(n, src, rgb)
			savePNG(fmt.Sprintf("%s-%07.3f.png", *sheetOut, float64(n)/fps), rgb)
		case w != nil:
			raster(&f.cells, a, src)
			f.shoot(n, src, rgb)
			_, err := w.Write(rgb)
			must(err)
		}
	}
	if w != nil {
		must(w.Close())
		must(cmd.Wait())
	}
	if *dry {
		data, err := json.MarshalIndent(timeline{f.drop, f.end, f.last, f.beat, f.e.Segments, f.log}, "", " ")
		must(err)
		must(os.WriteFile(*timelinePath, data, 0o644)) //nolint:gosec // G306: the timeline holds no secrets
		fmt.Printf("drop %.3f last %.3f end %.3f\n", float64(f.drop)/fps, float64(f.last)/fps, float64(f.end)/fps)
		for _, s := range f.e.Segments {
			fmt.Printf("%-16s %7.3f +%.3f\n", s.Key, float64(s.Start)/fps, float64(s.Frames)/fps)
		}
	}
}
