//go:build darwin

package rules

import (
	"os"
	"time"

	"golang.org/x/sys/unix"
)

func getBirthTime(info os.FileInfo) time.Time {
	if stat, ok := info.Sys().(*unix.Stat_t); ok {
		birth := time.Unix(stat.Btim.Sec, stat.Btim.Nsec)
		if !birth.IsZero() {
			return birth
		}
	}
	return info.ModTime()
}
