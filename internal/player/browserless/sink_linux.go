//go:build linux

package browserless

import "github.com/oglenyaboss/aux-player/internal/player/gst"

func newAudioSink() (audioSink, error) {
	if err := gst.EnsureAACDecoder(); err != nil {
		return nil, err
	}
	return gst.New()
}
