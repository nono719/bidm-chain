import { createRouter, createWebHistory } from 'vue-router'
import { getToken } from './api/client'
import Dashboard from './pages/Dashboard.vue'
import DeviceRegister from './pages/DeviceRegister.vue'
import DeviceManage from './pages/DeviceManage.vue'
import CrossDomainAuth from './pages/CrossDomainAuth.vue'
import Monitor from './pages/Monitor.vue'
import Audit from './pages/Audit.vue'
import Users from './pages/Users.vue'
import Approvals from './pages/Approvals.vue'
import TrustPolicies from './pages/TrustPolicies.vue'
import SystemManage from './pages/SystemManage.vue'
import Chain from './pages/Chain.vue'
import RemoteConsole from './pages/RemoteConsole.vue'
import Login from './pages/Login.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', name: 'login', component: Login },
    { path: '/', name: 'dashboard', component: Dashboard },
    { path: '/approvals', name: 'approvals', component: Approvals },
    { path: '/devices/register', name: 'deviceRegister', component: DeviceRegister },
    { path: '/devices/manage', name: 'deviceManage', component: DeviceManage },
    { path: '/auth/cross-domain', name: 'crossDomainAuth', component: CrossDomainAuth },
    { path: '/monitor', name: 'monitor', component: Monitor },
    { path: '/audit', name: 'audit', component: Audit },
    { path: '/users', name: 'users', component: Users },
    { path: '/trust-policies', name: 'trustPolicies', component: TrustPolicies },
    { path: '/system/manage', name: 'systemManage', component: SystemManage },
    { path: '/chain', name: 'chain', component: Chain },
    { path: '/remote-console', name: 'remoteConsole', component: RemoteConsole }
  ]
})

router.beforeEach((to) => {
  const token = getToken()
  if (to.path === '/login') {
    if (token) return '/'
    return true
  }
  if (!token) {
    return { path: '/login', query: { redirect: to.fullPath } }
  }
  return true
})

export default router
