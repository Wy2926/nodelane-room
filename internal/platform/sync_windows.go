//go:build windows

package platform

// Windows identity files retain DPAPI/ACL protection and use the existing atomic
// replacement path; opening a directory for fsync is unsupported on Windows.
func SyncDir(string) error { return nil }
