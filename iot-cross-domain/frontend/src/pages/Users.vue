<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import { apiRequest, getUser } from '../api/client'

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const loading = ref(false)
const users = ref([])
const domains = ref([])

const createOpen = ref(false)
const createLoading = ref(false)
const createDomainOpen = ref(false)
const createDomainLoading = ref(false)
const editOpen = ref(false)
const editLoading = ref(false)
const editForm = reactive({
  id: 0,
  username: '',
  displayName: '',
  role: 'DOMAIN_ADMIN',
  domainCode: '',
  password: '' // optional — empty means "don't change"
})
const form = reactive({
  username: '',
  displayName: '',
  password: '',
  role: 'DOMAIN_ADMIN',
  domainCode: 'domain-a'
})
const domainForm = reactive({
  code: '',
  name: '',
  adminUsername: '',
  adminName: '',
  adminPassword: ''
})

function fmt(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

async function load() {
  if (!isAdmin.value) return
  loading.value = true
  try {
    const [u, d] = await Promise.all([apiRequest('/api/users'), apiRequest('/api/domains')])
    if (d.code === 0) domains.value = d.data
    if (u.code === 0) users.value = u.data
  } finally {
    loading.value = false
  }
}

async function createUser() {
  createLoading.value = true
  try {
    const body = {
      username: form.username,
      displayName: form.displayName,
      password: form.password,
      role: form.role,
      domainCode: form.role === 'DOMAIN_ADMIN' ? form.domainCode : ''
    }
    const res = await apiRequest('/api/users', { method: 'POST', body })
    if (res.code !== 0) {
      antdMessage.error(res.message || '创建失败')
      return
    }
    antdMessage.success('用户创建成功')
    createOpen.value = false
    form.username = ''
    form.displayName = ''
    form.password = ''
    form.role = 'DOMAIN_ADMIN'
    form.domainCode = domains.value?.[0]?.code || 'domain-a'
    await load()
  } finally {
    createLoading.value = false
  }
}

async function createDomainWithAdmin() {
  createDomainLoading.value = true
  try {
    const body = {
      code: domainForm.code,
      name: domainForm.name,
      adminUsername: domainForm.adminUsername,
      adminName: domainForm.adminName,
      adminPassword: domainForm.adminPassword
    }
    const res = await apiRequest('/api/domains/with-admin', { method: 'POST', body })
    if (res.code !== 0) {
      antdMessage.error(res.message || '创建失败')
      return
    }
    antdMessage.success(`域 ${res.data.domain.code} 与管理员 ${res.data.admin.username} 创建成功`)
    createDomainOpen.value = false
    domainForm.code = ''
    domainForm.name = ''
    domainForm.adminUsername = ''
    domainForm.adminName = ''
    domainForm.adminPassword = ''
    await load()
  } finally {
    createDomainLoading.value = false
  }
}

const columns = [
  { title: '用户名', dataIndex: 'username', key: 'username', width: 160 },
  { title: '姓名', dataIndex: 'displayName', key: 'displayName', width: 180 },
  { title: '角色', dataIndex: 'role', key: 'role', width: 140 },
  { title: '域', dataIndex: 'domainCode', key: 'domainCode', width: 140 },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 180 },
  { title: '操作', key: 'action', width: 180 }
]

function openEdit(record) {
  editForm.id = record.id
  editForm.username = record.username
  editForm.displayName = record.displayName || ''
  editForm.role = record.role
  editForm.domainCode = record.domainCode || ''
  editForm.password = ''
  editOpen.value = true
}

async function saveEdit() {
  // Only send fields that actually changed; helps backend distinguish
  // "not provided" from "explicitly empty".
  const body = {}
  const orig = users.value.find((u) => u.id === editForm.id) || {}
  if (editForm.displayName.trim() && editForm.displayName !== orig.displayName) body.displayName = editForm.displayName.trim()
  if (editForm.role !== orig.role) body.role = editForm.role
  // Always send domainCode so backend can apply role-based domain rule
  body.domainCode = editForm.role === 'ADMIN' ? '' : editForm.domainCode
  if (editForm.password.trim()) body.password = editForm.password.trim()

  if (Object.keys(body).length === 0) {
    antdMessage.info('未做任何修改')
    editOpen.value = false
    return
  }

  editLoading.value = true
  try {
    const res = await apiRequest(`/api/users/${editForm.id}`, { method: 'PUT', body })
    if (res.code !== 0) {
      antdMessage.error(res.message || '保存失败')
      return
    }
    antdMessage.success(`已更新 ${editForm.username}`)
    editOpen.value = false
    await load()
  } finally {
    editLoading.value = false
  }
}

async function revokeDomainAdmin(record) {
  const res = await apiRequest(`/api/users/${record.id}/revoke`, { method: 'POST' })
  if (res.code !== 0) {
    antdMessage.error(res.message || '吊销失败')
    return
  }
  antdMessage.success(`已吊销 ${record.username}`)
  await load()
}

onMounted(load)
</script>

