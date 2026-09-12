// The privileged updater runs outside the networking service's lifetime.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nodelane/nodelane-room/internal/platform"
	"github.com/nodelane/nodelane-room/internal/update"
)

func main() {
	ctx := context.Background()
	if handled, err := update.SetupCommand(ctx, os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	dir := platform.DefaultDir()
	if err := platform.SecureDir(dir); err != nil {
		fmt.Fprintln(os.Stderr, "update permission denied")
		os.Exit(1)
	}
	var err error
	if len(os.Args) == 2 && os.Args[1] == "launch" {
		err = update.Bootstrap(ctx, dir)
	} else if len(os.Args) == 2 && os.Args[1] == "apply" {
		err = update.Apply(ctx, dir)
	} else {
		fmt.Fprintln(os.Stderr, "usage: nlroom-update launch|apply")
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "update failed; inspect the protected update result")
		os.Exit(1)
	}
}
