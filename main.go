// main.go
package main

import (
	"fmt"
	"os"

	"pdfreader/internal/ui"
)

func main() {
	if _, err := ui.Run(""); err != nil {
		fmt.Fprintln(os.Stderr, "pdfreader:", err)
		os.Exit(1)
	}
}
