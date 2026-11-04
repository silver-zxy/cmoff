//go:build android

package main

/*
#cgo LDFLAGS: -llog
#include <stdlib.h>
#include <android/log.h>
static void cm_log(const char *s) { __android_log_write(ANDROID_LOG_INFO, "cmserver", s); }
*/
import "C"

import (
	"log"
	"unsafe"
)

type logcat struct{}

func (logcat) Write(p []byte) (int, error) {
	s := C.CString(string(p))
	C.cm_log(s)
	C.free(unsafe.Pointer(s))
	return len(p), nil
}

// setupLog sends the server log to logcat (adb logcat -s cmserver).
func setupLog() {
	log.SetFlags(0)
	log.SetOutput(logcat{})
}
