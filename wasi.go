package wasmtime

// #include <wasi.h>
// #include <stdint.h>
// #include <stdlib.h>
import "C"
import (
	"errors"
	"os"
	"runtime"
	"unsafe"
)

type WasiConfig struct {
	_ptr *C.wasi_config_t
}

func NewWasiConfig() *WasiConfig {
	ptr := C.wasi_config_new()
	config := &WasiConfig{_ptr: ptr}
	runtime.SetFinalizer(config, func(config *WasiConfig) {
		C.wasi_config_delete(config._ptr)
	})
	return config
}

func (c *WasiConfig) ptr() *C.wasi_config_t {
	ret := c._ptr
	maybeGC()
	return ret
}

// SetArgv will explicitly configure the argv for this WASI configuration.
// Note that this field can only be set, it cannot be read
func (c *WasiConfig) SetArgv(argv []string) {
	ptrs := make([]*C.char, len(argv))
	for i, arg := range argv {
		ptrs[i] = C.CString(arg)
	}
	var argvRaw **C.char
	if len(ptrs) > 0 {
		argvRaw = &ptrs[0]
	}
	C.wasi_config_set_argv(c.ptr(), C.int(len(argv)), argvRaw)
	runtime.KeepAlive(c)
	for _, ptr := range ptrs {
		C.free(unsafe.Pointer(ptr))
	}
}

func (c *WasiConfig) InheritArgv() {
	C.wasi_config_inherit_argv(c.ptr())
	runtime.KeepAlive(c)
}

// SetEnv configures environment variables to be returned for this WASI configuration.
// The pairs provided must be an iterable list of key/value pairs of environment variables.
// Note that this field can only be set, it cannot be read
func (c *WasiConfig) SetEnv(keys, values []string) {
	if len(keys) != len(values) {
		panic("mismatched numbers of keys and values")
	}
	namePtrs := make([]*C.char, len(values))
	valuePtrs := make([]*C.char, len(values))
	for i, key := range keys {
		namePtrs[i] = C.CString(key)
	}
	for i, value := range values {
		valuePtrs[i] = C.CString(value)
	}
	var namesRaw, valuesRaw **C.char
	if len(keys) > 0 {
		namesRaw = &namePtrs[0]
		valuesRaw = &valuePtrs[0]
	}
	C.wasi_config_set_env(c.ptr(), C.int(len(keys)), namesRaw, valuesRaw)
	runtime.KeepAlive(c)
	for i, ptr := range namePtrs {
		C.free(unsafe.Pointer(ptr))
		C.free(unsafe.Pointer(valuePtrs[i]))
	}
}

func (c *WasiConfig) InheritEnv() {
	C.wasi_config_inherit_env(c.ptr())
	runtime.KeepAlive(c)
}

func (c *WasiConfig) SetStdinFile(path string) error {
	pathC := C.CString(path)
	ok := C.wasi_config_set_stdin_file(c.ptr(), pathC)
	runtime.KeepAlive(c)
	C.free(unsafe.Pointer(pathC))
	if ok {
		return nil
	}

	return errors.New("failed to open file")
}

func (c *WasiConfig) InheritStdin() {
	C.wasi_config_inherit_stdin(c.ptr())
	runtime.KeepAlive(c)
}

func (c *WasiConfig) SetStdoutFile(path string) error {
	pathC := C.CString(path)
	ok := C.wasi_config_set_stdout_file(c.ptr(), pathC)
	runtime.KeepAlive(c)
	C.free(unsafe.Pointer(pathC))
	if ok {
		return nil
	}

	return errors.New("failed to open file")
}

func (c *WasiConfig) InheritStdout() {
	C.wasi_config_inherit_stdout(c.ptr())
	runtime.KeepAlive(c)
}

func (c *WasiConfig) SetStderrFile(path string) error {
	pathC := C.CString(path)
	ok := C.wasi_config_set_stderr_file(c.ptr(), pathC)
	runtime.KeepAlive(c)
	C.free(unsafe.Pointer(pathC))
	if ok {
		return nil
	}

	return errors.New("failed to open file")
}

