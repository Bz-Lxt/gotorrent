package recordio

import (
	"os"
	"path/filepath"
	"time"
)

var beijing = time.FixedZone("CST", 8*3600)

// DirFile joins dir with a dated name so operators can find today's log.
func DirFile(dir, prefix string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	day := time.Now().In(beijing).Format("20060102")
	return filepath.Join(dir, prefix+"-"+day+".rec"), nil
}

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
