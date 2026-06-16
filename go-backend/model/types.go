package model

import "time"

// ── 请求体 ──

type ActionRequest struct {
	Action  string                 `json:"action"`
	Payload map[string]interface{} `json:"payload"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ChatRequest struct {
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
	Model    string    `json:"model"`
	Tools    []Tool    `json:"tools,omitempty"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ── 数据库模型 ──

type Vendor struct {
	ID      int    `json:"id"`
	Code    string `json:"code"`
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Enabled bool   `json:"enabled"`
}

type ModelEntry struct {
	ID          int    `json:"id"`
	VendorID    int    `json:"vendor_id"`
	ModelID     string `json:"model_id"`
	Name        string `json:"name"`
	ModelType   string `json:"model_type"`
	Enabled     bool   `json:"enabled"`
	MaxTokens   int    `json:"max_tokens"`
	DisplayName string `json:"display_name"`
}

type APIKey struct {
	ID      int    `json:"id"`
	VendorID int   `json:"vendor_id"`
	Name    string `json:"name"`
	APIKey  string `json:"api_key"`
	Enabled bool   `json:"enabled"`
}

type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Password  string    `json:"-"`
	Status    int       `json:"status"`
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Session struct {
	UserID   int       `json:"user_id"`
	Username string    `json:"username"`
	IsAdmin  bool      `json:"is_admin"`
	Expire   time.Time `json:"expire"`
}

type LLMStat struct {
	Ts              string `json:"ts"`
	UserID          int    `json:"user_id"`
	Username        string `json:"username"`
	VendorID        int    `json:"vendor_id"`
	ModelID         string `json:"model_id"`
	PromptTokens    int    `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	TotalTokens     int    `json:"total_tokens"`
	LatencyMs       int    `json:"latency_ms"`
	Success         bool   `json:"success"`
	Error           string `json:"error,omitempty"`
	ConversationID  string `json:"conversation_id,omitempty"`
}

type RouteInfo struct {
	VendorID     int    `json:"vendor_id"`
	VendorName   string `json:"vendor_name"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	KeyID        string `json:"key_id"`
	ModelID      string `json:"model_id"`
	QuotaExhausted bool  `json:"_quota_exhausted,omitempty"`
}

type Strategy struct {
	ID      string           `json:"id"`
	Name    string           `json:"name"`
	Type    string           `json:"type"` // fixed | round_robin
	Active  bool             `json:"active"`
	Options []StrategyOption `json:"options"`
}

type StrategyOption struct {
	VendorID    int    `json:"vendor_id"`
	KeyID       string `json:"key_id"`
	ModelID     string `json:"model_id"`
	VendorName  string `json:"vendor_name,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	KeyName     string `json:"key_name,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

type LLMLog struct {
	ID       int    `json:"id"`
	Ts       string `json:"ts"`
	Category string `json:"category"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	Detail   string `json:"detail"`
}

type AISuggestion struct {
	ID          int       `json:"id"`
	ReportDate  string    `json:"report_date"`
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Priority    string    `json:"priority"`
	Status      string    `json:"status"`
	CreatedAt   string    `json:"created_at"`
	ProcessedAt *string   `json:"processed_at"`
}

// ── God Rules ──

type GodRulesConfig struct {
	Enabled        bool   `json:"enabled"`
	Rules          string `json:"rules"`
	PromptOptimize bool   `json:"prompt_optimize"`
}

// ── Quota ──

type QuotaRecord struct {
	KeyID      string  `json:"key_id"`
	KeyName    string  `json:"key_name"`
	Pct        float64 `json:"pct"`
	LevelTier  string  `json:"level_tier"`
	Status     string  `json:"status"`
	NextReset  string  `json:"next_reset"`
	UpdatedAt  string  `json:"updated_at"`
	Error      *string `json:"error"`
}

// ── Skills ──

type Skill struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Description     string        `json:"description"`
	ExampleQueries  []string      `json:"example_queries"`
	Queries         []SkillQuery  `json:"queries"`
	PostInstruction string        `json:"post_instruction"`
}

type SkillQuery struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	SQL   string `json:"sql"`
}

// ── Chat History ──

type ChatMessage struct {
	ID        int    `json:"id"`
	CreatedAt string `json:"created_at"`
	Role      string `json:"role"`
	Content   string `json:"content"`
}
