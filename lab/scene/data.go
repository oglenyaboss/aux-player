package main

import "github.com/simone-vibes/vibez/internal/stage"

// Demo data of the scene. Language-sensitive texts are built on every call, so the language
// toggled by the onboarding lands with the next line.

// demoCandidate is one demo stream row: rejected ones appear with a reason and are dropped.
type demoCandidate struct {
	artist, title, why string
	dur                float64
	ok                 bool
	reason             string
}

func demoQuery() string {
	return stage.Tr("как Don Toliver, но не он, ночью в машине", "like Don Toliver but not him, late night drive")
}

func demoChips(query string) []stage.Chip {
	if query != demoQuery() {
		return nil
	}
	return []stage.Chip{
		{Token: "Don Toliver", Label: stage.Tr("Don Toliver ✕ не он", "Don Toliver ✕ not him")},
		{Token: stage.Tr("ночью", "night"), Label: stage.Tr("ночь", "late night")},
		{Token: stage.Tr("машине", "drive"), Label: stage.Tr("в машине", "driving")},
	}
}

func questions() []stage.Question {
	return []stage.Question{
		{Title: stage.Tr("Какая грань Don Toliver?", "Which side of Don Toliver?"),
			Options: []string{stage.Tr("мелодичный рэйдж", "melodic rage"), stage.Tr("тёмный и вязкий", "dark and dense"),
				stage.Tr("хуки на гитаре", "guitar hooks"), stage.Tr("всё вместе", "all of it")},
			Chip: stage.Tr("грань", "vibe")},
		{Title: stage.Tr("Сколько энергии?", "How much energy?"), Scale: true,
			Left: stage.Tr("плыть", "drift"), Right: stage.Tr("в пол", "floor it"), Def: 2, Chip: stage.Tr("энергия", "energy")},
	}
}

func laterQuestion() stage.Question {
	return stage.Question{
		Title:   stage.Tr("Дальше больше Атланты или ночного R&B?", "More Atlanta or late-night R&B next?"),
		Options: []string{stage.Tr("Атланта", "Atlanta"), stage.Tr("ночной R&B", "late-night R&B"), stage.Tr("поровну", "half and half")},
	}
}

func portionNote() string {
	return stage.Tr("без крика, держу ночной мелодичный рэйдж", "no shouting, keeping the melodic night rage")
}

func batches() []int { return []int{10, 10, 9} }

