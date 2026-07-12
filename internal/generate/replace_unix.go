//go:build !windows

package generate

import "os"

func replaceFile(source, target string) error {
	return os.Rename(source, target)
}
