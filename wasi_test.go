package wasmtime

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWasiConfig(t *testing.T) {
	config := NewWasiConfig()
	config.SetEnv([]string{"WASMTIME"}, []string{"GO"})
}

func TestWasiCtx(t *testing.T) {
	engine := NewEngine()
	store := NewStore(engine)

	wasm, err := Wat2Wasm(`
	(module
	  ;; Import the required fd_write WASI function which will write the given io vectors to stdout
	  ;; The function signature for fd_write is:
	  ;; (File Descriptor, *iovs, iovs_len, nwritten) -> Returns number of bytes written
	  (import "wasi_snapshot_preview1" "fd_write" (func $fd_write (param i32 i32 i32 i32) (result i32)))

	  (memory 1)
	  (export "memory" (memory 0))

	  ;; Write 'hello world\n' to memory at an offset of 8 bytes
	  ;; Note the trailing newline which is required for the text to appear
	  (data (i32.const 8) "hello world\n")
	  (data (i32.const 32) "thank you\n")

	  (func $main (export "_start")
		;; Creating a new io vector within linear memory
		(i32.store (i32.const 0) (i32.const 8))  ;; iov.iov_base - This is a pointer to the start of the 'hello world\n' string
		(i32.store (i32.const 4) (i32.const 12))  ;; iov.iov_len - The length of the 'hello world\n' string

	  	(call $fd_write
		  (i32.const 1) ;; file_descriptor - 1 for stdout
		  (i32.const 0) ;; *iovs - The pointer to the iov array, which is stored at memory location 0
		  (i32.const 1) ;; iovs_len - We're printing 1 string stored in an iov - so one.
		  (i32.const 80) ;; nwritten - A place in memory to store the number of bytes written
	    )
	  	drop ;; Discard the number of bytes written from the top of the stack
	  )

	  (func $thankyou 
		(param $fd i32)
	    (i32.store (i32.const 0) (i32.const 32))  ;; iov.iov_base - This is a pointer to the start of the 'hello world\n' string
	    (i32.store (i32.const 4) (i32.const 10))  ;; iov.iov_len - The length of the 'thank you\n' string

	    (call $fd_write
	      (local.get $fd) ;; file_descriptor
	      (i32.const 0) ;; *iovs - The pointer to the iov array, which is stored at memory location 0
	      (i32.const 1) ;; iovs_len - We're printing 1 string stored in an iov - so one.
	      (i32.const 20) ;; nwritten - A place in memory to store the number of bytes written
	    )
	    drop ;; Discard the number of bytes written from the top of the stack
	  )
	  (export "_thankyou" (func $thankyou))
	)
	`)
	if err != nil {
		t.Fatal(err)
	}

	module, err := NewModule(engine, wasm)
	if err != nil {
		t.Fatal(err)
	}

	// Create a linker with WASI functions defined within it
	linker := NewLinker(engine)
	err = linker.DefineWasi()
	if err != nil {
		t.Fatal(err)
	}

	wasiConfig := NewWasiConfig()
	// wasiConfig.InheritStdout()
	store.SetWasiConfig(wasiConfig)
	instance, err := linker.Instantiate(store, module)
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	hello := instance.GetFunc(store, "_start")
	_, err = hello.Call(store)
	if err != nil {
		t.Fatal(err)
	}

	wasiCtx := store.WasiCtx()
	require.NotNil(t, wasiCtx)

	/// Test#1: WasiCtx.InsertFile
	// Create a file
	file, err := os.CreateTemp("", "wasi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	t.Logf("Created file %s for WasiCtx.InsertFile", file.Name())

	// Insert the file into the WASI context with file descriptor 14
	err = wasiCtx.InsertFile(14, file, READ_WRITE)
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	thankyou := instance.GetFunc(store, "_thankyou")
	_, err = thankyou.Call(store, 14)
	if err != nil {
		t.Fatal(err)
	}

	filename1 := file.Name()
	// read the output
	out, err := os.ReadFile(filename1)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "thank you\n", string(out))
	t.Logf("WASI output: %s", string(out))

	/// Test#2: WasiCtx.PushFile
	// Create another file
	file, err = os.CreateTemp("", "wasi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	t.Logf("Created file %s for WasiCtx.PushFile", file.Name())

	fd, err := wasiCtx.PushFile(file, READ_WRITE)
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	_, err = thankyou.Call(store, int32(fd))
	if err != nil {
		t.Fatal(err)
	}
	// read the output
	out, err = os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, "thank you\n", string(out))
	t.Logf("WASI output: %s", string(out))

	/// Test#3: Check if previously inserted file is still accessible
	// Run the function again
	_, err = thankyou.Call(store, 14)
	if err != nil {
		t.Fatal(err)
	}

	// read the output
	out, err = os.ReadFile(filename1)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "thank you\nthank you\n", string(out))
	t.Logf("WASI output: %s", string(out))

	/// Test#4: Store.InsertFile
	// Create a file
	file, err = os.CreateTemp("", "wasi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	t.Logf("Created file %s for Store.InsertFile", file.Name())

	// Insert the file into the WASI context with file descriptor 24
	err = store.InsertFile(24, file, READ_WRITE)
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	_, err = thankyou.Call(store, 24)
	if err != nil {
		t.Fatal(err)
	}

	// read the output
	out, err = os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "thank you\n", string(out))
	t.Logf("WASI output: %s", string(out))

	/// Test#5: Store.PushFile
	// Create another file
	file, err = os.CreateTemp("", "wasi_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file.Name())
	t.Logf("Created file %s for Store.PushFile", file.Name())

	// Push the file into the WASI context
	fd2, err := store.PushFile(file, READ_WRITE)
	if err != nil {
		t.Fatal(err)
	}

	// Run the function
	_, err = thankyou.Call(store, int32(fd2))
	if err != nil {
		t.Fatal(err)
	}

	// read the output
	out, err = os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, "thank you\n", string(out))
	t.Logf("WASI output: %s", string(out))
}
