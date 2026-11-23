//go:build wasip1 && wasm

package main

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"github.com/mrpoundsign/PoundControl-Twitch/internal/twitch"
)

//go:wasmimport poundcontrol pc_update_visual
func pcUpdateVisual(ctxPtr, ctxLen, patchPtr, patchLen uint32) int32

//go:wasmimport poundcontrol pc_save_settings
func pcSaveSettings(ctxPtr, ctxLen, jsonPtr, jsonLen uint32) int32

//go:wasmimport poundcontrol pc_get_global_settings
func pcGetGlobalSettings(outPtr, outCap uint32) int32

//go:wasmimport poundcontrol pc_save_global_settings
func pcSaveGlobalSettings(jsonPtr, jsonLen uint32) int32

//go:wasmimport poundcontrol pc_log
func pcLog(msgPtr, msgLen uint32)

//go:wasmimport poundcontrol pc_http_request
func pcHTTPRequest(reqPtr, reqLen, respPtr, respCap uint32) int32

//go:wasmimport poundcontrol pc_set_timer
func pcSetTimer(idPtr, idLen, ctxPtr, ctxLen, intervalMs uint32) int32

const (
	initialGlobalBufSize = 16 * 1024
	httpRespBufSize      = 320 * 1024 // > 256 KiB host body cap + JSON envelope & headers
)

var (
	allocMu     sync.Mutex
	allocations = make(map[uint32][]byte)

	pluginOnce sync.Once
	pluginInst *twitch.Plugin
)

func getPlugin() *twitch.Plugin {
	pluginOnce.Do(func() {
		pluginInst = twitch.NewPlugin(&wasmHost{
			httpBuf: make([]byte, httpRespBufSize),
		})
	})
	return pluginInst
}

//go:wasmexport pc_alloc
func pcAlloc(size uint32) uint32 {
	if size == 0 {
		size = 1
	}
	buf := make([]byte, size)
	ptr := uint32(uintptr(unsafe.Pointer(&buf[0])))
	allocMu.Lock()
	allocations[ptr] = buf
	allocMu.Unlock()
	return ptr
}

//go:wasmexport pc_free
func pcFree(ptr uint32) {
	allocMu.Lock()
	delete(allocations, ptr)
	allocMu.Unlock()
}

func returnHostBytes(raw []byte) uint64 {
	ptr := pcAlloc(uint32(len(raw)))
	allocMu.Lock()
	buf := allocations[ptr]
	copy(buf, raw)
	allocMu.Unlock()
	return (uint64(ptr) << 32) | uint64(len(raw))
}

func bytesPtrLen(b []byte) (uint32, uint32) {
	if len(b) == 0 {
		return 0, 0
	}
	return uint32(uintptr(unsafe.Pointer(&b[0]))), uint32(len(b))
}

func readHostSlice(ptr, size uint32) ([]byte, bool) {
	allocMu.Lock()
	buf, ok := allocations[ptr]
	if !ok || uint32(len(buf)) < size {
		allocMu.Unlock()
		return nil, false
	}
	cp := make([]byte, size)
	copy(cp, buf[:size])
	allocMu.Unlock()
	return cp, true
}

//go:wasmexport pc_manifest
func pcManifest() uint64 {
	raw, _ := json.Marshal(twitch.BuildManifest())
	return returnHostBytes(raw)
}

//go:wasmexport pc_handle_event
func pcHandleEvent(ptr, size uint32) int32 {
	raw, ok := readHostSlice(ptr, size)
	if !ok {
		return -1
	}
	var ev twitch.ActionEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return -2
	}
	if err := getPlugin().HandleEvent(ev); err != nil {
		return -3
	}
	return 0
}

//go:wasmexport pc_on_timer
func pcOnTimer(ptr, size uint32) int32 {
	raw, ok := readHostSlice(ptr, size)
	if !ok {
		return -1
	}
	var tev twitch.TimerEvent
	if err := json.Unmarshal(raw, &tev); err != nil {
		return -2
	}
	if err := getPlugin().OnTimer(tev); err != nil {
		return -3
	}
	return 0
}

//go:wasmexport pc_on_global_settings
func pcOnGlobalSettings(ptr, size uint32) int32 {
	raw, ok := readHostSlice(ptr, size)
	if !ok {
		return -1
	}
	var gs map[string]any
	if err := json.Unmarshal(raw, &gs); err != nil {
		return -2
	}
	if err := getPlugin().HandleGlobalSettings(gs); err != nil {
		return -3
	}
	return 0
}

