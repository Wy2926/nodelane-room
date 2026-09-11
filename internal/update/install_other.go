//go:build !linux && !windows

package update

import (
	"context"
	"errors"
)

func SupportedInstall(string) bool               { return false }
func finishInstall()                             {}
func StartInstall(context.Context, string) error { return errors.New("update_install_unsupported") }
func Bootstrap(context.Context, string) error    { return errors.New("update_install_unsupported") }
func installLock(string) (func(), error)         { return nil, errors.New("update_install_unsupported") }
func FreeSpace(string) (uint64, error)           { return 0, errors.New("update_install_unsupported") }
func installPackage(context.Context, string, InstallJob) error {
	return errors.New("update_install_unsupported")
}
func rollbackPackage(context.Context, string, InstallJob) error {
	return errors.New("update_install_unsupported")
}
