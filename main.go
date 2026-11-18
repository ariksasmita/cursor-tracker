// Command cursor-tracker is a terminal dashboard for Cursor AI spend.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sasmitai/cursor-tracker/internal/app"
)

func main() {
	if err := app.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "cursor-tracker:", err)
		os.Exit(1)
	}
}
