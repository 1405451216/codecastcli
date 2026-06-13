package splash

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Splash 启动动画
type Splash struct {
	logo       string
	tagline    string
	version    string
	nodes      []node
	width      int
	height     int
	done       chan struct{}
	// startOnce 只保护 goroutine 启动，确保 RunAsync 只启动一次
	startOnce  sync.Once
	// finishOnce 保护 Finish 的 close(done) 也只执行一次
	finishOnce sync.Once
	frameDelay time.Duration
	// 上一次节点位置（用于差异绘制，避免闪屏）
	prevNodeX []int
	prevNodeY []int
	// LOGO 区域（一次绘制，不再重绘）
	logoY     int
	logoStart int
	tagStart  int
	verStart  int
	staticDrawn bool
}

type node struct {
	x, y     int
	char     rune
	speed    float64
	phase    float64
	color    byte // 0=purple, 1=cyan, 2=magenta
}

// DefaultSplash 创建默认的 Codecast 启动动画
func DefaultSplash() *Splash {
	s := &Splash{
		logo:       "CODECAST",
		tagline:    "AI-POWERED TERMINAL AGENT",
		version:    "v0.1.0",
		done:       make(chan struct{}),
		frameDelay: 80 * time.Millisecond,
	}
	s.detectSize()
	s.initNodes()
	return s
}

// Run 运行启动动画，阻塞直到动画完成
func (s *Splash) Run() {
	s.RunAsync()
	<-s.done
}

// RunFor 运行启动动画 N 帧后自动结束（用于测试和 Headless 模式）
func (s *Splash) RunFor(frames int) {
	s.startOnce.Do(func() {
		go s.renderLoopN(frames)
	})
	<-s.done
}

// RunAsync 异步运行启动动画
func (s *Splash) RunAsync() {
	s.startOnce.Do(func() {
		go s.renderLoop()
		go s.signalHandler()
	})
}

// Wait 等待动画完成
func (s *Splash) Wait() {
	<-s.done
}

// renderLoopN 渲染 N 帧后自动结束
func (s *Splash) renderLoopN(frames int) {
	ticker := time.NewTicker(s.frameDelay)
	defer ticker.Stop()

	for i := 0; i < frames; i++ {
		s.render(i)
		<-ticker.C
	}
	s.Finish()
}

func (s *Splash) detectSize() {
	w, h := 80, 24
	if runtime.GOOS != "windows" {
		w, h, _ = getTerminalSize()
	}
	if w < 40 {
		w = 80
	}
	if h < 12 {
		h = 24
	}
	s.width = w
	s.height = h
}

func (s *Splash) initNodes() {
	numNodes := 3
	s.nodes = make([]node, numNodes)
	s.prevNodeX = make([]int, numNodes)
	s.prevNodeY = make([]int, numNodes)
	// 3 个节点：左上、右上、底部
	positions := []struct{ x, y int }{
		{s.width / 4, 3},
		{3 * s.width / 4, 3},
		{s.width / 2, s.height - 2},
	}
	for i := 0; i < numNodes; i++ {
		s.nodes[i] = node{
			x:     positions[i].x,
			y:     positions[i].y,
			char:  []rune("●○◉")[i],
			speed: 0.3 + rand.Float64()*0.4,
			phase: rand.Float64() * math.Pi * 2,
			color: byte(i),
		}
		s.prevNodeX[i] = positions[i].x
		s.prevNodeY[i] = positions[i].y
	}
	// 预计算 LOGO 位置
	s.logoY = s.height / 2
	s.logoStart = (s.width - len(s.logo)) / 2
	s.tagStart = (s.width - len(s.tagline)) / 2
	s.verStart = (s.width - len(s.version)) / 2
}

func (s *Splash) renderLoop() {
	frameTime := 80 * time.Millisecond
	ticker := time.NewTicker(frameTime)
	defer ticker.Stop()

	frame := 0
	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			s.render(frame)
			frame++
		}
	}
}

func (s *Splash) render(frame int) {
	if !s.staticDrawn {
		s.drawStatic()
		s.staticDrawn = true
	}
	s.drawNodes(frame)
}

// drawStatic 一次性绘制 LOGO 区域，之后不再重绘
func (s *Splash) drawStatic() {
	fmt.Print("\033[2J\033[H") // 清屏
	fmt.Print("\033[?25l")     // 隐藏光标

	// 顶部空行
	for y := 0; y < s.logoY-1; y++ {
		fmt.Print("\n")
	}
	// LOGO 行
	fmt.Print("\033[38;2;99;102;241m")
	fmt.Print(strings.Repeat(" ", s.logoStart))
	fmt.Print(s.logo)
	fmt.Print("\033[0m\n")
	// Tagline 行
	fmt.Print(strings.Repeat(" ", s.tagStart))
	fmt.Print("\033[38;2;139;92;246m")
	fmt.Print(s.tagline)
	fmt.Print("\033[0m\n")
	// Version 行
	fmt.Print(strings.Repeat(" ", s.verStart))
	fmt.Print("\033[38;2;99;99;99m")
	fmt.Print(s.version)
	fmt.Print("\033[0m")
}

