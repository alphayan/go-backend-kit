//go:build windows

package generate

import "golang.org/x/sys/windows"

func replaceFile(source, target string) error {
	return windows.Rename(source, target)
}
