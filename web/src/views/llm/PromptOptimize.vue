<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { API_BASE } from '../../api'

function authHeaders() {
  const token = localStorage.getItem('aios_token')
  return { 'Content-Type': 'application/json', 'Authorization': `Bearer ${token}` }
}

// 完整配置（保存时全部回传，避免覆盖上帝指令）
const fullConfig = ref({
  enabled: true,
  rules: '',
  prompt_optimize: true,
  strip_noise: true,
  compress_file: true,
  simplify_lang: true,
  compress_tool_result: true,
  compress_tools: true,
})

const stripNoise = ref(true)
const compressFile = ref(true)
const simplifyLang = ref(true)
const compressToolResult = ref(true)
const compressTools = ref(true)

const saving = ref(false)
const loading = ref(true)

async function loadConfig() {
  loading.value = true
  try {
    const response = await fetch(`${API_BASE}/api/god-rules`, { headers: authHeaders() })
    const result = await response.json()
    if (result.ok) {
      const d = result.data || {}
      fullConfig.value = {
        enabled: d.enabled ?? true,
        rules: d.rules ?? '',
        prompt_optimize: d.prompt_optimize ?? true,
        strip_noise: d.strip_noise ?? true,
        compress_file: d.compress_file ?? true,
        simplify_lang: d.simplify_lang ?? true,
        compress_tool_result: d.compress_tool_result ?? true,
        compress_tools: d.compress_tools ?? true,
      }
      stripNoise.value = fullConfig.value.strip_noise
      compressFile.value = fullConfig.value.compress_file
      simplifyLang.value = fullConfig.value.simplify_lang
      compressToolResult.value = fullConfig.value.compress_tool_result
      compressTools.value = fullConfig.value.compress_tools
    }
  } catch (e: any) {
    ElMessage.error('加载配置失败: ' + e.message)
  }
  loading.value = false
}

async function saveConfig() {
  saving.value = true
  try {
    const response = await fetch(`${API_BASE}/api/god-rules/save`, {
      method: 'POST',
      headers: authHeaders(),
      body: JSON.stringify({
        ...fullConfig.value,
        strip_noise: stripNoise.value,
        compress_file: compressFile.value,
        simplify_lang: simplifyLang.value,
        compress_tool_result: compressToolResult.value,
        compress_tools: compressTools.value,
      }),
    })
    const result = await response.json()
    if (result.ok) {
      ElMessage.success('保存成功')
      fullConfig.value.strip_noise = stripNoise.value
      fullConfig.value.compress_file = compressFile.value
      fullConfig.value.simplify_lang = simplifyLang.value
      fullConfig.value.compress_tool_result = compressToolResult.value
      fullConfig.value.compress_tools = compressTools.value
    } else {
      ElMessage.error(result.error || '保存失败')
    }
  } catch (e: any) {
    ElMessage.error(e.message || '保存失败')
  }
  saving.value = false
}

onMounted(loadConfig)
</script>

