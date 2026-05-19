package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type FileInfo struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Type       string    `json:"type"` // "file" or "directory"
	Modified   time.Time `json:"modified"`
	Permission string    `json:"permission"`
}

func ValidatePath(username, requestedPath string) (string, error) {
	clean := filepath.Clean(requestedPath)
	userRoot := filepath.Join("/data", username)
	if !strings.HasPrefix(clean, userRoot) {
		return "", fmt.Errorf("access denied")
	}
	return clean, nil
}

func ListDir(path string) ([]FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fi := FileInfo{
			Name:     e.Name(),
			Modified: info.ModTime(),
		}
		if e.IsDir() {
			fi.Type = "directory"
		} else {
			fi.Type = "file"
			fi.Size = info.Size()
		}
		fi.Permission = permStr(info.Mode())
		files = append(files, fi)
	}
	return files, nil
}

func OpenFile(path string) (io.ReadCloser, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

func WriteFile(path string, src io.Reader) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, src)
	return err
}

func MakeDir(path string) error {
	return os.MkdirAll(path, 0750)
}

func DeletePath(path string) error {
	return os.RemoveAll(path)
}

func MovePath(from, to string) error {
	return os.Rename(from, to)
}

func permStr(mode os.FileMode) string {
	b := make([]byte, 3)
	if mode&0400 != 0 {
		b[0] = 'r'
	} else {
		b[0] = '-'
	}
	if mode&0200 != 0 {
		b[1] = 'w'
	} else {
		b[1] = '-'
	}
	if mode&0100 != 0 {
		b[2] = 'x'
	} else {
		b[2] = '-'
	}
	return string(b)
}
