<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()

const username = ref('')
const password = ref('')
const rememberMe = ref(true)
const loading = ref(false)
const errorMsg = ref('')

import { API_BASE } from '../api'

onMounted(() => {
  const savedUsername = localStorage.getItem('aios_saved_username')
  const savedPassword = localStorage.getItem('aios_saved_password')
  if (savedUsername) {
    username.value = savedUsername
    password.value = savedPassword || ''
  }
})

async function handleLogin() {
  errorMsg.value = ''

  if (!username.value.trim()) {
    errorMsg.value = '请输入用户名'
    return
  }
  if (!password.value) {
    errorMsg.value = '请输入密码'
    return
  }

  loading.value = true
  try {
    const res = await fetch(`${API_BASE}/api/system/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: username.value.trim(), password: password.value }),
    })
    const data = await res.json()

    if (data.ok && data.data) {
      const { token, username: name, is_admin } = data.data
      localStorage.setItem('aios_token', token)
      localStorage.setItem('aios_username', name)
      localStorage.setItem('aios_is_admin', String(is_admin))

      if (rememberMe.value) {
        localStorage.setItem('aios_saved_username', username.value.trim())
        localStorage.setItem('aios_saved_password', password.value)
      } else {
        localStorage.removeItem('aios_saved_username')
        localStorage.removeItem('aios_saved_password')
      }

      router.replace('/voice')
    } else {
      errorMsg.value = data.msg || data.error || '登录失败，请检查用户名和密码'
    }
  } catch (e: any) {
    errorMsg.value = '网络错误，无法连接服务器'
  } finally {
    loading.value = false
  }
}

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !loading.value) {
    handleLogin()
  }
}
</script>

<template>
  <div class="login-page" @keydown="onKeydown">
    <div class="login-card">
      <div class="login-header">
        <div class="login-logo">
          <svg width="28" height="28" viewBox="0 0 24 24" fill="none">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z" fill="url(#login-bolt)" />
            <defs>
              <linearGradient id="login-bolt" x1="3" y1="2" x2="21" y2="22" gradientUnits="userSpaceOnUse">
                <stop offset="0%" stop-color="#06b6d4" />
                <stop offset="100%" stop-color="#a78bfa" />
              </linearGradient>
            </defs>
          </svg>
        </div>
        <h1 class="login-title">AI-OS</h1>
        <p class="login-subtitle">智能操作系统</p>
      </div>

      <div class="login-form">
        <div class="form-group">
          <label class="form-label">用户名</label>
          <input
            v-model="username"
            type="text"
            class="form-input"
            placeholder="请输入用户名"
            autocomplete="username"
            :disabled="loading"
          />
        </div>

        <div class="form-group">
          <label class="form-label">密码</label>
          <input
            v-model="password"
            type="password"
            class="form-input"
            placeholder="请输入密码"
            autocomplete="current-password"
            :disabled="loading"
          />
        </div>

        <label class="remember-row">
          <input v-model="rememberMe" type="checkbox" class="remember-checkbox" />
          <span class="remember-text">记住密码</span>
        </label>

        <div v-if="errorMsg" class="error-msg">{{ errorMsg }}</div>

        <button class="login-btn" :disabled="loading" @click="handleLogin">
          <span v-if="loading" class="btn-loader"></span>
          <span v-else>登 录</span>
        </button>
      </div>
    </div>

    <div class="login-footer">AI-OS v1.0.0</div>
  </div>
</template>

<style scoped>
.login-page {
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  background: #0f172a;
  position: relative;
  overflow: hidden;
}

/* Ambient glow */
.login-page::before {
  content: '';
  position: absolute;
  top: -120px;
  right: -80px;
  width: 360px;
  height: 360px;
  background: radial-gradient(circle, rgba(6, 182, 212, 0.08) 0%, transparent 70%);
  pointer-events: none;
}

.login-page::after {
  content: '';
  position: absolute;
  bottom: -100px;
  left: -60px;
  width: 300px;
  height: 300px;
  background: radial-gradient(circle, rgba(139, 92, 246, 0.07) 0%, transparent 70%);
  pointer-events: none;
}

.login-card {
  width: 380px;
  padding: 40px 36px 32px;
  border-radius: 16px;
  background: rgba(30, 41, 59, 0.65);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.03) inset;
  position: relative;
  z-index: 1;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-logo {
  width: 52px;
  height: 52px;
  border-radius: 14px;
  background: linear-gradient(135deg, rgba(6, 182, 212, 0.15), rgba(139, 92, 246, 0.15));
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 16px;
}

.login-title {
  font-size: 22px;
  font-weight: 700;
  color: #e2e8f0;
  letter-spacing: 4px;
  margin-bottom: 6px;
}

.login-subtitle {
  font-size: 13px;
  color: #64748b;
  letter-spacing: 1px;
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.form-group {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.form-label {
  font-size: 12px;
  font-weight: 500;
  color: #94a3b8;
  letter-spacing: 0.5px;
}

.form-input {
  height: 42px;
  padding: 0 14px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(15, 23, 42, 0.6);
  color: #e2e8f0;
  font-size: 14px;
  outline: none;
  transition: border-color 0.2s, box-shadow 0.2s;
}

.form-input::placeholder {
  color: #475569;
}

.form-input:focus {
  border-color: rgba(99, 102, 241, 0.5);
  box-shadow: 0 0 0 3px rgba(99, 102, 241, 0.1);
}

.form-input:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.remember-row {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  user-select: none;
}

.remember-checkbox {
  width: 15px;
  height: 15px;
  border-radius: 4px;
  accent-color: #6366f1;
  cursor: pointer;
}

.remember-text {
  font-size: 13px;
  color: #94a3b8;
}

.error-msg {
  font-size: 13px;
  color: #f87171;
  padding: 8px 12px;
  border-radius: 6px;
  background: rgba(248, 113, 113, 0.08);
  border: 1px solid rgba(248, 113, 113, 0.15);
}

.login-btn {
  height: 44px;
  border-radius: 8px;
  border: none;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 2px;
  cursor: pointer;
  transition: opacity 0.2s, transform 0.15s;
  display: flex;
  align-items: center;
  justify-content: center;
}

.login-btn:hover:not(:disabled) {
  opacity: 0.92;
  transform: translateY(-1px);
}

.login-btn:active:not(:disabled) {
  transform: translateY(0);
}

.login-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.btn-loader {
  width: 18px;
  height: 18px;
  border: 2px solid rgba(255, 255, 255, 0.3);
  border-top-color: #fff;
  border-radius: 50%;
  animation: login-spin 0.7s linear infinite;
}

@keyframes login-spin {
  to { transform: rotate(360deg); }
}

.login-footer {
  position: absolute;
  bottom: 24px;
  font-size: 11px;
  color: #334155;
  letter-spacing: 1px;
}
</style>
