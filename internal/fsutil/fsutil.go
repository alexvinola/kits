// Package fsutil holds small filesystem helpers.
package fsutil

import (
	"io/fs"
	"os"
	"path/filepath"
)

// WriteAtomic writes data to name through a temporary file in the same
// directory and a rename, so readers never see a half-written file.
func WriteAtomic(name string, data []byte, perm fs.FileMode) (err error) {
	f, err := os.CreateTemp(filepath.Dir(name), ".kits-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Chmod(perm); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, name)
}
