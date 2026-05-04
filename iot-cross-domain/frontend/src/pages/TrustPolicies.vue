<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import { apiRequest, getUser } from '../api/client'

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const loading = ref(false)
const rows = ref([])
const domains = ref([])
const form = reactive({
  fromDomainCode: '',
  toDomainCode: '',
  policyLevel: 'ALLOW',
  allowedPerms: 'READ,WRITE',
  allowedRes: '*',
  enabled: true
})

async function loadDomains() {
  const res = await apiRequest('/api/domains')
  if (res.code !== 0) return
  const list = Array.isArray(res.data) ? res.data : []
  domains.value = list.map((d) => ({ value: d.code, label: `${d.name}（${d.code}）` }))
  if (isAdmin.value && !form.fromDomainCode && domains.value.length > 0) form.fromDomainCode = domains.value[0].value
  if (!isAdmin.value) form.fromDomainCode = me.value?.domainCode || ''
  if (!form.toDomainCode && domains.value.length > 0) form.toDomainCode = domains.value[0].value
}

async function load() {
  loading.value = true
  try {
    const res = await apiRequest('/api/trust-policies')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载失败')
      return
    }
    rows.value = res.data || []
  } finally {
    loading.value = false
  }
}

async function savePolicy() {
  const payload = {
    ...form,
    fromDomainCode: isAdmin.value ? form.fromDomainCode : (me.value?.domainCode || ''),
    toDomainCode: form.toDomainCode
  }
  const res = await apiRequest('/api/trust-policies', { method: 'POST', body: payload })
  if (res.code !== 0) {
    antdMessage.error(res.message || '保存失败')
    return
  }
  antdMessage.success('策略已保存')
  await load()
}

async function removePolicy(row) {
  const res = await apiRequest(`/api/trust-policies/${row.id}`, { method: 'DELETE' })
  if (res.code !== 0) {
    antdMessage.error(res.message || '删除失败')
    return
  }
  antdMessage.success('策略已删除')
  await load()
}

onMounted(async () => {
  await loadDomains()
  await load()
})
</script>

<template>
  <a-card class="panel-card" title="管理域信任策略">
    <a-card class="inner-card" size="small" title="策略编辑">
      <a-form layout="vertical">
        <a-row :gutter="12">
          <a-col :xs="24" :lg="6">
            <a-form-item label="源域">
              <a-select v-model:value="form.fromDomainCode" :disabled="!isAdmin" :options="domains" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :lg="6">
            <a-form-item label="目标域">
              <a-select v-model:value="form.toDomainCode" :options="domains" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :lg="4">
            <a-form-item label="级别">
              <a-select v-model:value="form.policyLevel" :options="[{value:'ALLOW',label:'ALLOW'},{value:'DENY',label:'DENY'}]" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :lg="8">
            <a-form-item label="允许权限(逗号分隔)">
              <a-input v-model:value="form.allowedPerms" />
            </a-form-item>
          </a-col>
        </a-row>
        <a-row :gutter="12">
          <a-col :xs="24" :lg="12">
            <a-form-item label="允许资源(逗号分隔，*代表全部)">
              <a-input v-model:value="form.allowedRes" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :lg="6">
            <a-form-item label="启用">
              <a-switch v-model:checked="form.enabled" />
            </a-form-item>
          </a-col>
          <a-col :xs="24" :lg="6" style="display:flex;align-items:end;">
            <a-button type="primary" @click="savePolicy">保存策略</a-button>
          </a-col>
        </a-row>
      </a-form>
    </a-card>

    <a-card class="inner-card" size="small" title="策略列表" style="margin-top: 12px;">
      <a-table :dataSource="rows" rowKey="id" :loading="loading" size="small" :pagination="{ pageSize: 10 }">
        <a-table-column title="源域" dataIndex="fromDomainCode" key="fromDomainCode" />
        <a-table-column title="目标域" dataIndex="toDomainCode" key="toDomainCode" />
        <a-table-column title="级别" dataIndex="policyLevel" key="policyLevel" />
        <a-table-column title="允许权限" dataIndex="allowedPerms" key="allowedPerms" />
        <a-table-column title="允许资源" dataIndex="allowedRes" key="allowedRes" />
        <a-table-column title="启用" key="enabled">
          <template #default="{ record }">
            <a-tag :color="record.enabled ? 'green' : 'default'">{{ record.enabled ? '启用' : '停用' }}</a-tag>
          </template>
        </a-table-column>
        <a-table-column title="操作" key="action">
          <template #default="{ record }">
            <a-popconfirm title="确认删除策略？" ok-text="删除" cancel-text="取消" @confirm="removePolicy(record)">
              <a-button size="small" danger>删除</a-button>
            </a-popconfirm>
          </template>
        </a-table-column>
      </a-table>
    </a-card>
  </a-card>
</template>