func demoCandidates() []demoCandidate {
	return []demoCandidate{
		{"Travis Scott", "FE!N", stage.Tr("гипнотичный луп, вокал в эфире", "hypnotic loop, vocals in the air"), 191, true, ""},
		{"Don Toliver", "After Party", "", 0, false, stage.Tr("ориентир «не он»", `"not him" pick`)},
		{"Lil Uzi Vert", "Just Wanna Rock", stage.Tr("джерси-бит, лёгкий как неон", "jersey beat, light as neon"), 123, true, ""},
		{"Travis Scott", "goosebumps", "", 0, false, stage.Tr("известный трек · библиотека", "known track · library")},
		{"Playboi Carti", "Magnolia", stage.Tr("сухой бит, ночной расслабон", "dry beat, night wind-down"), 181, true, ""},
		{"Ken Carson", "Freestyle 2", stage.Tr("мелодия поверх жирных синтов", "melody over fat synths"), 132, true, ""},
		{"Yeat", "Talk", "", 0, false, stage.Tr("уже выдавали 21.10", "served on 21.10")},
		{"Trippie Redd", "Miss The Rage", stage.Tr("рэйдж-синты, мелодия на хуке", "rage synths, melody on the hook"), 207, true, ""},
		{"Lil Yachty", "the BLACK seminole.", stage.Tr("психоделика, гитары, долгий разгон", "psychedelia, guitars, slow build"), 412, true, ""},
		{"Tame Impala", "The Less I Know the Better", stage.Tr("тягучий бас, тёплый фальцет", "dragging bass, warm falsetto"), 216, true, ""},

		{"Kid Cudi", "Day 'N' Nite", stage.Tr("гул и напев, ночь в окне", "hum and croon, night in the window"), 221, true, ""},
		{"Gunna", "Midnight Drive", "", 0, false, stage.Tr("нет в Apple Music", "not on Apple Music")},
		{"Brent Faiyaz", "WASTING TIME", stage.Tr("вязкий R&B, мягкие клавиши", "syrupy R&B, soft keys"), 231, true, ""},
		{"The Weeknd", "Often", stage.Tr("тёмный R&B, медленный пульс", "dark R&B, slow pulse"), 249, true, ""},
		{"Future", "Mask Off", "", 0, false, stage.Tr("известные артисты: 4 из 4", "famous artists: 4 of 4")},
		{"Swae Lee", "Won't Be Late", stage.Tr("воздушный вокал, лёгкий грув", "airy vocals, light groove"), 188, true, ""},
		{"Rae Sremmurd", "Black Beatles", stage.Tr("пружинистый бас, ленивая мелодия", "springy bass, lazy melody"), 291, true, ""},
		{"Travis Scott", "SICKO MODE", "", 0, false, stage.Tr("уже 2 трека артиста", "2 artist tracks already")},
		{"6LACK", "PRBLMS", stage.Tr("полушёпот, пустой ночной бит", "half whisper, empty night beat"), 247, true, ""},
		{"PARTYNEXTDOOR", "Break From Toronto", stage.Tr("приглушённый бит, вокал в реверберации", "muffled beat, vocals in reverb"), 175, true, ""},

		{"Bryson Tiller", "Exchange", stage.Tr("трэп-соул, медленно и близко", "trap-soul, slow and close"), 194, true, ""},
		{"Kanye West", "Ghost Town", "", 0, false, stage.Tr("известный трек · библиотека", "known track · library")},
		{"Metro Boomin", "Superhero (Heroes & Villains)", stage.Tr("кино-бит, напор без крика", "cinematic beat, pressure without shouting"), 182, true, ""},
		{"Playboi Carti", "Sky", stage.Tr("вампирская эпоха, лёгкий хук", "vampire era, weightless hook"), 193, true, ""},
		{"Don Toliver", "No Idea", "", 0, false, stage.Tr("ориентир «не он»", `"not him" pick`)},
		{"Destroy Lonely", "if looks could kill", stage.Tr("холодный бас, ровный флоу", "cold bass, even flow"), 190, true, ""},
		{"Lil Uzi Vert", "XO Tour Llif3", "", 0, false, stage.Tr("уже выдавали 22.10", "served on 22.10")},
		{"Lil Tecca", "Dark Thoughts", stage.Tr("быстрые хэты, светлая мелодия", "fast hats, bright melody"), 152, true, ""},
		{"Frank Ocean", "Nights", stage.Tr("бит ломается посередине, ночная трасса", "beat flips midway, night highway"), 307, true, ""},
	}
}

// demoTracks is the accepted part of the demo stream, in stream order.
func demoTracks() []stage.Track {
	var out []stage.Track
	for _, c := range demoCandidates() {
		if c.ok {
			out = append(out, stage.Track{Artist: c.artist, Title: c.title, Why: c.why, Dur: c.dur})
		}
	}
	return out
}

// demoModels is the canned model list the fake agent answers LoadModels with.
var demoModels = []stage.ModelInfo{
	{ID: "deepseek/deepseek-v4.1-flash", Reasoning: true, Rec: true},
	{ID: "anthropic/claude-haiku-5.5", Reasoning: true, Rec: true},
	{ID: "qwen/qwen3.8-flash", Reasoning: true},
	{ID: "z-ai/glm-5.3-flash", Reasoning: true},
	{ID: "anthropic/claude-sonnet-5.5", Reasoning: true},
	{ID: "openai/gpt-oss-120b", Reasoning: true},
	{ID: "google/gemini-3-flash", Reasoning: true},
	{ID: "moonshotai/kimi-k3", Reasoning: true},
	{ID: "meta-llama/llama-4-maverick"},
	{ID: "mistralai/mistral-medium-3"},
	{ID: "x-ai/grok-4-fast", Reasoning: true},
}

// demoModelTotal is how many models the provider lists in all.
const demoModelTotal = 312

// demoKey is the key the autopilot types into the agent step.
const demoKey = "sk-or-v1-4f9a2c71e0b8d3"