func (c *WasiConfig) InheritStderr() {
	C.wasi_config_inherit_stderr(c.ptr())
	runtime.KeepAlive(c)
}

func (c *WasiConfig) PreopenDir(path, guestPath string) error {
	pathC := C.CString(path)
	guestPathC := C.CString(guestPath)
	ok := C.wasi_config_preopen_dir(c.ptr(), pathC, guestPathC)
	runtime.KeepAlive(c)
	C.free(unsafe.Pointer(pathC))
	C.free(unsafe.Pointer(guestPathC))
	if ok {
		return nil
	}

	return errors.New("failed to preopen directory")
}

// FileAccessMode Indicates whether the file-like object being inserted into the
// WASI configuration (by PushFile and InsertFile) can be used to read, write,
// or both using bitflags. This seems to be a wasmtime specific mapping as it
// does not match syscall.O_RDONLY, O_WRONLY, etc.
type WasiFileAccessMode uint32

const (
	READ_ONLY WasiFileAccessMode = 1 << iota
	WRITE_ONLY
	READ_WRITE = READ_ONLY | WRITE_ONLY
)

type WasiCtx struct {
	_ptr *C.wasi_ctx_t
}

// NewWasiCtx creates a new WASI context.
//
// This is only for debugging purposes. A typical user should use
// (*Store).WasiCtx() instead, or directly use WASI functions under
// Store.
func NewWasiCtx() *WasiCtx {
	ptr := C.wasi_ctx_new()
	ctx := &WasiCtx{_ptr: ptr}
	runtime.SetFinalizer(ctx, func(ctx *WasiCtx) {
		runtime.KeepAlive(ctx) // no-op here in place of `C.wasi_ctx_delete` <- TODO
	})
	return ctx
}

func (ctx *WasiCtx) ptr() *C.wasi_ctx_t {
	ret := ctx._ptr
	maybeGC()
	return ret
}

// InsertFile inserts a file into the WASI context, it calls to the underlying C-API
// to invoke Rust's WasiCtx::insert_file method.
//
// Safety: this function is unsafe because it does not check for ownership nor does it guarantee
// memory consistency. It is the caller's responsibility to ensure that the WasiCtx is not
// currently in use by any other thread. The input file needs to be kept alive as long as the
// WasiCtx is alive, otherwise the WasiCtx may lose access to the underlying file descriptor
// in case of garbage collection.
func (ctx *WasiCtx) InsertFile(guestFD uint32, file *os.File, accessMode WasiFileAccessMode) error {
	err := C.wasi_ctx_insert_file(ctx.ptr(), C.uint32_t(guestFD), C.uintptr_t(file.Fd()), C.uint32_t(accessMode))
	runtime.KeepAlive(ctx)
	runtime.KeepAlive(file)
	if err != nil {
		return mkError(err)
	}
	return nil
}

// PushFile pushes a file into the WASI context, it calls to the underlying C-API to invoke
// Rust's WasiCtx::push_file method.
//
// Safety: this function is unsafe because it does not check for ownership nor does it guarantee
// memory consistency. It is the caller's responsibility to ensure that the WasiCtx is not
// currently in use by any other thread. The input file needs to be kept alive as long as the
// WasiCtx is alive, otherwise the WasiCtx may lose access to the underlying file descriptor
// in case of garbage collection.
func (ctx *WasiCtx) PushFile(file *os.File, accessMode WasiFileAccessMode) (uint32, error) {
	var guestFd uint32
	c_guest_fd := C.uint32_t(guestFd)

	err := C.wasi_ctx_push_file(ctx.ptr(), C.uintptr_t(file.Fd()), C.uint32_t(accessMode), &c_guest_fd)
	runtime.KeepAlive(ctx)
	runtime.KeepAlive(file)
	if err != nil {
		return 0, mkError(err)
	}
	return uint32(c_guest_fd), nil
}
