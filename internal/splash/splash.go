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

	"codecast/cli/internal/version"
	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

// ─── 调色板 ────────────────────────────────────────────────────
// 紫→青品牌色，保留 MiMo 风格暗底
var palette = struct {
	purple  string // #8b5cf6 亮紫
	violet  string // #a855f7 紫
	cyan    string // #06b6d4 青
	teal    string // #14b8a6 青绿
	white   string
	dim     string // 中灰
	muted   string // 灰
	subtle  string // 深灰
	lineDim string // 最暗灰（框线）
	reset   string
}{
	purple:  "\033[38;2;139;92;246m",
	violet:  "\033[38;2;168;85;247m",
	cyan:    "\033[38;2;6;182;212m",
	teal:    "\033[38;2;20;184;166m",
	white:   "\033[38;2;249;250;251m",
	dim:     "\033[38;2;75;85;99m",
	muted:   "\033[38;2;107;114;128m",
	subtle:  "\033[38;2;55;65;81m",
	lineDim: "\033[38;2;31;41;55m",
	reset:   "\033[0m",
}

// ─── 像素艺术 Logo：CODECAST（8 字母 × 每行 76 字符）────────
// 每个字母 8 字符宽 + 1 空格间隔，7 行主体 + 1 行底座
// 使用纯方块字符 █ 和 ░，确保所有终端 100% 兼容
var logoLines = []string{
	//  1         2         3         4         5         6         7
	// 123456789012345678901234567890123456789012345678901234567890123456789012345678
	"   ██████   ████████   ██████   ████████   ██████   ██████   ████████   ██████   ", // C O D E C A S T
	"  ████████ ███     ██ ████████ ███     ██ ████████ ████████ ████████   ████████  ",
	" ███░░░░██ ███   ████ ███░░███ ███        ███░░░░█ ███░░░░█ ███░████   ███░░░░█  ",
	" ███       ██████████ ███   ██ ████████   ███      ███████  ███   ██   ████████  ",
	" ███░█████ ███░░░░███ ███░░███ ███░░░░█   ███░████ ███░░░░█ ███░░████  ███░████  ",
	" ███   ███ ███    ███ ███  ███ ███   ███  ███   ██ ███   ██ ███    ██  ███  ███  ",
	" ████████  ███    ███ ████████ █████████  ████████ ████████ ███    ███ ████████  ",
	"  ██████   ███    ███  ██████   ████████   ██████   ██████  ███     ██  ██████   ",
}

// 每行颜色（紫→青渐变）
var logoLineColors = []string{
	palette.violet,
	palette.purple,
	palette.purple,
	palette.violet,
	palette.cyan,
	palette.cyan,
	palette.teal,
	palette.teal,
}

// ─── Splash 结构体 ─────────────────────────────────────────────
type Splash struct {
	version    string
	width      int
	height     int
	done       chan struct{}
	startOnce  sync.Once
	finishOnce sync.Once
	frameDelay time.Duration

	stars []star
	tag   string
}

type star struct {
	x, y       int
	brightness int
	char       rune
}

// ─── 公开 API ──────────────────────────────────────────────────

func DefaultSplash() *Splash {
	s := &Splash{
		version:    version.ShortVersion(),
		tag:        "AI-POWERED TERMINAL AGENT",
		done:       make(chan struct{}),
		frameDelay: 100 * time.Millisecond,
	}
	s.detectSize()
	s.initStars()
	return s
}

func (s *Splash) Run() {
	s.RunAsync()
	<-s.done
}

func (s *Splash) RunFor(frames int) {
	s.startOnce.Do(func() {
		go s.renderLoopN(frames)
	})
	<-s.done
}

// RunAsync 启动动画循环（只渲染一帧 logo + 星空静态）
func (s *Splash) RunAsync() {
	s.startOnce.Do(func() {
		// C-05 修复：Windows 平台启用虚拟终端处理（VTP）
		if runtime.GOOS == "windows" {
			enableWindowsVTP()
		}
		go s.renderLoop()
		go s.signalHandler()
	})
}

