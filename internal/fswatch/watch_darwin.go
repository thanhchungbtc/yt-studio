//go:build darwin

package fswatch

/*
#cgo LDFLAGS: -framework CoreServices
#include <stdint.h>
#include <stdlib.h>
void *fswatchStart(const char *root, uintptr_t handle, double latency);
void *fswatchStartPaths(const char **roots, int count, uintptr_t handle, double latency, int fileEvents);
void fswatchStop(void *stream);
*/
import "C"

import (
	"errors"
	"runtime/cgo"
	"time"
	"unsafe"
)

type stream struct {
	ref    unsafe.Pointer
	handle cgo.Handle
}

func start(root string, latency time.Duration, report func(paths []string, rescan bool)) (*stream, error) {
	h := cgo.NewHandle(report)
	croot := C.CString(root)
	defer C.free(unsafe.Pointer(croot))
	ref := C.fswatchStart(croot, C.uintptr_t(h), C.double(latency.Seconds()))
	if ref == nil {
		h.Delete()
		return nil, errors.New("could not watch " + root)
	}
	return &stream{ref: ref, handle: h}, nil
}

func startDirs(roots []string, latency time.Duration, report func(paths []string, rescan bool)) (*stream, error) {
	if len(roots) == 0 {
		return nil, errors.New("nothing to watch")
	}
	h := cgo.NewHandle(report)
	list := (**C.char)(C.malloc(C.size_t(len(roots)) * C.size_t(unsafe.Sizeof((*C.char)(nil)))))
	defer C.free(unsafe.Pointer(list))
	croots := unsafe.Slice(list, len(roots))
	for i, r := range roots {
		croots[i] = C.CString(r)
	}
	defer func() {
		for _, c := range croots {
			C.free(unsafe.Pointer(c))
		}
	}()
	ref := C.fswatchStartPaths(list, C.int(len(roots)), C.uintptr_t(h), C.double(latency.Seconds()), 0)
	if ref == nil {
		h.Delete()
		return nil, errors.New("could not watch folders")
	}
	return &stream{ref: ref, handle: h}, nil
}

func (s *stream) stop() {
	C.fswatchStop(s.ref)
	s.handle.Delete()
}

//export goFSEvents
func goFSEvents(handle C.uintptr_t, paths **C.char, count C.int, rescan C.int) {
	report, ok := cgo.Handle(handle).Value().(func([]string, bool))
	if !ok {
		return
	}
	n := int(count)
	list := unsafe.Slice(paths, n)
	out := make([]string, n)
	for i, p := range list {
		out[i] = C.GoString(p)
	}
	report(out, rescan != 0)
}