//go:wasmexport pc_on_config_action
func pcOnConfigAction(ptr, size uint32) int32 {
	raw, ok := readHostSlice(ptr, size)
	if !ok {
		return -1
	}
	var ev twitch.ConfigActionEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return -2
	}
	if err := getPlugin().HandleConfigAction(ev); err != nil {
		return -3
	}
	return 0
}

type wasmHost struct {
	httpMu  sync.Mutex
	httpBuf []byte
}

func (h *wasmHost) UpdateVisual(ctxID string, patch twitch.VisualPatch) {
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return
	}
	ctxBytes := []byte(ctxID)
	cp, cl := bytesPtrLen(ctxBytes)
	pp, pl := bytesPtrLen(patchBytes)
	pcUpdateVisual(cp, cl, pp, pl)
	runtime.KeepAlive(ctxBytes)
	runtime.KeepAlive(patchBytes)
}

func (h *wasmHost) SaveSettings(ctxID string, settings map[string]any) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	ctxBytes := []byte(ctxID)
	cp, cl := bytesPtrLen(ctxBytes)
	jp, jl := bytesPtrLen(raw)
	rc := pcSaveSettings(cp, cl, jp, jl)
	runtime.KeepAlive(ctxBytes)
	runtime.KeepAlive(raw)
	if rc != 0 {
		return fmt.Errorf("pc_save_settings failed: %d", rc)
	}
	return nil
}

func (h *wasmHost) GetGlobalSettings() map[string]any {
	buf := make([]byte, initialGlobalBufSize)
	for attempt := 0; attempt < 2; attempt++ {
		bp, bl := bytesPtrLen(buf)
		n := pcGetGlobalSettings(bp, bl)
		runtime.KeepAlive(buf)
		if n >= 0 {
			var out map[string]any
			if err := json.Unmarshal(buf[:n], &out); err != nil {
				return map[string]any{}
			}
			return out
		}
		if n < -1 {
			buf = make([]byte, int(-n))
			continue
		}
		break
	}
	return map[string]any{}
}

func (h *wasmHost) SaveGlobalSettings(settings map[string]any) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	jp, jl := bytesPtrLen(raw)
	rc := pcSaveGlobalSettings(jp, jl)
	runtime.KeepAlive(raw)
	if rc != 0 {
		return fmt.Errorf("pc_save_global_settings failed: %d", rc)
	}
	return nil
}

func (h *wasmHost) Log(msg string) {
	b := []byte(msg)
	mp, ml := bytesPtrLen(b)
	pcLog(mp, ml)
	runtime.KeepAlive(b)
}

func (h *wasmHost) HTTPRequest(req twitch.HTTPRequest) twitch.HTTPResponse {
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return twitch.HTTPResponse{Error: err.Error()}
	}

	h.httpMu.Lock()
	defer h.httpMu.Unlock()

	rp, rl := bytesPtrLen(reqBytes)
	bp, bl := bytesPtrLen(h.httpBuf)
	n := pcHTTPRequest(rp, rl, bp, bl)
	runtime.KeepAlive(reqBytes)
	runtime.KeepAlive(h.httpBuf)

	if n < 0 {
		return twitch.HTTPResponse{
			Error: fmt.Sprintf("host HTTP response error or buffer overflow (rc=%d)", n),
		}
	}
	var resp twitch.HTTPResponse
	if err := json.Unmarshal(h.httpBuf[:n], &resp); err != nil {
		return twitch.HTTPResponse{Error: "decode host HTTP response: " + err.Error()}
	}
	return resp
}

func (h *wasmHost) SetTimer(timerID, ctxID string, intervalMs uint32) error {
	idBytes := []byte(timerID)
	ctxBytes := []byte(ctxID)
	ip, il := bytesPtrLen(idBytes)
	cp, cl := bytesPtrLen(ctxBytes)
	rc := pcSetTimer(ip, il, cp, cl, intervalMs)
	runtime.KeepAlive(idBytes)
	runtime.KeepAlive(ctxBytes)
	if rc != 0 {
		return fmt.Errorf("pc_set_timer failed: %d", rc)
	}
	return nil
}

func (h *wasmHost) Now() time.Time {
	return time.Now()
}

func main() {}