// enableWindowsVTP 在 Windows 上启用虚拟终端处理，使 ANSI 转义序列生效
func enableWindowsVTP() {
	stdout := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(stdout, &mode); err == nil {
		// 启用虚拟终端处理标志
		windows.SetConsoleMode(stdout, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}

func (s *Splash) Wait() {
	<-s.done
}

// Finish 优雅结束动画，清屏恢复光标
func (s *Splash) Finish() {
	s.finishOnce.Do(func() {
		close(s.done)
		// 使用硬重置序列 + 光标恢复，彻底清理
		fmt.Print("\033[?25h") // 光标恢复
		fmt.Print("\033[2J")   // 清屏
		fmt.Print("\033[H")    // 光标回左上角
	})
}

// ─── 内部实现 ──────────────────────────────────────────────────

func (s *Splash) detectSize() {
	w, h := 80, 28
	// C-03/C-04 修复：使用 golang.org/x/term 获取真实终端尺寸，支持所有平台
	if width, height, err := term.GetSize(int(os.Stdout.Fd())); err == nil {
		w, h = width, height
	}
	if w < 76 {
		w = 80
	}
	if h < 20 {
		h = 28
	}
	s.width = w
	s.height = h
}

// initStars 初始化星空（避开 logo 区域）
func (s *Splash) initStars() {
	logoTop := s.height/2 - 5
	logoBot := s.height/2 + 4
	s.stars = make([]star, 40)
	for i := range s.stars {
		x := rand.Intn(s.width-2) + 1
		y := rand.Intn(s.height-3) + 1
		// 如果落在 logo 区域，推到上方
		if y >= logoTop && y <= logoBot {
			y = rand.Intn(logoTop - 2)
			if y < 1 {
				y = 1
			}
		}
		// 星星字符：只用简单的 * · +，避免 Unicode 渲染问题
		chars := []rune{'*', '·', '+', '.', '*'}
		s.stars[i] = star{
			x:          x,
			y:          y,
			brightness: rand.Intn(3), // 0=暗 1=中 2=亮
			char:       chars[rand.Intn(len(chars))],
		}
	}
}

// renderLoop 动画主循环：Logo 渐入（前 12 帧），之后保持静态但星星偶尔闪烁
func (s *Splash) renderLoop() {
	frame := 0
	for {
		select {
		case <-s.done:
			return
		default:
		}

		s.render(frame)
		time.Sleep(s.frameDelay)
		frame++
	}
}

func (s *Splash) renderLoopN(frames int) {
	for i := 0; i < frames; i++ {
		s.render(i)
		time.Sleep(s.frameDelay)
	}
	s.Finish()
}

// render 绘制一帧
func (s *Splash) render(frame int) {
	// 清屏：用硬重置（比 \033[2J 在 Windows 更可靠）
	fmt.Print("\033[?25l") // 光标隐藏
	fmt.Print("\033[H")    // 光标回左上角
	// 用空格覆盖整屏（比 ANSI 清屏更可靠）
	for y := 1; y <= s.height; y++ {
		fmt.Printf("\033[%d;1H%s", y, strings.Repeat(" ", s.width))
	}

	logoHeight := len(logoLines)       // 8 行
	logoWidth := 0                      // 每行实际宽度
	for _, line := range logoLines {
		if len(line) > logoWidth {
			logoWidth = len(line)
		}
	}
	logoTop := s.height/2 - logoHeight/2 - 2
	if logoTop < 1 {
		logoTop = 1
	}
	logoStart := (s.width - logoWidth) / 2
	if logoStart < 0 {
		logoStart = 0
	}

	// ═══════ 星空（背景层，先画）═══════
	for _, st := range s.stars {
		// 根据 frame 做缓慢闪烁（每 6 帧改变一次）
		phase := (frame + st.y) % 8
		visible := true
		if st.brightness == 0 && phase > 2 {
			visible = false
		}
		if st.brightness == 1 && phase > 5 {
			visible = false
		}
		if !visible {
			continue
		}

		var color string
		switch st.brightness {
		case 0:
			color = palette.subtle
		case 1:
			color = palette.muted
		default:
			if phase < 3 {
				color = palette.white
			} else {
				color = palette.muted
			}
		}
		fmt.Printf("\033[%d;%dH%s%c", st.y, st.x, color, st.char)
	}

	// ═══════ Logo（前景层）═══════
	// Logo 渐入：前 12 帧从左到右逐字符显现
	for i, line := range logoLines {
		y := logoTop + i
		if y < 1 || y > s.height {
			continue
		}
		color := logoLineColors[i]

		if frame < 12 {
			// 渐入模式：按 alpha 决定显示多少
			alpha := float64(frame) / 12.0
			showChars := int(float64(len(line)) * alpha)
			if showChars < 0 {
				showChars = 0
			}
			// 前几帧用暗色，后几帧用主色
			if frame < 6 {
				color = palette.subtle
			}
			if showChars > len(line) {
				showChars = len(line)
			}
			fmt.Printf("\033[%d;%dH%s%s", y, logoStart+1, color, line[:showChars])
		} else {
			// 正常显示
			fmt.Printf("\033[%d;%dH%s%s", y, logoStart+1, color, line)
		}
	}

	// ═══════ 右上角：AgentPrimordia ═══════
	brand := "AgentPrimordia"
	brandX := s.width - len(brand) - 2
	if brandX < 1 {
		brandX = 1
	}
	fmt.Printf("\033[1;%dH%s%s", brandX, palette.subtle, brand)

	// ═══════ Tagline（Logo 下方两行）═══════
	tagY := logoTop + logoHeight + 2
	tagWidth := len(s.tag) + 4 // 加左右 padding
	tagStart := (s.width - tagWidth) / 2
	if tagStart < 1 {
		tagStart = 1
	}
	// 顶部装饰线
	fmt.Printf("\033[%d;%dH%s%s", tagY, tagStart, palette.lineDim,
		"┌"+strings.Repeat("─", tagWidth-2)+"┐")
	// 中间 tag
	fmt.Printf("\033[%d;%dH%s│%s %s%s│%s",
		tagY+1, tagStart, palette.lineDim, palette.cyan, s.tag, palette.lineDim, palette.reset)
	// 底部装饰线
	fmt.Printf("\033[%d;%dH%s%s", tagY+2, tagStart, palette.lineDim,
		"└"+strings.Repeat("─", tagWidth-2)+"┘")

	// ═══════ 提示行 ═══════
	hintY := tagY + 4
	if hintY > s.height-3 {
		hintY = s.height - 3
	}
	fmt.Printf("\033[%d;1H%s  按 Enter 开始，或输入 /help 查看命令", hintY, palette.dim)

	// ═══════ 状态栏 ═══════
	statusY := s.height - 1
	cwd, _ := os.Getwd()
	if len(cwd) > 30 {
		cwd = "..." + cwd[len(cwd)-27:]
	}
	fmt.Printf("\033[%d;1H%s%s", statusY, palette.subtle, cwd)

	verX := s.width - len(s.version) - 3
	if verX < 1 {
		verX = 1
	}
	fmt.Printf("\033[%d;%dH%s%s", statusY, verX, palette.muted, s.version)

	// 光标移到最下面第一列（避免闪烁）
	fmt.Printf("\033[%d;1H", s.height)
}

// signalHandler 监听 OS 信号
func (s *Splash) signalHandler() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigChan)
	select {
	case <-sigChan:
		s.Finish()
		os.Exit(1)
	case <-s.done:
	}
}

func getTerminalSize() (int, int, error) {
	return 80, 28, nil
}

// ─── 简化版 Banner（headless 模式）──────────────────────────

func PrintBanner(ver string) {
	fmt.Println()
	for i, line := range logoLines {
		pad := (80 - len(line)) / 2
		if pad < 0 {
			pad = 0
		}
		fmt.Print(strings.Repeat(" ", pad))
		fmt.Print(logoLineColors[i])
		fmt.Println(line)
	}
	fmt.Println(palette.reset)
	tag := "AI-POWERED TERMINAL AGENT"
	pad := (80 - len(tag)) / 2
	if pad < 0 {
		pad = 0
	}
	fmt.Print(strings.Repeat(" ", pad))
	fmt.Print(palette.cyan)
	fmt.Println(tag)
	fmt.Println(palette.reset)
	fmt.Printf("  Version:    %s\n", ver)
	fmt.Printf("  Framework:  AgentPrimordia\n")
	fmt.Println()
}

// 保留 math 引用以避免 unused warning（后续如果需要正弦动画）
var _ = math.Sin
