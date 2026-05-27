// Package logbuf 提供线程安全的环形缓冲，用于记录系统操作日志。
// 所有 admin 文件操作、用户管理操作都会写入此缓冲，
// 供 Dashboard "最近操作" 和审计日志页面读取。
package logbuf

import (
	"sync"
	"time"
)

// Entry 表示一条操作日志记录。
type Entry struct {
	Timestamp time.Time `json:"timestamp"` // 操作时间（UTC）
	Type      string    `json:"type"`      // 日志类型："file" / "auth" / "system"
	User      string    `json:"user"`      // 操作者用户名
	Action    string    `json:"action"`    // 操作名称：upload / delete / mkdir / move / download
	Path      string    `json:"path"`      // 相关文件路径
	Size      int64     `json:"size"`      // 文件大小（字节），非文件操作为 0
	Detail    string    `json:"detail"`    // 补充说明
}

// RingBuffer 是一个固定容量的线程安全环形缓冲。
// 写满后新条目覆盖最旧条目。
type RingBuffer struct {
	mu   sync.RWMutex
	buf  []Entry // 底层存储数组
	head int     // 下一个写入位置
	size int     // 当前已存储条目数（不超过 cap）
	cap  int     // 最大容量
}

// Default 是全局共享的默认环形缓冲实例，容量 1000 条。
// 所有 handler 共用此实例，通过 Append 写入，通过 Recent/Filter 读取。
var Default = New(1000)

// New 创建一个指定容量的环形缓冲。
func New(cap int) *RingBuffer {
	return &RingBuffer{
		buf: make([]Entry, cap),
		cap: cap,
	}
}

// Append 向缓冲追加一条日志。如果缓冲已满，覆盖最旧的条目。
// e.Timestamp 会被自动设置为当前时间。
func (rb *RingBuffer) Append(e Entry) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	e.Timestamp = time.Now()
	rb.buf[rb.head] = e
	rb.head = (rb.head + 1) % rb.cap
	if rb.size < rb.cap {
		rb.size++
	}
}

// Recent 返回最近的 limit 条日志，按时间倒序（最新在前）。
func (rb *RingBuffer) Recent(limit int) []Entry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if limit > rb.size {
		limit = rb.size
	}
	result := make([]Entry, limit)
	for i := 0; i < limit; i++ {
		// 从 head-1 开始（最新条目），向前遍历
		idx := (rb.head - 1 - i + rb.cap) % rb.cap
		result[limit-1-i] = rb.buf[idx]
	}
	return result
}

// Filter 返回指定类型的最近 limit 条日志，按时间正序排列。
// entryType 为 "all" 时不过滤类型。
func (rb *RingBuffer) Filter(entryType string, limit int) []Entry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	result := make([]Entry, 0, limit)
	for i := 0; i < rb.size && len(result) < limit; i++ {
		idx := (rb.head - 1 - i + rb.cap) % rb.cap
		e := rb.buf[idx]
		if entryType == "all" || e.Type == entryType {
			result = append(result, e)
		}
	}
	// 反转为时间正序
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}
