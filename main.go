// main.go
package main

import (
	"fmt"
	"os"

	"pdfreader/internal/ui"
)

func main() {
	fixStdHandles()

	var initialFile string
	if len(os.Args) > 1 {
		if info, err := os.Stat(os.Args[1]); err == nil && !info.IsDir() {
			initialFile = os.Args[1]
		}
	}

	if _, err := ui.Run(initialFile); err != nil {
		fmt.Fprintln(os.Stderr, "pdfreader:", err)
		os.Exit(1)
	}
}

// fixStdHandles replaces os.Stdout/os.Stderr with the NUL device when
// they're not usable file handles. The exe is built with -H=windowsgui
// (see README) so it never gets a console of its own; when launched with
// no inherited console either (e.g. double-clicked from Explorer),
// Windows leaves the process with invalid standard handles. go-pdfium's
// wazero WebAssembly runtime calls GetFileType on os.Stdout while
// instantiating its module (inside pdfengine.NewPool, called from
// ui.Run below), and crashes with "could not instantiate webassembly
// module: getFileType /dev/stdout the handle is invalid" before the
// window ever opens if that handle is invalid. Running from a terminal
// (which does have valid inherited handles) is unaffected - Stat only
// fails, and only gets replaced, when the handle is genuinely unusable.
func fixStdHandles() {
	if _, err := os.Stdout.Stat(); err != nil {
		if f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
			os.Stdout = f
		}
	}
	if _, err := os.Stderr.Stat(); err != nil {
		if f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
			os.Stderr = f
		}
	}
}
