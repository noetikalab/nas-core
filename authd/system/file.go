// Package system 封装 Linux 系统级操作，包括：
//   - 文件系统：目录列表、文件读写、路径安全检查
//   - 用户管理：useradd、家目录创建、ACL 设置、用户删除
//
// 路径安全检查是所有文件操作的入口关卡：
//   - ValidatePath：普通用户，限制在 /data/{username}/ 子树内
//   - ValidateAdminPath：管理员，限制在 /data/ 子树内
package system

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileInfo 文件/目录信息，序列化为 API 响应 JSON。
type FileInfo struct {
	Name       string    `json:"name"`       // 文件或目录名
	Size       int64     `json:"size"`       // 文件大小（字节），目录为 0
	Type       string    `json:"type"`       // "file" 或 "directory"
	Modified   time.Time `json:"modified"`   // 最后修改时间
	Permission string    `json:"permission"` // 权限位字符串，如 "rwx"
}

// ValidatePath 校验请求路径是否在用户的数据目录 /data/{username}/ 下。
//
// 路径穿越攻击防护：使用 filepath.Clean 规范化路径，
// 然后检查是否以 /data/{username} 为前缀。
//
// 返回值：
//   - 规范化后的绝对路径
//   - 路径不合法时返回错误 "access denied"
func ValidatePath(username, requestedPath string) (string, error) {
	clean := filepath.Clean(requestedPath)
	// 拼接用户根目录，如 /data/alice
	userRoot := filepath.Join("/data", username)
	// 校验：请求路径必须以用户根目录为前缀
	if !strings.HasPrefix(clean, userRoot) {
		return "", fmt.Errorf("access denied")
	}
	return clean, nil
}

// ValidateAdminPath 校验请求路径是否在 /data/ 下。
// 管理员可以访问所有用户的数据目录，但限制不能超出 /data/。
func ValidateAdminPath(requestedPath string) (string, error) {
	clean := filepath.Clean(requestedPath)
	if !strings.HasPrefix(clean, "/data") {
		return "", fmt.Errorf("access denied")
	}
	return clean, nil
}

// ListDir 列出目录下的所有文件和子目录。
// 返回的 FileInfo 包含名称、大小、类型、修改时间和权限。
// 如果目录不存在或无权限，返回原始 OS 错误（调用方按类型判断）。
func ListDir(path string) ([]FileInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	files := make([]FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // 跳过无法获取信息的条目（极少发生）
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

// OpenFile 打开文件并返回 reader、文件大小。
// 调用方负责在使用完毕后 reader.Close()。
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

// WriteFile 将 reader 的内容写入指定路径，自动创建父目录（权限 0750）。
//
// uid 参数指定文件创建后的 owner UID（通常为实际用户的 UID 而非进程的 root）。
// Go 进程以 root 运行，os.Create 创建的文件的 owner 是 root，
// 导致该文件通过 SMB/NFS 以用户身份访问时权限不匹配。
// 因此写入后必须 chown 给真正的用户。
//
// 返回值：写入错误或 chown 错误。chown 失败不影响写入成功（仅记录日志即可）。
func WriteFile(path string, src io.Reader, uid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = io.Copy(f, src); err != nil {
		return err
	}
	// root 进程创建文件的 owner 是 root:root，chown 为实际用户
	// 使得 SMB/NFS 能以用户 UID 正常访问该文件
	_ = os.Chown(path, uid, 1000)
	return nil
}

// MakeDir 创建目录（含所有父目录），权限 0750。
//
// uid 参数指定目录创建后的 owner UID，原因同 WriteFile 的说明。
// 父目录不存在时也会被一并创建（权限 0750），但仅最后的目录会被 chown。
func MakeDir(path string, uid int) error {
	if err := os.MkdirAll(path, 0750); err != nil {
		return err
	}
	// chown 给真实用户，确保 SMB/NFS 能正常访问
	_ = os.Chown(path, uid, 1000)
	return nil
}

// DeletePath 递归删除路径（文件或目录）。危险操作，调用前务必做路径校验。
func DeletePath(path string) error {
	return os.RemoveAll(path)
}

// MovePath 移动/重命名文件或目录。from 和 to 必须在同一文件系统上。
func MovePath(from, to string) error {
	return os.Rename(from, to)
}

// permStr 将 os.FileMode 转换为 "rwx" 格式的三字符权限字符串。
// 只取 user 权限位（owner），忽略 group/other。
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
