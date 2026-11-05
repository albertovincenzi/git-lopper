package lopper

import (
	"io/fs"
	"path/filepath"
	"syscall"
)

// DiskSize is the space that the files under path take on disk, like du -s.
// What it cannot read counts as 0.
//
// shortcut: a file with several hard links counts once per link; du counts it once.
func DiskSize(path string) int64 {
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable folder: skip it, go on
		}
		if info, err := d.Info(); err == nil {
			if st, ok := info.Sys().(*syscall.Stat_t); ok {
				total += st.Blocks * 512
			} else {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// DiskSizes measures the paths in parallel. found runs once per path, from several goroutines.
func DiskSizes(paths []string, found func(path string, size int64)) {
	forEach(len(paths), func(i int) { found(paths[i], DiskSize(paths[i])) })
}
