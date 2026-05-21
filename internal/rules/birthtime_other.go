//go:build !darwin

package rules

import (
	"os"
	"time"
)

func getBirthTime(info os.FileInfo) time.Time {
	return info.ModTime()
}
