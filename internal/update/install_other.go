//go:build !linux && !windows

package update

import (
	"context"
	"github.com/nodelane/nodelane-room/internal/model"
)

func SupportedInstall(string) bool { return false }
func StartInstall(context.Context, string) error {
	return model.Failure("local_update_install_unsupported")
}
func Bootstrap(context.Context, string) error {
	return model.Failure("local_update_install_unsupported")
}
func installLock(string) (func(), error) {
	return nil, model.Failure("local_update_install_unsupported")
}
func FreeSpace(string) (uint64, error) { return 0, model.Failure("local_update_install_unsupported") }
func installPackage(context.Context, string, InstallJob) error {
	return model.Failure("local_update_install_unsupported")
}
func installFailure(context.Context, string, InstallJob) (string, string) {
	return "failed", "local_update_install_unsupported"
}
