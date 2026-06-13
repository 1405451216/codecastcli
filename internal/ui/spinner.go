package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// spinnerChars Unicode spinner 动画字符
var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner 管理一个轻量级 Unicode spinner 动画
type Spinner struct {
	mu       sync.Mutex
	message  string
	active   bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	writer   io.Writer
	frame    int
}

var defaultSpinner *Spinner

// StartSpinner 启动全局 spinner，显示指定消息
func StartSpinner(message string) {
	if defaultSpinner != nil {
		defaultSpinner.Stop()
	}
	defaultSpinner = NewSpinner(os.Stderr)
	defaultSpinner.Start(message)
}

// StopSpinner 停止全局 spinner
func StopSpinner() {
	if defaultSpinner != nil {
		defaultSpinner.Stop()
		defaultSpinner = nil
	}
}

// UpdateSpinnerMessage 更新全局 spinner 的消息
func UpdateSpinnerMessage(message string) {
	if defaultSpinner != nil {
		defaultSpinner.UpdateMessage(message)
	}
}

// NewSpinner 创建一个新的 Spinner 实例
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{
		writer: w,
	}
}

// Start 启动 spinner 动画
func (s *Spinner) Start(message string) {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.message = message
	s.active = true
	s.stopCh = make(chan struct{})
	s.doneCh = make(chan struct{})
	s.frame = 0
	s.mu.Unlock()

	go s.run()
}

// Stop 停止 spinner 动画并清除行
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.stopCh)
	s.mu.Unlock()

	// 等待 goroutine 退出
	<-s.doneCh

	// 清除 spinner 行
	fmt.Fprint(s.writer, "\r\033[K")
}

// UpdateMessage 更新 spinner 显示的消息
func (s *Spinner) UpdateMessage(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.message = message
}

// IsActive 返回 spinner 是否正在运行
func (s *Spinner) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *Spinner) run() {
	defer close(s.doneCh)

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			if !s.active {
				s.mu.Unlock()
				return
			}
			char := spinnerChars[s.frame%len(spinnerChars)]
			msg := s.message
			s.frame++
			s.mu.Unlock()

			// \r 回到行首，\033[K 清除行尾
			fmt.Fprintf(s.writer, "\r\033[K%s %s", char, msg)
		}
	}
}
