package service

import (
	"strings"
)

// ChunkConfig 分块配置
type ChunkConfig struct {
	MinSize int // 最小块大小（字符）
	MaxSize int // 最大块大小（字符）
	Overlap int // 重叠字符数
}

// ChunkByType 根据文件类型选择分块策略
func ChunkByType(text string, ext string, cfg ChunkConfig) []string {
	if cfg.MinSize <= 0 {
		cfg.MinSize = 200
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = 800
	}

	switch ext {
	case ".md", ".markdown":
		return chunkMarkdown(text, cfg)
	case ".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".rs":
		return chunkCode(text, cfg)
	case ".xlsx":
		return chunkTable(text, cfg)
	case ".pdf":
		return chunkPages(text, cfg)
	default:
		return chunkParagraphs(text, cfg)
	}
}

// ── Markdown 分块：按 ## 标题切分，保留章节上下文 ──

func chunkMarkdown(text string, cfg ChunkConfig) []string {
	sections := splitByMarkdownHeaders(text)

	var chunks []string
	var current strings.Builder
	var sectionHeader string // 当前所属章节标题

	for _, sec := range sections {
		sec = strings.TrimSpace(sec)
		if sec == "" {
			continue
		}

		// 提取本节标题行
		firstLineEnd := strings.Index(sec, "\n")
		firstLine := sec
		body := ""
		if firstLineEnd > 0 {
			firstLine = sec[:firstLineEnd]
			body = strings.TrimSpace(sec[firstLineEnd+1:])
		}

		// 如果是标题行，更新章节上下文
		if strings.HasPrefix(firstLine, "#") {
			sectionHeader = firstLine
			if body == "" {
				continue // 纯标题行，不单独成块
			}
		}

		// 构建带上下文的块内容
		content := body
		if sectionHeader != "" && !strings.HasPrefix(firstLine, "#") {
			content = sectionHeader + "\n\n" + body
		}

		if content == "" {
			continue
		}

		// 如果当前块 + 新内容超过最大长度，保存当前块
		if current.Len() > 0 && current.Len()+len(content) > cfg.MaxSize {
			if current.Len() >= cfg.MinSize {
				chunks = append(chunks, strings.TrimSpace(current.String()))
			}
			current.Reset()
		}

		if current.Len() == 0 {
			current.WriteString(content)
		} else {
			current.WriteString("\n\n" + content)
		}
	}

	// 最后一块
	if current.Len() >= cfg.MinSize {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	} else if len(chunks) > 0 {
		chunks[len(chunks)-1] += "\n\n" + current.String()
	}

	return chunks
}

// splitByMarkdownHeaders 按 ## / ### 标题分割
func splitByMarkdownHeaders(text string) []string {
	lines := strings.Split(text, "\n")
	var sections []string
	var current strings.Builder

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isH2 := strings.HasPrefix(trimmed, "##") && !strings.HasPrefix(trimmed, "###")
		isH3 := strings.HasPrefix(trimmed, "###") && !strings.HasPrefix(trimmed, "####")

		if isH2 || isH3 {
			if current.Len() > 0 {
				sections = append(sections, current.String())
			}
			current.Reset()
		}
		current.WriteString(line + "\n")
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}

// ── 代码分块：按空行 + 函数/类边界切分 ──

func chunkCode(text string, cfg ChunkConfig) []string {
	lines := strings.Split(text, "\n")
	var blocks []string
	var current strings.Builder
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 追踪代码块标记
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
		}

		// 非代码块内的空行 = 函数边界
		if !inCodeBlock && trimmed == "" && current.Len() >= cfg.MinSize {
			blocks = append(blocks, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}

		current.WriteString(line + "\n")
	}

	if current.Len() >= cfg.MinSize {
		blocks = append(blocks, strings.TrimSpace(current.String()))
	} else if len(blocks) > 0 {
		blocks[len(blocks)-1] += "\n" + current.String()
	}

	return blocks
}

// ── 表格分块：按行数切分 ──

func chunkTable(text string, cfg ChunkConfig) []string {
	lines := strings.Split(text, "\n")
	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		if current.Len()+len(line) > cfg.MaxSize && current.Len() >= cfg.MinSize {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
		current.WriteString(line + "\n")
	}
	if current.Len() >= cfg.MinSize {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	} else if len(chunks) > 0 {
		chunks[len(chunks)-1] += "\n" + current.String()
	}
	return chunks
}

// ── PDF 分块：按页切分 ──

func chunkPages(text string, cfg ChunkConfig) []string {
	pages := strings.Split(text, "\n\n\n")
	var chunks []string
	for _, page := range pages {
		page = strings.TrimSpace(page)
		if len(page) >= cfg.MinSize {
			chunks = append(chunks, page)
		}
	}
	return chunks
}

// ── 通用段落分块（DOCX / TXT 等）──

func chunkParagraphs(text string, cfg ChunkConfig) []string {
	paragraphs := strings.Split(text, "\n\n")
	var chunks []string
	var current strings.Builder

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		if current.Len() > 0 && current.Len()+len(p) > cfg.MaxSize {
			if current.Len() >= cfg.MinSize {
				chunks = append(chunks, strings.TrimSpace(current.String()))
			}
			current.Reset()
		}

		if current.Len() == 0 {
			current.WriteString(p)
		} else {
			current.WriteString("\n\n" + p)
		}
	}

	if current.Len() >= cfg.MinSize {
		chunks = append(chunks, strings.TrimSpace(current.String()))
	} else if len(chunks) > 0 {
		chunks[len(chunks)-1] += "\n\n" + current.String()
	}

	return chunks
}
