package service

import (
	"regexp"
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

		// 单个 section 超长时，按段落二次拆分
		if len(content) > cfg.MaxSize {
			// 先保存当前累积的块
			if current.Len() >= cfg.MinSize {
				chunks = append(chunks, strings.TrimSpace(current.String()))
				current.Reset()
			}
			// 按 \n\n 段落拆分超长 section
			for _, sub := range splitLongContent(content, cfg.MaxSize) {
				if len(strings.TrimSpace(sub)) >= 50 {
					chunks = append(chunks, strings.TrimSpace(sub))
				}
			}
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

	// 过滤纯代码块/Mermaid 碎片：文本占比 < 30% 的块无检索价值
	chunks = filterCodeOnlyChunks(chunks)

	return chunks
}
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

// splitLongContent 将超长文本按段落（\n\n）拆分，超过 maxSize 的段落硬切
func splitLongContent(text string, maxSize int) []string {
	paragraphs := strings.Split(text, "\n\n")
	var result []string
	var current strings.Builder

	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		// 单个段落本身就超长 → 硬切
		if len(p) > maxSize {
			// 先保存当前累积的内容
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
			// 按 maxSize 硬切
			for i := 0; i < len(p); i += maxSize {
				end := i + maxSize
				if end > len(p) {
					end = len(p)
				}
				result = append(result, p[i:end])
			}
			continue
		}

		// 累积到超过 maxSize 就保存
		if current.Len()+len(p)+2 > maxSize {
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		}

		if current.Len() == 0 {
			current.WriteString(p)
		} else {
			current.WriteString("\n\n" + p)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
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

// filterCodeOnlyChunks 过滤纯代码块/Mermaid 碎片
// 逻辑：移除 ```...``` 包裹的纯代码内容（剔除代码后剩余文本 < 30%），防止无上下文的流程图残片入库
// 对纯 Markdown 代码块不影响（没有 ``` 包裹的段落不受影响）
func filterCodeOnlyChunks(chunks []string) []string {
	// 匹配三反引号代码块（含 mermaid/flowchart/graph/sequenceDiagram 等）
	codeBlockRe := regexp.MustCompile("(?s)```[a-zA-Z]*\\n.*?```")

	var filtered []string
	for _, chunk := range chunks {
		// 剔除所有代码块后的纯文本
		textOnly := codeBlockRe.ReplaceAllString(chunk, "")
		textOnly = strings.TrimSpace(textOnly)

		// 去除标题行（# 开头）后计算纯文本
		textLines := []string{}
		for _, line := range strings.Split(textOnly, "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				textLines = append(textLines, line)
			}
		}
		pureText := strings.Join(textLines, " ")

		// 纯文本占原 chunk 比例 < 30% → 丢弃（碎片化的纯代码/流程图无检索价值）
		if len(chunk) > 0 && float64(len(pureText))/float64(len(chunk)) < 0.3 {
			continue
		}
		filtered = append(filtered, chunk)
	}

	// 兜底：如果全部被过滤了，保留原始 chunks（防止极端情况丢全部数据）
	if len(filtered) == 0 && len(chunks) > 0 {
		return chunks
	}

	return filtered
}
