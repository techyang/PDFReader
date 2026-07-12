// main.go
package main

import (
	"fmt"
	"os"

	"pdfreader/internal/ui"
)

func main() {
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
