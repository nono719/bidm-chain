<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
import {
  UserOutlined,
  LockOutlined,
  SafetyCertificateOutlined,
  ApartmentOutlined,
  AuditOutlined,
  BlockOutlined
} from '@ant-design/icons-vue'
import { apiRequest, setToken, setUser } from '../api/client'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const form = reactive({
  username: 'admin',
  password: '123456'
})

async function doLogin() {
  loading.value = true
  try {
    const res = await apiRequest('/api/auth/login', { method: 'POST', body: form })
    if (res.code !== 0) {
      antdMessage.error(res.message || '登录失败')
      return
    }
    setToken(res.data.token)
    setUser(res.data.user)
    antdMessage.success('登录成功')
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    router.replace(redirect || '/')
  } finally {
    loading.value = false
  }
}

function fillDemo() {
  form.username = 'admin'
  form.password = '123456'
}
</script>

<template>
  <div class="login-page">
    <!-- Decorative ambient blobs -->
    <div class="bg-blob blob-1"></div>
    <div class="bg-blob blob-2"></div>

    <div class="login-shell">
      <!-- Left: brand panel -->
      <div class="brand-panel">
        <div class="brand-top">
          <div class="brand-logo">
            <BlockOutlined />
          </div>
          <div class="brand-name">BIDM-Chain</div>
          <div class="brand-tagline">基于 Hyperledger Fabric 的物联网身份管理与跨域认证系统</div>
        </div>

        <div class="brand-features">
          <div class="feature-item">
            <SafetyCertificateOutlined class="feature-icon" />
            <div>
              <div class="feature-title">DID 设备身份</div>
              <div class="feature-desc">每台 IoT 设备唯一可验证身份</div>
            </div>
          </div>
          <div class="feature-item">
            <ApartmentOutlined class="feature-icon" />
            <div>
              <div class="feature-title">跨域信任策略</div>
              <div class="feature-desc">联盟成员之间细粒度授权访问</div>
            </div>
          </div>
          <div class="feature-item">
            <AuditOutlined class="feature-icon" />
            <div>
              <div class="feature-title">链上审计留证</div>
              <div class="feature-desc">每笔操作 Org1 + Org2 双方背书</div>
            </div>
          </div>
        </div>

        <div class="brand-footer">
          Hyperledger Fabric · Vue 3 · Gin · {{ new Date().getFullYear() }}
        </div>
      </div>

      <!-- Right: login form -->
      <div class="form-panel">
        <div class="form-inner">
          <div class="form-header">
            <div class="form-eyebrow">控制台访问</div>
            <h1 class="form-title">欢迎回来</h1>
            <p class="form-sub">请使用您的账号登录系统</p>
          </div>

          <a-form layout="vertical" @submit.prevent="doLogin" class="login-form">
            <a-form-item label="用户名">
              <a-input
                v-model:value="form.username"
                size="large"
                placeholder="请输入用户名"
                allowClear
                @pressEnter="doLogin"
              >
                <template #prefix>
                  <UserOutlined style="color: #94a3b8" />
                </template>
              </a-input>
            </a-form-item>

            <a-form-item label="密码">
              <a-input-password
                v-model:value="form.password"
                size="large"
                placeholder="请输入密码"
                allowClear
                @pressEnter="doLogin"
              >
                <template #prefix>
                  <LockOutlined style="color: #94a3b8" />
                </template>
              </a-input-password>
            </a-form-item>

            <a-button
              type="primary"
              block
              size="large"
              :loading="loading"
              class="submit-btn"
              @click="doLogin"
            >
              登 录
            </a-button>
          </a-form>

          <div class="demo-hint" @click="fillDemo">
            <span class="demo-dot"></span>
            <span>默认账号：</span>
            <span class="mono">admin / 123456</span>
            <span class="demo-action">（点击自动填充）</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-page {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: linear-gradient(135deg, #f0f4ff 0%, #e8f0ff 50%, #fef3e0 100%);
  overflow: hidden;
}

.bg-blob {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.45;
  pointer-events: none;
  z-index: 0;
}

.blob-1 {
  width: 480px;
  height: 480px;
  background: linear-gradient(135deg, #182078 0%, #5b6ce4 100%);
  top: -120px;
  left: -100px;
}

.blob-2 {
  width: 420px;
  height: 420px;
  background: linear-gradient(135deg, #f8c000 0%, #ffa840 100%);
  bottom: -160px;
  right: -120px;
  opacity: 0.35;
}

.login-shell {
  position: relative;
  z-index: 1;
  display: flex;
  width: 920px;
  max-width: 100%;
  min-height: 560px;
  border-radius: 18px;
  overflow: hidden;
  box-shadow: 0 24px 64px rgba(16, 24, 40, 0.12), 0 4px 12px rgba(16, 24, 40, 0.04);
  background: #fff;
}

.brand-panel {
  flex: 1;
  background: linear-gradient(150deg, #1a247e 0%, #2c3aa3 50%, #4453d9 100%);
  color: #fff;
  padding: 44px 38px 32px;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  position: relative;
  overflow: hidden;
}

.brand-panel::before {
  content: '';
  position: absolute;
  top: -40%;
  right: -30%;
  width: 80%;
  height: 80%;
  background: radial-gradient(circle, rgba(248, 192, 0, 0.18) 0%, transparent 60%);
  pointer-events: none;
}

.brand-panel::after {
  content: '';
  position: absolute;
  bottom: -20%;
  left: -10%;
  width: 60%;
  height: 60%;
  background: radial-gradient(circle, rgba(91, 108, 228, 0.4) 0%, transparent 60%);
  pointer-events: none;
}

.brand-top {
  position: relative;
  z-index: 1;
}

.brand-logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 56px;
  height: 56px;
  border-radius: 14px;
  background: rgba(248, 192, 0, 0.16);
  border: 1px solid rgba(248, 192, 0, 0.4);
  font-size: 28px;
  color: #f8c000;
  margin-bottom: 18px;
}

.brand-name {
  font-size: 32px;
  font-weight: 800;
  letter-spacing: 1px;
  background: linear-gradient(90deg, #ffffff 0%, #f8c000 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.brand-tagline {
  margin-top: 14px;
  font-size: 13px;
  line-height: 1.7;
  color: rgba(255, 255, 255, 0.78);
  max-width: 280px;
}

.brand-features {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 18px;
  margin-top: 36px;
}

.feature-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
}

.feature-icon {
  font-size: 20px;
  color: #f8c000;
  margin-top: 2px;
  flex-shrink: 0;
}

.feature-title {
  font-size: 13px;
  font-weight: 600;
  color: #ffffff;
  letter-spacing: 0.4px;
}

.feature-desc {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.6);
  margin-top: 2px;
  line-height: 1.5;
}

.brand-footer {
  position: relative;
  z-index: 1;
  font-size: 11px;
  color: rgba(255, 255, 255, 0.45);
  letter-spacing: 0.5px;
  margin-top: 24px;
}

.form-panel {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 44px 48px;
  background: #fff;
}

.form-inner {
  width: 100%;
  max-width: 360px;
}

.form-header {
  margin-bottom: 28px;
}

.form-eyebrow {
  font-size: 11px;
  font-weight: 600;
  color: #4453d9;
  letter-spacing: 1.2px;
  text-transform: uppercase;
  margin-bottom: 8px;
}

.form-title {
  font-size: 26px;
  font-weight: 700;
  color: #0f172a;
  margin: 0;
  letter-spacing: 0.4px;
}

.form-sub {
  margin: 6px 0 0;
  color: #64748b;
  font-size: 13px;
}

.login-form :deep(.ant-form-item-label) > label {
  font-size: 12px;
  color: #475569;
  font-weight: 500;
}

.login-form :deep(.ant-input-affix-wrapper-lg) {
  border-radius: 10px;
}

.login-form :deep(.ant-input-affix-wrapper:focus),
.login-form :deep(.ant-input-affix-wrapper-focused) {
  border-color: #4453d9;
  box-shadow: 0 0 0 3px rgba(68, 83, 217, 0.12);
}

.submit-btn {
  margin-top: 6px;
  height: 44px;
  border-radius: 10px;
  font-weight: 600;
  letter-spacing: 4px;
  background: linear-gradient(135deg, #182078 0%, #4453d9 100%);
  border: none;
  box-shadow: 0 6px 16px rgba(24, 32, 120, 0.28);
  transition: transform 0.15s, box-shadow 0.15s;
}

.submit-btn:hover {
  transform: translateY(-1px);
  box-shadow: 0 10px 24px rgba(24, 32, 120, 0.36);
}

.submit-btn:active {
  transform: translateY(0);
}

.demo-hint {
  margin-top: 22px;
  padding: 10px 14px;
  border-radius: 10px;
  background: #f8fafc;
  border: 1px dashed #cbd5e1;
  font-size: 12px;
  color: #64748b;
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  transition: all 0.15s;
}

.demo-hint:hover {
  background: #eef2ff;
  border-color: #a5b4fc;
  color: #4453d9;
}

.demo-dot {
  width: 6px;
  height: 6px;
  border-radius: 999px;
  background: #4453d9;
  flex-shrink: 0;
}

.demo-action {
  margin-left: auto;
  font-size: 11px;
  color: #94a3b8;
}

.mono {
  font-family: var(--mono);
  color: #0f172a;
  font-weight: 600;
}

@media (max-width: 768px) {
  .login-shell {
    width: 100%;
    min-height: auto;
    flex-direction: column;
  }

  .brand-panel {
    padding: 32px 28px 24px;
  }

  .brand-name {
    font-size: 26px;
  }

  .brand-features {
    flex-direction: row;
    gap: 12px;
    margin-top: 22px;
  }

  .feature-item {
    flex-direction: column;
    align-items: flex-start;
    flex: 1;
  }

  .feature-desc {
    display: none;
  }

  .form-panel {
    padding: 36px 28px;
  }
}

@media (max-width: 480px) {
  .brand-features {
    display: none;
  }

  .brand-footer {
    display: none;
  }
}
</style>
