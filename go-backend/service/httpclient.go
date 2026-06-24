package service

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// SharedHTTPClient 全局共享 HTTP Client（连接池复用 TCP/TLS）
// 解决每次请求新建 Client 导致的：
//   - 无法复用 TCP 连接（每次重新握手 ~200ms）
//   - TLS 握手开销重复
//   - 大量 TIME_WAIT 套接字堆积
var SharedHTTPClient = &http.Client{
	Timeout: 120 * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,  // TCP 连接超时
			KeepAlive: 30 * time.Second,  // TCP keepalive 探测间隔
		}).DialContext,
		TLSClientConfig:       &tls.Config{},
		MaxIdleConns:          100,              // 全局最大空闲连接
		MaxIdleConnsPerHost:   20,               // 每个 host 最大空闲连接（智谱）
		IdleConnTimeout:       90 * time.Second, // 空闲连接超时
		ResponseHeaderTimeout: 90 * time.Second, // 等待响应头超时（大 prompt 场景首 token 可能较慢）
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true, // 尝试 HTTP/2（智谱支持）
	},
}

// SharedHTTPClientLong 超时版（用于蒸馏等长任务，晚上自动跑，允许较长时间）
var SharedHTTPClientLong = &http.Client{
	Timeout: 10 * time.Minute, // 蒸馏任务可能很长，给 10 分钟
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{},
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 10 * time.Minute,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}
