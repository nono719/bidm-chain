<script setup>
import { computed, ref, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
import {
  AuditOutlined,
  DashboardOutlined,
  DeploymentUnitOutlined,
  DatabaseOutlined,
  MonitorOutlined,
  SafetyCertificateOutlined,
  TeamOutlined,
  CheckCircleOutlined,
  SettingOutlined,
  BlockOutlined
} from '@ant-design/icons-vue'
import { apiRequest, getToken, getUser, setToken, setUser } from '../api/client'

const route = useRoute()
const router = useRouter()

const currentUser = ref(getUser())

const navItems = computed(() => {
  const base = [
    { key: '/', label: '控制台', icon: DashboardOutlined },
    { key: '/devices/register', label: '设备注册', icon: DeploymentUnitOutlined },
    { key: '/devices/manage', label: '设备管理', icon: DatabaseOutlined },
    { key: '/trust-policies', label: '信任策略', icon: SafetyCertificateOutlined },
    { key: '/auth/cross-domain', label: '跨域认证', icon: SafetyCertificateOutlined },
    { key: '/remote-console', label: '远程运维台', icon: DeploymentUnitOutlined },
    { key: '/monitor', label: '状态监控', icon: MonitorOutlined },
    { key: '/audit', label: '审计日志', icon: AuditOutlined },
    { key: '/chain', label: '联盟链浏览', icon: BlockOutlined },
    { key: '/oracle', label: '预言机控制中心', icon: MonitorOutlined }
  ]
  if (!currentUser.value?.role) {
    return base.filter((i) => i.key !== '/trust-policies')
  }
  if (currentUser.value?.role === 'ADMIN') {
    base.splice(1, 0, { key: '/approvals', label: '跨域审批', icon: CheckCircleOutlined })
    base.push({ key: '/users', label: '用户管理', icon: TeamOutlined })
    base.push({ key: '/system/manage', label: '系统管理', icon: SettingOutlined })
  }
  return base
})

const selectedKeys = computed(() => [route.path])

const token = ref(getToken())

function shortToken(v) {
  if (!v) return '未登录'
  return `${v.slice(0, 14)}...${v.slice(-10)}`
}

function logout() {
  token.value = ''
  setToken('')
  setUser(null)
  currentUser.value = null
  router.replace('/login')
  antdMessage.info('已退出登录')
}

function goLogin() {
  router.push('/login')
}

function onNavClick(key) {
  router.push(key)
}

async function refreshMe() {
  if (!token.value) return
  const res = await apiRequest('/api/auth/me')
  if (res.code === 0) {
    setUser(res.data)
    currentUser.value = res.data
    return
  }
}

watchEffect(() => {
  if (token.value) refreshMe()
})
</script>

<template>
  <a-config-provider
    :theme="{ token: { colorPrimary: '#182078', colorInfo: '#182078', borderRadius: 10 } }"
  >
    <a-layout class="app-layout">
      <a-layout-sider class="app-sider" :width="208" theme="dark">
        <div class="brand">
          <div class="brand-title">BIDM-Chain</div>
          <div class="brand-sub">跨域认证与审计控制台</div>
        </div>
        <a-menu theme="dark" mode="inline" :selectedKeys="selectedKeys">
          <a-menu-item v-for="it in navItems" :key="it.key" @click="onNavClick(it.key)">
            <component :is="it.icon" />
            <span>{{ it.label }}</span>
          </a-menu-item>
        </a-menu>
      </a-layout-sider>

      <a-layout>
        <a-layout-header class="app-header">
          <div class="header-left">
            <div class="page-title">{{ navItems.find((i) => i.key === route.path)?.label || 'BIDM-Chain' }}</div>
          </div>
          <div class="header-right">
            <a-tag v-if="currentUser?.role" color="blue">{{ currentUser.role }}</a-tag>
            <a-tag v-if="currentUser?.domainCode" color="geekblue">{{ currentUser.domainCode }}</a-tag>
            <a-tag v-if="token" color="default">{{ shortToken(token) }}</a-tag>
            <a-button v-if="!token" type="primary" size="small" @click="goLogin">登录</a-button>
            <a-button v-else size="small" @click="logout">退出</a-button>
          </div>
        </a-layout-header>

        <a-layout-content class="app-content">
          <router-view />
        </a-layout-content>
      </a-layout>
    </a-layout>
  </a-config-provider>
</template>

<style scoped>
.app-layout {
  min-height: 100vh;
}

.app-sider {
  background: var(--primary);
}

.app-sider :deep(.ant-layout-sider-children) {
  background: var(--primary);
}

.app-sider :deep(.ant-menu) {
  background: var(--primary);
}

.app-sider :deep(.ant-menu-item) {
  margin-inline: 10px;
  width: calc(100% - 20px);
  border-radius: 10px;
}

.app-sider :deep(.ant-menu-item-selected) {
  background: rgba(248, 192, 0, 0.18) !important;
}

.app-sider :deep(.ant-menu-item-selected::after) {
  border-right: 3px solid var(--accent);
}

.brand {
  padding: 18px 16px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.12);
}

.brand-title {
  font-weight: 800;
  color: rgba(255, 255, 255, 0.95);
  letter-spacing: 0.4px;
}

.brand-sub {
  margin-top: 6px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.6);
}

.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 52px;
  line-height: 52px;
  padding: 0 18px;
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}

.header-left {
  display: flex;
  align-items: center;
}

.page-title {
  font-weight: 700;
  font-size: 15px;
  color: var(--text);
  letter-spacing: 0.3px;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.app-content {
  padding: 18px;
  background: var(--bg);
}
</style>
