<script setup>
import { reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
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
</script>

<template>
  <div class="login-page">
    <a-card class="login-card" title="BIDM-Chain 登录" :bordered="false">
      <a-form layout="vertical" @submit.prevent="doLogin">
        <a-form-item label="用户名">
          <a-input v-model:value="form.username" placeholder="admin" />
        </a-form-item>
        <a-form-item label="密码">
          <a-input-password v-model:value="form.password" placeholder="123456" />
        </a-form-item>
        <a-button type="primary" block :loading="loading" @click="doLogin">登录</a-button>
      </a-form>
      <a-alert style="margin-top: 14px" type="info" showIcon message="默认账号：admin / 123456" />
    </a-card>
  </div>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f4f6fb;
  padding: 16px;
}

.login-card {
  width: 420px;
  border-radius: 12px;
  box-shadow: 0 12px 32px rgba(16, 24, 40, 0.08);
}
</style>
