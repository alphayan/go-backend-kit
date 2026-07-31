//go:build windows

package generate

import "os"

func replaceFile(root *os.Root, source, target string) error {
	return root.Rename(source, target)
}
