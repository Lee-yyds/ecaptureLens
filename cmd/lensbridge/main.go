// The Flutter bridge starts the same gateway used by `ecapturelens serve`.
package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
	"unsafe"

	"ecapturelens/gateway"
)

var bridge struct {
	sync.Mutex
	service *gateway.Service
	server  *http.Server
	done    chan error
	url     string
	token   string
}

func result(v any) *C.char {
	b, err := json.Marshal(v)
	if err != nil {
		return C.CString(`{"error":"cannot encode bridge result"}`)
	}
	return C.CString(string(b))
}
func failure(err error) *C.char { return result(map[string]string{"error": err.Error()}) }

//export LensStart
func LensStart(dir *C.char) *C.char {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.service != nil {
		return result(map[string]string{"url": bridge.url, "token": bridge.token})
	}
	svc, err := gateway.Open(C.GoString(dir))
	if err != nil {
		return failure(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return failure(errors.Join(err, svc.Close()))
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return failure(errors.Join(err, listener.Close(), svc.Close()))
	}
	bridge.service = svc
	bridge.token = hex.EncodeToString(key)
	bridge.url = "http://" + listener.Addr().String()
	bridge.server = &http.Server{Handler: svc.Handler(bridge.token), ReadHeaderTimeout: 5 * time.Second}
	bridge.done = make(chan error, 1)
	server, done := bridge.server, bridge.done
	go func() {
		err := server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}()
	return result(map[string]string{"url": bridge.url, "token": bridge.token})
}

//export LensStop
func LensStop() *C.char {
	bridge.Lock()
	defer bridge.Unlock()
	if bridge.service == nil {
		return result(map[string]bool{"stopped": true})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := bridge.server.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, bridge.server.Close())
	}
	err = errors.Join(err, <-bridge.done, bridge.service.Close())
	bridge.service = nil
	bridge.server = nil
	bridge.done = nil
	bridge.url = ""
	bridge.token = ""
	if err != nil {
		return failure(err)
	}
	return result(map[string]bool{"stopped": true})
}

//export LensFree
func LensFree(value *C.char) { C.free(unsafe.Pointer(value)) }

func main() {}
