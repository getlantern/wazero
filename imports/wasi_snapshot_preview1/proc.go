package wasi_snapshot_preview1

import (
	"context"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasip1"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/sys"
)

// procExit is the WASI function named ProcExitName that terminates the
// execution of the module with an exit code. The only successful exit code is
// zero.
//
// # Parameters
//
//   - exitCode: exit code.
//
// See https://github.com/WebAssembly/WASI/blob/main/phases/snapshot/docs.md#proc_exit
var procExit = &wasm.HostFunc{
	ExportName: wasip1.ProcExitName,
	Name:       wasip1.ProcExitName,
	ParamTypes: []api.ValueType{i32},
	ParamNames: []string{"rval"},
	Code:       wasm.Code{GoFunc: api.GoModuleFunc(procExitFn)},
}

func procExitFn(ctx context.Context, mod api.Module, params []uint64) {
	exitCode := uint32(params[0])

	if exitCode != 0 {
		// Only close the module on non-zero (error) exit codes.
		// Exit code 0 means success — the module should remain usable
		// so that exported functions can still be called after _start
		// returns. This is needed for runtimes like TinyGo 0.40+ which
		// call proc_exit(0) after main() completes.
		_ = mod.CloseWithExitCode(ctx, exitCode)
	}

	// Prevent any code from executing after this function. For example, LLVM
	// inserts unreachable instructions after calls to exit.
	// See: https://github.com/emscripten-core/emscripten/issues/12322
	panic(sys.NewExitError(exitCode))
}

// procRaise is stubbed and will never be supported, as it was removed.
//
// See https://github.com/WebAssembly/WASI/pull/136
var procRaise = stubFunction(wasip1.ProcRaiseName, []api.ValueType{i32}, "sig")
