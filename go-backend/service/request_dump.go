package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	DUMP_DIR     = "logs/requests"
	RAW_DUMP_DIR = "logs/raw_requests"
)

// DumpRequest 将请求体转储到文件系统
func DumpRequest(body map[string]interface{}) {
	// 确保目录存在
	os.MkdirAll(DUMP_DIR, 0755)

	// 生成带时间戳的文件名
	timestamp := time.Now().Format("20060102_150405")
	random := randomID(4)
	filename := fmt.Sprintf("%s_%s.json", timestamp, random)
	filepath := filepath.Join(DUMP_DIR, filename)

	// 序列化请求体
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return
	}

	// 写入文件
	os.WriteFile(filepath, data, 0644)

	// 清理旧文件（只保留最近20个）
	cleanupDumpFilesIn(DUMP_DIR, 20)
}

// DumpRawRequest 将原始请求体（加工前）转储到文件系统，用于分析 Trae 注入的噪音
func DumpRawRequest(body map[string]interface{}) {
	os.MkdirAll(RAW_DUMP_DIR, 0755)
	timestamp := time.Now().Format("20060102_150405")
	random := randomID(4)
	filename := fmt.Sprintf("%s_%s.json", timestamp, random)
	filepath := filepath.Join(RAW_DUMP_DIR, filename)
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(filepath, data, 0644)
	cleanupDumpFilesIn(RAW_DUMP_DIR, 20)
}

// cleanupDumpFilesIn 清理指定目录的旧转储文件，只保留最近的 N 个
func cleanupDumpFilesIn(dir string, keep int) {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return
	}

	// 按修改时间排序（最新的在前）
	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		return infoI.ModTime().After(infoJ.ModTime())
	})

	// 删除超出保留数量的旧文件
	if len(files) > keep {
		for _, f := range files[keep:] {
			os.Remove(f)
		}
	}
}

// GetDumpFiles 获取最近的转储文件列表
func GetDumpFiles(limit int) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(DUMP_DIR, "*.json"))
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		infoI, _ := os.Stat(files[i])
		infoJ, _ := os.Stat(files[j])
		return infoI.ModTime().After(infoJ.ModTime())
	})

	if len(files) > limit {
		files = files[:limit]
	}

	return files, nil
}

// ReadDumpFile 读取转储文件内容
func ReadDumpFile(filename string) ([]byte, error) {
	filepath := filepath.Join(DUMP_DIR, filename)
	return os.ReadFile(filepath)
}