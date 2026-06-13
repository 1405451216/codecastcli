package vision

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	ap "agentprimordia/pkg"
	"codecast/cli/internal/config"
	"codecast/cli/internal/provider"
)

// Analyzer 图片分析器
type Analyzer struct {
	config   *config.Config
	provider ap.Provider
}

// NewAnalyzer 创建图片分析器
func NewAnalyzer(cfg *config.Config) (*Analyzer, error) {
	llmProvider, err := provider.CreateProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Provider 失败: %w", err)
	}

	return &Analyzer{
		config:   cfg,
		provider: llmProvider,
	}, nil
}

// AnalyzeImage 分析图片文件
func (a *Analyzer) AnalyzeImage(ctx context.Context, imagePath string, prompt string) (string, error) {
	// 读取图片文件
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("读取图片失败: %w", err)
	}

	// 检测 MIME 类型
	mime := detectMIME(imagePath)
	if mime == "" {
		return "", fmt.Errorf("不支持的图片格式: %s", filepath.Ext(imagePath))
	}

	// 编码为 base64
	b64Data := base64.StdEncoding.EncodeToString(data)

	// 尝试使用多模态适配器
	mmAdapter, err := ap.NewMultimodalAdapter(a.provider)
	if err != nil {
		// 回退到文本描述模式
		return a.analyzeWithTextFallback(ctx, imagePath, prompt)
	}

	// 构建多模态消息
	textPart := ap.NewTextContent(prompt)
	imagePart := ap.NewImageB64Content(b64Data, mime)

	req := &ap.CompletionRequestExt{
		Messages: []*ap.ChatMessageExt{
			ap.NewUserMultimodalMessage(textPart, imagePart),
		},
	}

	resp, err := mmAdapter.CompleteMultimodal(ctx, req)
	if err != nil {
		return "", fmt.Errorf("多模态分析失败: %w", err)
	}

	return resp.Content, nil
}

// AnalyzeImageURL 分析网络图片
func (a *Analyzer) AnalyzeImageURL(ctx context.Context, imageURL string, prompt string) (string, error) {
	mmAdapter, err := ap.NewMultimodalAdapter(a.provider)
	if err != nil {
		return "", fmt.Errorf("当前 Provider 不支持多模态: %w", err)
	}

	textPart := ap.NewTextContent(prompt)
	imagePart := ap.NewImageURLContent(imageURL)

	req := &ap.CompletionRequestExt{
		Messages: []*ap.ChatMessageExt{
			ap.NewUserMultimodalMessage(textPart, imagePart),
		},
	}

	resp, err := mmAdapter.CompleteMultimodal(ctx, req)
	if err != nil {
		return "", fmt.Errorf("图片分析失败: %w", err)
	}

	return resp.Content, nil
}

// analyzeWithTextFallback 文本回退模式（当 Provider 不支持多模态时）
func (a *Analyzer) analyzeWithTextFallback(_ context.Context, imagePath string, prompt string) (string, error) {
	return fmt.Sprintf("当前 Provider 不支持多模态分析。图片路径: %s\n提示: %s", imagePath, prompt), nil
}

// detectMIME 检测图片 MIME 类型
func detectMIME(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// IsImageFile 判断文件是否为支持的图片格式
func IsImageFile(path string) bool {
	return detectMIME(path) != ""
}
