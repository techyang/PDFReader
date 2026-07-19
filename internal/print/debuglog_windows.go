// internal/print/debuglog_windows.go
package print

import (
	"log"
	"os"
	"path/filepath"
)

// debugLogFile is a TEMPORARY diagnostic aid for tracking down the
// "打印完最后一页后卡死，标题栏未响应" hang (2026-07-19) - every major
// step of a print job is timestamped here so a hung run's log tail shows
// exactly which call never returned. Remove once the hang is root-caused
// and fixed.
var debugLogFile = openDebugLog()

func openDebugLog() *log.Logger {
	f, err := os.OpenFile(filepath.Join(os.TempDir(), "pdfreader_print_debug.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return log.New(os.Stderr, "print-debug: ", log.LstdFlags|log.Lmicroseconds)
	}
	return log.New(f, "", log.LstdFlags|log.Lmicroseconds)
}

// Debugf appends a timestamped diagnostic line - see debugLogFile.
func Debugf(format string, args ...interface{}) {
	debugLogFile.Printf(format, args...)
}