// drawNodes 增量绘制节点（只在变化位置操作光标）
func (s *Splash) drawNodes(frame int) {
	for _, n := range s.nodes {
		// 节点原地脉冲（不移动位置，避免 PowerShell 渲染问题）
		nx := n.x
		ny := n.y

		// 跳过 LOGO 区域
		if ny == s.logoY || ny == s.logoY+1 || ny == s.logoY+2 {
			if nx >= s.logoStart-1 && nx <= s.logoStart+len(s.logo)+1 {
				continue
			}
		}

		// 脉冲效果（字符在 ●/○ 之间切换 + 颜色亮度）
		pulse := (math.Sin(float64(frame)*n.speed*0.2 + n.phase) + 1) / 2
		var ch rune
		if pulse > 0.7 {
			ch = '●'
		} else {
			ch = '○'
		}

		// ANSI 颜色（按节点 color 字段 + 脉冲亮度）
		var color string
		switch n.color {
		case 0:
			if pulse > 0.5 {
				color = "\033[38;2;99;102;241m"  // 亮紫
			} else {
				color = "\033[38;2;60;60;90m"    // 暗紫
			}
		case 1:
			if pulse > 0.5 {
				color = "\033[38;2;6;182;212m"   // 亮青
			} else {
				color = "\033[38;2;40;70;80m"    // 暗青
			}
		case 2:
			if pulse > 0.5 {
				color = "\033[38;2;217;70;239m"  // 亮品红
			} else {
				color = "\033[38;2;80;40;90m"    // 暗品红
			}
		}
		// 移动光标到节点位置并打印
		fmt.Printf("\033[%d;%dH%s%c\033[0m", ny+1, nx+1, color, ch)
	}
	// 光标移到底部
	fmt.Printf("\033[%d;1H", s.height)
}

// signalHandler 监听 OS 信号，被取消时退出。
//
// v0.2.0 修复：原实现永久阻塞在 <-sigChan（测试环境不会发信号），
// 导致信号 goroutine 永远占用，且无法唤醒 done。
// 现改为 select 同时监听 sigChan 和 s.done，s.done 被关闭时立即退出。
func (s *Splash) signalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	select {
	case <-sigChan:
		// 用户按 Ctrl+C / SIGTERM → 关闭 splash
		s.Finish()
	case <-s.done:
		// 已被 Finish 关闭 → 直接退出
	}
}

// Finish 优雅结束动画。
//
// v0.2.0 修复：原实现使用 sync.Once，但这个 once 与 RunAsync 启动
// goroutine 的 once 共享，导致 RunAsync 先调用后 Finish 的 once
// 永远不触发，splash 永远不停。
// 现在 Finish 使用独立的 finishOnce，确保每次调用都有效。
//
// 另外：close(s.done) 后，所有监听 s.done 的 goroutine 都会被唤醒，
// 包括 renderLoop 和 signalHandler，从而让所有后台任务自然退出。
func (s *Splash) Finish() {
	s.finishOnce.Do(func() {
		close(s.done)
		// 恢复光标，清屏
		fmt.Print("\033[?25h\033[2J\033[H")
		fmt.Println()
	})
}

// getTerminalSize 获取终端尺寸（Unix）
func getTerminalSize() (int, int, error) {
	// Windows 下返回默认值
	return 80, 24, nil
}

// PrintBanner 简化版：一次性打印启动横幅（无动画）
func PrintBanner(version string) {
	fmt.Println()
	logo := `
  ██████╗ ██╗   ██╗███╗   ██╗██╗  ██╗███████╗██████╗
 ██╔════╝ ██║   ██║████╗  ██║██║ ██╔╝██╔════╝██╔══██╗
 ██║      ██║   ██║██╔██╗ ██║█████╔╝ █████╗  ██████╔╝
 ██║      ██║   ██║██║╚██╗██║██╔═██╗ ██╔══╝  ██╔══██╗
 ╚██████╗ ╚██████╔╝██║ ╚████║██║  ██╗███████╗██║  ██║
  ╚═════╝  ╚═════╝ ╚═╝  ╚═══╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝`

	tagline := "  AI-POWERED TERMINAL AGENT"
	line := strings.Repeat("─", 60)

	fmt.Print("\033[38;2;99;102;241m") // 紫色
	fmt.Println(logo)
	fmt.Print("\033[38;2;168;85;247m") // 紫色
	fmt.Println(tagline)
	fmt.Print("\033[0m")             // 重置
	fmt.Println(line)
	fmt.Printf("  Version:  %s\n", version)
	fmt.Printf("  Provider: OpenAI\n")
	fmt.Printf("  Framework: AgentPrimordia\n")
	fmt.Println(line)
	fmt.Println()
}
