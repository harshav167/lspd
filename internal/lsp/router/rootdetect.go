package router

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/harshav167/lspd/internal/config"
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

type route struct {
	languageName string
	lang         config.LanguageConfig
	root         string
	key          string
}

func resolveRoute(cfg config.Config, path string) (route, error) {
	ext := filepath.Ext(path)
	languageName, ok := cfg.LanguageByExt[ext]
	if !ok {
		return route{}, fmt.Errorf("unsupported extension %s", ext)
	}
	lang, ok := cfg.Languages[languageName]
	if !ok {
		return route{}, fmt.Errorf("language %s not configured", languageName)
	}
	root := detectRoot(path, lang.RootMarkers)
	return route{
		languageName: languageName,
		lang:         lang,
		root:         root,
		key:          languageName + ":" + root,
	}, nil
}