<template>
  <div class="prompt-opt-page">
    <div class="page-header">
      <h2 class="page-title">提示词优化</h2>
      <p class="page-desc">清理发往大模型的冗余信息，省 Token、提高遵从度。仅影响发送给大模型的内容，不改动原始对话记录。</p>
    </div>

    <div v-if="loading" class="empty-state">
      <p class="empty-text">加载中...</p>
    </div>

    <div v-else class="opt-cards">
      <!-- 清理历史噪音 -->
      <div class="opt-card" :class="{ off: !stripNoise }">
        <div class="opt-card-header">
          <div class="opt-icon">1</div>
          <div class="opt-info">
            <div class="opt-name">清理历史噪音</div>
            <div class="opt-desc">删掉对话历史里每轮都重复出现的「终端状态」「语言设置」等无用信息，只保留你的真实提问</div>
          </div>
          <label class="toggle-label">
            <input type="checkbox" v-model="stripNoise" class="toggle-check" />
            <span class="toggle-switch"></span>
          </label>
        </div>
        <div class="opt-effect">
          <span class="effect-tag" :class="stripNoise ? 'on' : 'off'">
            {{ stripNoise ? '预计每轮省 ~600 字符' : '未启用' }}
          </span>
        </div>
      </div>

      <!-- 压缩文件信息 -->
      <div class="opt-card" :class="{ off: !compressFile }">
        <div class="opt-card-header">
          <div class="opt-icon">2</div>
          <div class="opt-info">
            <div class="opt-name">压缩文件信息</div>
            <div class="opt-desc">把「用户打开了 xxx 文件」这一大段 XML 标签，压缩成一行 [当前文件: xxx.go]，拼到你的提问后面</div>
          </div>
          <label class="toggle-label">
            <input type="checkbox" v-model="compressFile" class="toggle-check" />
            <span class="toggle-switch"></span>
          </label>
        </div>
        <div class="opt-effect">
          <span class="effect-tag" :class="compressFile ? 'on' : 'off'">
            {{ compressFile ? '压缩率约 90%' : '未启用' }}
          </span>
        </div>
      </div>

      <!-- 语言要求精简 -->
      <div class="opt-card" :class="{ off: !simplifyLang }">
        <div class="opt-card-header">
          <div class="opt-icon">3</div>
          <div class="opt-info">
            <div class="opt-name">语言要求精简</div>
            <div class="opt-desc">删掉 IDE 自带的英文语言要求（跟上帝指令重复了），语言规则以你上帝指令里的为准</div>
          </div>
          <label class="toggle-label">
            <input type="checkbox" v-model="simplifyLang" class="toggle-check" />
            <span class="toggle-switch"></span>
          </label>
        </div>
        <div class="opt-effect">
          <span class="effect-tag" :class="simplifyLang ? 'on' : 'off'">
            {{ simplifyLang ? '去重 system + user 语言块' : '未启用' }}
          </span>
        </div>
      </div>

      <!-- 压缩历史工具结果 -->
      <div class="opt-card" :class="{ off: !compressToolResult }">
        <div class="opt-card-header">
          <div class="opt-icon">4</div>
          <div class="opt-info">
            <div class="opt-name">压缩历史工具结果</div>
            <div class="opt-desc">5 轮之前的工具返回结果（如文件读取、搜索结果）超过 500 字符时截断，并提示 AI 可重新执行。最新 5 轮完全保留</div>
          </div>
          <label class="toggle-label">
            <input type="checkbox" v-model="compressToolResult" class="toggle-check" />
            <span class="toggle-switch"></span>
          </label>
        </div>
        <div class="opt-effect">
          <span class="effect-tag" :class="compressToolResult ? 'on' : 'off'">
            {{ compressToolResult ? '长对话省 50%+ tokens' : '未启用' }}
          </span>
        </div>
      </div>

      <!-- 精简工具定义 -->
      <div class="opt-card" :class="{ off: !compressTools }">
        <div class="opt-card-header">
          <div class="opt-icon">5</div>
          <div class="opt-info">
            <div class="opt-name">精简工具定义</div>
            <div class="opt-desc">删除 RunCommand 的 git 提交/推送教程和 Task 的示例代码块。安全红线（禁止 force push 等）完整保留</div>
          </div>
          <label class="toggle-label">
            <input type="checkbox" v-model="compressTools" class="toggle-check" />
            <span class="toggle-switch"></span>
          </label>
        </div>
        <div class="opt-effect">
          <span class="effect-tag" :class="compressTools ? 'on' : 'off'">
            {{ compressTools ? '每轮固定省 ~1000 token' : '未启用' }}
          </span>
        </div>
      </div>

      <!-- 保存按钮 -->
      <div class="actions-row">
        <button class="btn-save" @click="saveConfig" :disabled="saving">
          {{ saving ? '保存中...' : '保存' }}
        </button>
      </div>

      <!-- 说明 -->
      <div class="tips-card">
        <div class="tips-title">工作原理</div>
        <ul class="tips-list">
          <li>三项优化均在<strong>发送给大模型之前</strong>执行，不改动原始对话记录</li>
          <li>「清理历史噪音」只处理历史轮消息，<strong>最新一轮的终端状态会保留</strong>（大模型需要知道当前环境）</li>
          <li>配置按用户隔离，每个用户独立设置自己的优化策略</li>
          <li>默认全部开启，无需手动配置即可生效</li>
        </ul>
      </div>
    </div>
  </div>