<template>
  <a-card class="panel-card" title="用户管理">
    <template v-if="!isAdmin">
      <a-result status="403" title="无权限" sub-title="仅管理员可管理用户" />
    </template>

    <template v-else>
      <a-space style="margin-bottom: 12px">
        <a-button type="primary" ghost @click="createDomainOpen = true">创建域+管理员</a-button>
        <a-button type="primary" @click="createOpen = true">创建用户</a-button>
        <a-button :loading="loading" @click="load">刷新</a-button>
      </a-space>

      <a-card class="inner-card" size="small">
        <a-table :columns="columns" :dataSource="users" rowKey="id" size="small" :loading="loading" :pagination="{ pageSize: 10 }">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'role'">
              <a-tag :color="record.role === 'ADMIN' ? 'red' : 'blue'">{{ record.role }}</a-tag>
            </template>
            <template v-else-if="column.key === 'domainCode'">
              <a-tag v-if="record.domainCode" color="geekblue">{{ record.domainCode }}</a-tag>
              <span v-else>-</span>
            </template>
            <template v-else-if="column.key === 'createdAt'">
              {{ fmt(record.createdAt) }}
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space size="small">
                <a-button size="small" @click="openEdit(record)">编辑</a-button>
                <a-popconfirm
                  v-if="record.role === 'DOMAIN_ADMIN'"
                  title="确认吊销该域管理员？"
                  ok-text="吊销"
                  cancel-text="取消"
                  @confirm="revokeDomainAdmin(record)"
                >
                  <a-button danger size="small">吊销</a-button>
                </a-popconfirm>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-card>

      <!-- Edit user modal -->
      <a-modal v-model:open="editOpen" :title="`编辑用户：${editForm.username}`" okText="保存" cancelText="取消" :confirmLoading="editLoading" @ok="saveEdit">
        <a-form layout="vertical">
          <a-form-item label="用户名">
            <a-input :value="editForm.username" disabled />
            <div style="font-size: 11px; color: #94a3b8; margin-top: 4px">用户名是业务唯一标识，不可修改</div>
          </a-form-item>
          <a-form-item label="姓名">
            <a-input v-model:value="editForm.displayName" placeholder="例如 域A管理员" />
          </a-form-item>
          <a-form-item label="角色">
            <a-select v-model:value="editForm.role" :options="[{ value: 'DOMAIN_ADMIN', label: 'DOMAIN_ADMIN' }, { value: 'ADMIN', label: 'ADMIN' }]" />
          </a-form-item>
          <a-form-item v-if="editForm.role === 'DOMAIN_ADMIN'" label="所属域">
            <a-select v-model:value="editForm.domainCode" :options="domains.map((d) => ({ value: d.code, label: `${d.name}（${d.code}）` }))" placeholder="请选择" />
          </a-form-item>
          <a-form-item label="重置密码（可选）">
            <a-input-password v-model:value="editForm.password" placeholder="留空表示不修改密码，至少 4 位" autocomplete="new-password" />
          </a-form-item>
          <a-alert
            v-if="editForm.username === me?.username"
            type="warning"
            showIcon
            message="正在编辑自己的账号；不允许将自己降级为非 ADMIN（避免误操作锁死自己）。"
          />
        </a-form>
      </a-modal>

      <a-modal v-model:open="createOpen" title="创建用户" okText="创建" cancelText="取消" :confirmLoading="createLoading" @ok="createUser">
        <a-form layout="vertical">
          <a-form-item label="用户名">
            <a-input v-model:value="form.username" placeholder="例如 domainA-admin" />
          </a-form-item>
          <a-form-item label="姓名">
            <a-input v-model:value="form.displayName" placeholder="例如 域A管理员" />
          </a-form-item>
          <a-form-item label="初始密码">
            <a-input-password v-model:value="form.password" placeholder="至少 6 位" />
          </a-form-item>
          <a-form-item label="角色">
            <a-select v-model:value="form.role" :options="[{ value: 'DOMAIN_ADMIN', label: 'DOMAIN_ADMIN' }, { value: 'ADMIN', label: 'ADMIN' }]" />
          </a-form-item>
          <a-form-item v-if="form.role === 'DOMAIN_ADMIN'" label="所属域">
            <a-select v-model:value="form.domainCode" :options="domains.map((d) => ({ value: d.code, label: `${d.name}（${d.code}）` }))" />
          </a-form-item>
          <a-alert type="info" showIcon message="域管理员登录后，创建设备默认属于其域；跨域认证需管理员审批。" />
        </a-form>
      </a-modal>

      <a-modal
        v-model:open="createDomainOpen"
        title="创建域并生成域管理员"
        okText="创建"
        cancelText="取消"
        :confirmLoading="createDomainLoading"
        @ok="createDomainWithAdmin"
      >
        <a-form layout="vertical">
          <a-form-item label="域编码">
            <a-input v-model:value="domainForm.code" placeholder="例如 domain-c" />
          </a-form-item>
          <a-form-item label="域名称">
            <a-input v-model:value="domainForm.name" placeholder="例如 城市交通域" />
          </a-form-item>
          <a-divider style="margin: 8px 0 16px" />
          <a-form-item label="管理员用户名">
            <a-input v-model:value="domainForm.adminUsername" placeholder="例如 domain-c-admin" />
          </a-form-item>
          <a-form-item label="管理员姓名">
            <a-input v-model:value="domainForm.adminName" placeholder="例如 域C管理员" />
          </a-form-item>
          <a-form-item label="管理员初始密码">
            <a-input-password v-model:value="domainForm.adminPassword" placeholder="至少 6 位" />
          </a-form-item>
          <a-alert type="info" showIcon message="提交后会原子创建新域与 DOMAIN_ADMIN 账号。若任一步失败会整体回滚。" />
        </a-form>
      </a-modal>
    </template>
  </a-card>
</template>
