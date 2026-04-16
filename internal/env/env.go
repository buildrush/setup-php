package env

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
)

type Exporter struct {
	envFile    string
	outputFile string
	pathFile   string
}

func NewExporter() (*Exporter, error) {
	return &Exporter{
		envFile:    os.Getenv("GITHUB_ENV"),
		outputFile: os.Getenv("GITHUB_OUTPUT"),
		pathFile:   os.Getenv("GITHUB_PATH"),
	}, nil
}

func (e *Exporter) AddPath(dir string) error {
	return appendToFile(e.pathFile, dir+"\n")
}

func (e *Exporter) SetEnv(key, value string) error {
	delim := generateDelimiter()
	content := fmt.Sprintf("%s<<%s\n%s\n%s\n", key, delim, value, delim)
	return appendToFile(e.envFile, content)
}

func (e *Exporter) SetOutput(key, value string) error {
	delim := generateDelimiter()
	content := fmt.Sprintf("%s<<%s\n%s\n%s\n", key, delim, value, delim)
	return appendToFile(e.outputFile, content)
}

func appendToFile(path, content string) (err error) {
	if path == "" {
		return nil
	}
	f, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = f.WriteString(content)
	return err
}

func generateDelimiter() string {
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("ghadelimiter_%x", b)
}
