package router

import (
	"os"
	"path/filepath"
)

func detectRoot(path string, markers []string) string {
	dir := filepath.Dir(path)
	for {
		for _, marker := range markers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(path)
		}
		dir = parent
	}
}
