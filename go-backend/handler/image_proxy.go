package handler

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ImageDownload 图片下载代理
//
// 解决问题：智谱生图返回的 URL（ufileos.com 对象存储）默认不带 CORS 头，
// 浏览器 fetch 跨域下载会被拦截。本接口在服务端 fetch 后透传给浏览器，
// 绕开 CORS，并强制 attachment 下载。
//
// 路由：GET /api/image/download?url=<智谱图片URL>（登录态）
// 安全：只允许智谱相关域名，防 SSRF
func ImageDownload(w http.ResponseWriter, r *http.Request) {
	imageURL := r.URL.Query().Get("url")
	if imageURL == "" {
		errResponse(w, "缺少 url 参数", 400)
		return
	}

	// 安全：解析 URL，只允许智谱相关域名（防 SSRF）
	parsed, err := url.Parse(imageURL)
	if err != nil {
		errResponse(w, "无效的 URL", 400)
		return
	}
	host := strings.ToLower(parsed.Hostname())
	if !isAllowedImageHost(host) {
		errResponse(w, "不允许的图片域名: "+host, 403)
		return
	}

	// 服务端 fetch 图片（30 秒超时，下载图片通常很快）
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		errResponse(w, "构造请求失败: "+err.Error(), 500)
		return
	}
	req.Header.Set("User-Agent", "AI-OS/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[image_download] fetch 失败 host=%s err=%v", host, err)
		errResponse(w, "下载图片失败: "+err.Error(), 502)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errResponse(w, fmt.Sprintf("源站返回 HTTP %d", resp.StatusCode), resp.StatusCode)
		return
	}

	// 透传 Content-Type（image/png 或 image/jpeg）
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	// 强制浏览器下载，文件名带时间戳
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=aios_image_%d%s", time.Now().Unix(), extFromContentType(contentType)))

	// 流式透传 body
	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("[image_download] 透传失败 host=%s err=%v", host, err)
	}
}

// isAllowedImageHost 判断是否为允许的图片域名（防 SSRF）
// 智谱 GLM-Image 返回的图片托管在 ufileos.com（UCloud 对象存储）
func isAllowedImageHost(host string) bool {
	allowedSuffixes := []string{
		"ufileos.com",      // 智谱 GLM-Image 图片存储
		"bigmodel.cn",      // 智谱自有域名
		"zhipuai.cn",       // 智谱备用域名
	}
	for _, suffix := range allowedSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// extFromContentType 根据 Content-Type 推断文件扩展名
func extFromContentType(ct string) string {
	switch {
	case strings.Contains(ct, "jpeg"), strings.Contains(ct, "jpg"):
		return ".jpg"
	case strings.Contains(ct, "png"):
		return ".png"
	case strings.Contains(ct, "webp"):
		return ".webp"
	default:
		return ".png"
	}
}
