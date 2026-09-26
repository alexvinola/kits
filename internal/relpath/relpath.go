// Package relpath validates project-relative paths coming from config, kits
// or the lock file before kits reads or writes anything through them.
package relpath

import (
	"fmt"
	"path"
	"strings"
)

// Check requires a clean, slash-separated path that stays inside the
// project: no absolute paths, no "..", no backslashes, no redundant parts.
func Check(p string) error {
	switch {
	case p == "":
		return fmt.Errorf("empty path")
	case strings.Contains(p, `\`):
		return fmt.Errorf("%q: use forward slashes", p)
	case path.IsAbs(p) || (len(p) >= 2 && p[1] == ':'):
		return fmt.Errorf("%q: must be relative to the project root", p)
	case path.Clean(p) != p || p == ".":
		return fmt.Errorf("%q: not a clean path (want %q)", p, path.Clean(p))
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return fmt.Errorf("%q: must not leave the project root", p)
		}
	}
	return nil
}
