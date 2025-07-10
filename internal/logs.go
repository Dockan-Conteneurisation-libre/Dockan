package internal

import (
	"io"
	"os"
	"path/filepath"
)

// LogWriters retourne des io.Writer pour stdout et stderr, logguant aussi dans un fichier logs/dockan.log
func LogWriters(imagePath string) (stdout, stderr io.Writer, close func(), err error) {
	logsDir := filepath.Join(imagePath, "logs")
	os.MkdirAll(logsDir, 0755)
	logFile, err := os.OpenFile(filepath.Join(logsDir, "dockan.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, nil, nil, err
	}
	mw := io.MultiWriter(os.Stdout, logFile)
	closeFn := func() { logFile.Close() }
	return mw, mw, closeFn, nil
}
