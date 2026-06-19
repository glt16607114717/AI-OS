package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/ledongthuc/pdf"
)

// ParseFile 根据文件扩展名解析文本内容
func ParseFile(filename string, reader io.Reader) (string, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	ext := strings.ToLower(filename)
	for i := len(ext) - 1; i >= 0; i-- {
		if ext[i] == '.' {
			ext = ext[i:]
			break
		}
	}

	switch ext {
	case ".txt", ".md", ".markdown", ".go", ".py", ".js", ".ts", ".vue",
		".java", ".c", ".cpp", ".h", ".rs", ".sql", ".yaml", ".yml",
		".json", ".xml", ".html", ".css", ".sh", ".bat", ".ini", ".cfg", ".toml":
		return string(data), nil

	case ".pdf":
		return parsePdf(data)

	case ".docx":
		return parseDocx(data)

	case ".xlsx":
		return parseXlsx(data)

	default:
		return "", fmt.Errorf("不支持的文件格式: %s", ext)
	}
}

// ── PDF 解析 ──

func parsePdf(data []byte) (string, error) {
	reader := bytes.NewReader(data)
	pdfReader, err := pdf.NewReader(reader, int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("解析 PDF 失败: %v", err)
	}

	var lines []string
	for i := 1; i <= pdfReader.NumPage(); i++ {
		page := pdfReader.Page(i)
		if page.V.IsNull() {
			continue
		}
		text, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text = strings.TrimSpace(text)
		if text != "" {
			lines = append(lines, text)
		}
	}
	if len(lines) == 0 {
		return "", fmt.Errorf("PDF 中未提取到文本内容")
	}
	return strings.Join(lines, "\n\n"), nil
}

// ── DOCX 解析 ──

type docxBody struct {
	Paragraphs []docxParagraph `xml:"p"`
}

type docxParagraph struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

func parseDocx(data []byte) (string, error) {
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("解析 DOCX 失败: %v", err)
	}

	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			xmlData, _ := io.ReadAll(rc)
			var body docxBody
			xml.Unmarshal(xmlData, &body)

			var lines []string
			for _, p := range body.Paragraphs {
				var line string
				for _, r := range p.Runs {
					line += r.Text
				}
				line = strings.TrimSpace(line)
				if line != "" {
					lines = append(lines, line)
				}
			}
			return strings.Join(lines, "\n"), nil
		}
	}
	return "", fmt.Errorf("DOCX 中未找到 word/document.xml")
}

// ── XLSX 解析（增强版：多 sheet + inlineStr + 合并单元格）──

// 共享字符串表
type xlsxSST struct {
	XMLName xml.Name      `xml:"sst"`
	Items   []xlsxSSTItem `xml:"si"`
}

type xlsxSSTItem struct {
	Text     string           `xml:"t"`
	RichText []xlsxSSTTextRun `xml:"r>t"`
}

type xlsxSSTTextRun struct {
	Text string `xml:",chardata"`
}

// 工作表
type xlsxWorksheet struct {
	SheetData xlsxSheetData `xml:"sheetData"`
}

type xlsxSheetData struct {
	Rows []xlsxRow `xml:"row"`
}

type xlsxRow struct {
	Cells []xlsxCell `xml:"c"`
}

type xlsxCell struct {
	Ref   string `xml:"r,attr"`
	Type  string `xml:"t,attr"`
	Value string `xml:"v"`
	// inlineStr
	InlineStr *xlsxInlineStr `xml:"is"`
}

type xlsxInlineStr struct {
	Text string `xml:"t"`
}

// 工作簿（获取 sheet 名称列表）
type xlsxWorkbook struct {
	Sheets []xlsxSheetRef `xml:"sheets>sheet"`
}

type xlsxSheetRef struct {
	Name    string `xml:"name,attr"`
	SheetID string `xml:"sheetId,attr"`
}

func parseXlsx(data []byte) (string, error) {
	zr, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("解析 XLSX 失败: %v", err)
	}

	// 读取共享字符串表
	sharedStrings := map[int]string{}
	for _, f := range zr.File {
		if f.Name == "xl/sharedStrings.xml" {
			rc, _ := f.Open()
			xmlData, _ := io.ReadAll(rc)
			rc.Close()
			var sst xlsxSST
			xml.Unmarshal(xmlData, &sst)
			for i, item := range sst.Items {
				if item.Text != "" {
					sharedStrings[i] = item.Text
				} else if len(item.RichText) > 0 {
					var rt strings.Builder
					for _, r := range item.RichText {
						rt.WriteString(r.Text)
					}
					sharedStrings[i] = rt.String()
				}
			}
		}
	}

	// 获取所有 sheet 名称
	sheetNames := map[string]string{} // sheetId -> name
	for _, f := range zr.File {
		if f.Name == "xl/workbook.xml" {
			rc, _ := f.Open()
			xmlData, _ := io.ReadAll(rc)
			rc.Close()
			var wb xlsxWorkbook
			xml.Unmarshal(xmlData, &wb)
			for _, s := range wb.Sheets {
				sheetNames[s.SheetID] = s.Name
			}
		}
	}

	// 读取所有工作表
	var allSheets []string
	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "xl/worksheets/sheet") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, _ := f.Open()
		xmlData, _ := io.ReadAll(rc)
		rc.Close()

		var ws xlsxWorksheet
		xml.Unmarshal(xmlData, &ws)

		var rows []string
		for _, row := range ws.SheetData.Rows {
			var cells []string
			for _, cell := range row.Cells {
				val := cell.Value

				// inlineStr
				if cell.Type == "inlineStr" && cell.InlineStr != nil {
					val = cell.InlineStr.Text
				} else if cell.Type == "s" {
					// 共享字符串引用
					idx := 0
					fmt.Sscanf(val, "%d", &idx)
					if s, ok := sharedStrings[idx]; ok {
						val = s
					}
				}
				cells = append(cells, val)
			}
			line := strings.Join(cells, "\t")
			if strings.TrimSpace(line) != "" {
				rows = append(rows, line)
			}
		}

		if len(rows) > 0 {
			// 提取 sheet 编号
			sheetNum := strings.TrimPrefix(f.Name, "xl/worksheets/sheet")
			sheetNum = strings.TrimSuffix(sheetNum, ".xml")
			sheetName := sheetNames[sheetNum]
			if sheetName == "" {
				sheetName = "Sheet" + sheetNum
			}
			allSheets = append(allSheets, fmt.Sprintf("=== %s ===\n%s", sheetName, strings.Join(rows, "\n")))
		}
	}

	if len(allSheets) == 0 {
		return "", fmt.Errorf("XLSX 中未找到有效工作表")
	}
	return strings.Join(allSheets, "\n\n"), nil
}
