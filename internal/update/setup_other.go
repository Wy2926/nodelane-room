//go:build !windows

package update

import "context"

func SetupCommand(context.Context, []string) (bool, error) { return false, nil }