</template>

<style scoped>
.prompt-opt-page {
  padding: 24px 24px 60px;
  box-sizing: border-box;
  max-width: 800px;
}
.page-header { margin-bottom: 24px; }
.page-title {
  font-size: 20px; font-weight: 600; color: #303133; margin: 0 0 8px 0;
}
.page-desc {
  font-size: 13px; color: #909399; margin: 0; line-height: 1.6;
}
.empty-state { text-align: center; padding: 60px 0; color: #c0c4cc; }

.opt-cards { display: flex; flex-direction: column; gap: 16px; }

.opt-card {
  border: 1px solid #e4e7ed; border-radius: 10px;
  background: #fff; padding: 20px;
  transition: border-color 0.2s, opacity 0.2s;
}
.opt-card.off { opacity: 0.6; }
.opt-card-header {
  display: flex; align-items: flex-start; gap: 14px;
}
.opt-icon {
  flex-shrink: 0; width: 28px; height: 28px; border-radius: 50%;
  background: #409eff; color: #fff; font-size: 14px; font-weight: 600;
  display: flex; align-items: center; justify-content: center;
}
.opt-card.off .opt-icon { background: #c0c4cc; }
.opt-info { flex: 1; }
.opt-name {
  font-size: 15px; font-weight: 600; color: #303133; margin-bottom: 4px;
}
.opt-desc {
  font-size: 13px; color: #909399; line-height: 1.6;
}

.toggle-label {
  display: inline-flex; align-items: center; cursor: pointer; flex-shrink: 0;
}
.toggle-check { display: none; }
.toggle-switch {
  width: 40px; height: 22px; border-radius: 11px;
  background: #dcdfe6; position: relative; transition: background 0.3s;
}
.toggle-switch::after {
  content: ''; position: absolute; top: 2px; left: 2px;
  width: 18px; height: 18px; border-radius: 50%;
  background: #fff; transition: transform 0.3s;
  box-shadow: 0 1px 3px rgba(0,0,0,0.2);
}
.toggle-check:checked + .toggle-switch { background: #67c23a; }
.toggle-check:checked + .toggle-switch::after { transform: translateX(18px); }

.opt-effect { margin-top: 12px; padding-left: 42px; }
.effect-tag {
  display: inline-block; padding: 2px 10px; border-radius: 4px;
  font-size: 12px; font-weight: 500;
}
.effect-tag.on {
  background: #f0f9eb; color: #67c23a; border: 1px solid #c2e7b0;
}
.effect-tag.off {
  background: #f4f4f5; color: #c0c4cc; border: 1px solid #e4e7ed;
}

.actions-row { margin-top: 8px; }
.btn-save {
  padding: 10px 32px; font-size: 14px; font-weight: 500;
  color: #fff; background: #409eff; border: none;
  border-radius: 6px; cursor: pointer; transition: background 0.2s;
}
.btn-save:hover { background: #66b1ff; }
.btn-save:disabled { background: #a0cfff; cursor: not-allowed; }

.tips-card {
  margin-top: 8px; padding: 16px 20px; background: #f4f4f5; border-radius: 8px;
}
.tips-title {
  font-size: 13px; font-weight: 600; color: #606266; margin-bottom: 8px;
}
.tips-list {
  margin: 0; padding-left: 18px; font-size: 12px; color: #909399; line-height: 2;
}
.tips-list strong { color: #606266; }
</style>
