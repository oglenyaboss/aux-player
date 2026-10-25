//go:build darwin && cgo

package browserless

/*
#cgo darwin CXXFLAGS: -std=c++17
#cgo darwin LDFLAGS: -framework AudioToolbox -framework CoreFoundation -lc++
#include "sink_darwin.h"
#include <stdlib.h>

extern void auxDarwinOnEOS(uintptr_t userData);
extern void auxDarwinOnError(uintptr_t userData, char* errMsg);
extern void auxDarwinOnStreamStart(uintptr_t userData, char* uri);

static inline aux_darwinsink_t aux_create_darwin_sink(uintptr_t userData) {
    return aux_darwinsink_create(userData, auxDarwinOnEOS, auxDarwinOnError, auxDarwinOnStreamStart);
}
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

type darwinAudioSink struct {
	mu            sync.Mutex
	handle        C.aux_darwinsink_t
	cgoH          cgo.Handle
	onEOS         func()
	onError       func(error)
	onStreamStart func(string)
}

func newAudioSink() (audioSink, error) {
	s := &darwinAudioSink{}
	h := cgo.NewHandle(s)
	s.cgoH = h

	playerHandle := C.aux_create_darwin_sink(C.uintptr_t(h))
	if playerHandle == nil {
		h.Delete()
		return nil, errors.New("failed to create Darwin audio sink")
	}
	s.handle = playerHandle
	return s, nil
}

func sinkFromHandle(userData C.uintptr_t) (s *darwinAudioSink) {
	defer func() { _ = recover() }()
	s, _ = cgo.Handle(userData).Value().(*darwinAudioSink)
	return s
}

//export auxDarwinOnEOS
func auxDarwinOnEOS(userData C.uintptr_t) {
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onEOS
	s.mu.Unlock()
	if cb != nil {
		go cb()
	}
}

//export auxDarwinOnError
func auxDarwinOnError(userData C.uintptr_t, cErrMsg *C.char) {
	msg := C.GoString(cErrMsg)
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onError
	s.mu.Unlock()
	if cb != nil {
		go cb(errors.New(msg))
	}
}

//export auxDarwinOnStreamStart
func auxDarwinOnStreamStart(userData C.uintptr_t, cURI *C.char) {
	uri := C.GoString(cURI)
	s := sinkFromHandle(userData)
	if s == nil {
		return
	}
	s.mu.Lock()
	cb := s.onStreamStart
	s.mu.Unlock()
	if cb != nil {
		go cb(uri)
	}
}

func (s *darwinAudioSink) PlayURI(uri string) {
	cs := C.CString(uri)
	defer C.free(unsafe.Pointer(cs))
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_play_uri(s.handle, cs)
	}
}

func (s *darwinAudioSink) SetNextURI(uri string) {
	var cs *C.char
	if uri != "" {
		cs = C.CString(uri)
		defer C.free(unsafe.Pointer(cs))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_set_next_uri(s.handle, cs)
	}
}

func (s *darwinAudioSink) OnStreamStart(fn func(string)) {
	s.mu.Lock()
	s.onStreamStart = fn
	s.mu.Unlock()
}
func (s *darwinAudioSink) Play() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_play(s.handle)
	}
}

func (s *darwinAudioSink) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_pause(s.handle)
	}
}

func (s *darwinAudioSink) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_stop(s.handle)
	}
}

func (s *darwinAudioSink) Seek(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_seek(s.handle, C.double(d.Seconds()))
	}
}

func (s *darwinAudioSink) SetVolume(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		C.aux_darwinsink_set_volume(s.handle, C.float(v))
	}
}

func (s *darwinAudioSink) SetEqualizer(gains []float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle == nil {
		return
	}
	if len(gains) == 0 {
		C.aux_darwinsink_set_eq(s.handle, nil, 0)
		return
	}
	cg := make([]C.double, len(gains))
	for i, g := range gains {
		cg[i] = C.double(g)
	}
	C.aux_darwinsink_set_eq(s.handle, &cg[0], C.int(len(cg)))
}

func (s *darwinAudioSink) Position() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.handle != nil {
		sec := C.aux_darwinsink_position(s.handle)
		return time.Duration(float64(sec) * float64(time.Second))
	}
	return 0
}

func (s *darwinAudioSink) OnEOS(fn func()) {
	s.mu.Lock()
	s.onEOS = fn
	s.mu.Unlock()
}

func (s *darwinAudioSink) OnError(fn func(error)) {
	s.mu.Lock()
	s.onError = fn
	s.mu.Unlock()
}

func (s *darwinAudioSink) Destroy() {
	s.mu.Lock()
	handle := s.handle
	s.handle = nil
	s.mu.Unlock()
	if handle != nil {
		C.aux_darwinsink_destroy(handle)
		s.cgoH.Delete()
	}
}
