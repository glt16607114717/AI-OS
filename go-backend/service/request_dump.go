package service

import (
	"encoding/json"
	"fmt"
	"log"
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
//
// 性能优化：请求时不再触发清理（避免高并发下的磁盘 IO 锁竞争 + nil panic）。
// 改为凌晨 4 点 cron 定时清理（见 CleanupDumpFiles），单日累积约 2.8G 完全可接受。
// 清理时仍保留 nil 防御，防止 stat 失败导致 panic。
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

	// 写入文件（不再调用 cleanupDumpFilesIn，改由 cron 凌晨清理）
	os.WriteFile(filepath, data, 0644)
}

// DumpRawRequest 将原始请求体（加工前）转储到文件系统，用于分析 Trae 注入的噪音
//
// 性能优化：同 DumpRequest，请求时不清理，由 cron 定时清理。
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
}

// CleanupDumpFiles 定时清理 dump 文件（cron 凌晨 4 点调用）
// 保留每个目录最近 keepPerDir 个文件，删多余的
func CleanupDumpFiles() {
	log.Printf("[cron] 开始清理 dump 文件...")
	kept1 := cleanupDumpFilesIn(DUMP_DIR, 500)
	kept2 := cleanupDumpFilesIn(RAW_DUMP_DIR, 500)
	log.Printf("[cron] dump 清理完成: requests 保留 %d 个，raw_requests 保留 %d 个", kept1, kept2)
}

// cleanupDumpFilesIn 清理指定目录的旧转储文件，只保留最近的 N 个。返回保留的文件数。
//
// 修复 panic：高并发下多个 goroutine 同时调 DumpRequest -> 同时 cleanup，
// 一个在 sort.Slice 里 os.Stat，另一个已 os.Remove 删了文件 -> os.Stat 返回 nil ->
// infoI.ModTime() 空指针 panic。整个进程卡死（Recoverer 接不住 sort 内部 panic）。
//
// 修复方案：stat 失败的文件按"最旧"处理（被排到末尾优先删除），避免 nil 解引用。
//
// 性能优化：现在请求时不再调用本函数，改为凌晨 4 点 cron 单线程调用，彻底避免并发问题。
func cleanupDumpFilesIn(dir string, keep int) int {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return 0
	}

	// 按修改时间排序（最新的在前）
	// 用 oldestTime 兜底：stat 失败的文件视为最旧，排到末尾（会被优先删除，正好清理掉）
	oldestTime := time.Unix(0, 0)
	sort.Slice(files, func(i, j int) bool {
		infoI, errI := os.Stat(files[i])
		infoJ, errJ := os.Stat(files[j])
		// stat 失败（文件已被并发删除）-> 兜底为最早时间，排末尾
		mtimeI, mtimeJ := oldestTime, oldestTime
		if errI == nil && infoI != nil {
			mtimeI = infoI.ModTime()
		}
		if errJ == nil && infoJ != nil {
			mtimeJ = infoJ.ModTime()
		}
		return mtimeI.After(mtimeJ)
	})

	// 删除超出保留数量的旧文件
	deleted := 0
	if len(files) > keep {
		for _, f := range files[keep:] {
			if err := os.Remove(f); err == nil {
				deleted++
			}
		}
	}
	if deleted > 0 {
		log.Printf("[dump] 清理 %s：删除 %d 个旧文件，保留 %d 个", dir, deleted, keep)
	}
	return len(files) - deleted
}

// GetDumpFiles 获取最近的转储文件列表
func GetDumpFiles(limit int) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(DUMP_DIR, "*.json"))
	if err != nil {
		return nil, err
	}

	// 修复 nil panic：stat 失败的文件兜底为最早时间（与 cleanupDumpFilesIn 同样的修复）
	oldestTime := time.Unix(0, 0)
	sort.Slice(files, func(i, j int) bool {
		infoI, errI := os.Stat(files[i])
		infoJ, errJ := os.Stat(files[j])
		mtimeI, mtimeJ := oldestTime, oldestTime
		if errI == nil && infoI != nil {
			mtimeI = infoI.ModTime()
		}
		if errJ == nil && infoJ != nil {
			mtimeJ = infoJ.ModTime()
		}
		return mtimeI.After(mtimeJ)
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