package service

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// SharedHTTPClient 全局共享 HTTP Client（连接池复用 TCP/TLS）
// 流式请求：ResponseHeaderTimeout=30s 保证首字节，不设总超时（长回答可能很久）
// 非流式请求：由调用方通过 context.WithTimeout 控制总超时
var SharedHTTPClient = &http.Client{
	Timeout: 0, // 不设总超时，流式与非流式由各自场景控制
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,  // TCP 连接超时
			KeepAlive: 30 * time.Second,  // TCP keepalive 探测间隔
		}).DialContext,
		TLSClientConfig:       &tls.Config{},
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second, // 首字节超时：60秒
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}

// SharedHTTPClientLong 蒸馏专用（定时任务，晚上跑，不设超时）
var SharedHTTPClientLong = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{},
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 0,
		ExpectContinueTimeout: 1 * time.Second,
		ForceAttemptHTTP2:     true,
	},
}
