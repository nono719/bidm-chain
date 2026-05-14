<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import { DownloadOutlined } from '@ant-design/icons-vue'
import dayjs from 'dayjs'
import { apiRequest, getBaseURL, getToken } from '../api/client'

const loading = ref(false)
const logs = ref([])
const selected = ref(null)

const filters = reactive({
  module: 'ALL',
  result: 'ALL',
  q: '',
  range: []
})

function shortHash(v) {
  if (!v) return '-'
  return `${v.slice(0, 10)}...${v.slice(-8)}`
}

function shortDid(v) {
  if (!v) return '-'
  if (v.length <= 22) return v
  return `${v.slice(0, 14)}...${v.slice(-6)}`
}

function fmt(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}

function buildQuery() {
  const params = new URLSearchParams()
  if (filters.module !== 'ALL') params.set('module', filters.module)
  if (filters.result !== 'ALL') params.set('result', filters.result)
  if (filters.q) params.set('q', filters.q)
  if (filters.range && filters.range.length === 2) {
    const [a, b] = filters.range
    if (a) params.set('from', dayjs(a).toISOString())
    if (b) params.set('to', dayjs(b).toISOString())
  }
  params.set('limit', '200')
  return params.toString()
}

async function search() {
  loading.value = true
  try {
    const res = await apiRequest(`/api/audit/search?${buildQuery()}`)
    if (res.code !== 0) {
      antdMessage.error(res.message || '查询失败')
      return
    }
    logs.value = res.data
  } finally {
    loading.value = false
  }
}

function reset() {
  filters.module = 'ALL'
  filters.result = 'ALL'
  filters.q = ''
  filters.range = []
  search()
}

const exporting = ref(false)
async function exportCsv() {
  exporting.value = true
  try {
    const params = new URLSearchParams()
    if (filters.module !== 'ALL') params.set('module', filters.module)
    if (filters.result !== 'ALL') params.set('result', filters.result)
    if (filters.q) params.set('q', filters.q)
    if (filters.range && filters.range.length === 2) {
      const [a, b] = filters.range
      if (a) params.set('from', dayjs(a).toISOString())
      if (b) params.set('to', dayjs(b).toISOString())
    }
    params.set('limit', '5000')

    const resp = await fetch(`${getBaseURL()}/api/audit/export?${params.toString()}`, {
      headers: { Authorization: `Bearer ${getToken()}` }
    })
    if (!resp.ok) {
      antdMessage.error(`导出失败：HTTP ${resp.status}`)
      return
    }

    // Derive filename from Content-Disposition; fall back to a stamp.
    const cd = resp.headers.get('Content-Disposition') || ''
    let filename = `audit_log_${dayjs().format('YYYYMMDD_HHmmss')}.csv`
    const m = cd.match(/filename="([^"]+)"/)
    if (m) filename = m[1]

    const blob = await resp.blob()
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    antdMessage.success(`已导出 ${filename}`)
  } catch (e) {
    antdMessage.error(`导出异常：${e?.message || e}`)
  } finally {
    exporting.value = false
  }
}

onMounted(search)

const columns = [
  { title: '调用方法', dataIndex: 'method', key: 'method', width: 140 },
  { title: '合约', dataIndex: 'contract', key: 'contract', width: 120 },
  { title: 'From', dataIndex: 'from', key: 'from', width: 120 },
  { title: '主体DID', dataIndex: 'subjectDid', key: 'subjectDid', width: 200 },
  { title: '区块高度', dataIndex: 'blockHeight', key: 'blockHeight', width: 110 },
  { title: '交易哈希', dataIndex: 'txHash', key: 'txHash', width: 170 },
  { title: 'Gas', dataIndex: 'gasUsed', key: 'gasUsed', width: 90 },
  { title: '状态', dataIndex: 'result', key: 'result', width: 110 },
  { title: '时间', dataIndex: 'occurredAt', key: 'occurredAt', width: 170 }
]

const detailPairs = computed(() => {
  if (!selected.value) return []
  let detail = null
  try {
    detail = selected.value.detailJson ? JSON.parse(selected.value.detailJson) : null
  } catch {
    detail = null
  }
  if (!detail || typeof detail !== 'object') return []

  const rows = []
  for (const k of Object.keys(detail)) {
    const v = detail[k]
    if (v === null || v === undefined) continue
    if (typeof v === 'object') {
      rows.push({ k, v: Array.isArray(v) ? `Array(${v.length})` : 'Object' })
      continue
    }
    rows.push({ k, v: String(v) })
  }
  return rows
})

function setSelected(record) {
  selected.value = record
}

function resultTag(r) {
  if (r === 'OK') return { color: 'green', text: 'OK' }
  if (r === 'DENY') return { color: 'red', text: 'DENY' }
  if (r === 'FAIL') return { color: 'red', text: 'FAIL' }
  return { color: 'orange', text: r || '-' }
}
</script>

<template>
  <a-card class="panel-card" title="审计日志（图5-5）">
    <template #extra>
      <a-space>
        <a-tag color="blue">共 {{ logs.length }} 条</a-tag>
        <a-button :loading="exporting" :disabled="!logs.length" @click="exportCsv">
          <template #icon><DownloadOutlined /></template>
          导出 CSV
        </a-button>
      </a-space>
    </template>
    <a-card class="inner-card" size="small">
      <a-row :gutter="12" align="middle">
        <a-col :xs="24" :lg="4">
          <a-select
            v-model:value="filters.module"
            :options="[
              { value: 'ALL', label: '全部类型' },
              { value: 'device', label: 'device' },
              { value: 'cross_auth', label: 'cross_auth' },
              { value: 'oracle', label: 'oracle' },
              { value: 'operation', label: 'operation' },
              { value: 'domain', label: 'domain' }
            ]"
          />
        </a-col>
        <a-col :xs="24" :lg="4">
          <a-select
            v-model:value="filters.result"
            :options="[
              { value: 'ALL', label: '全部状态' },
              { value: 'OK', label: 'OK' },
              { value: 'DENY', label: 'DENY' },
              { value: 'FAIL', label: 'FAIL' }
            ]"
          />
        </a-col>
        <a-col :xs="24" :lg="8">
          <a-range-picker v-model:value="filters.range" show-time style="width: 100%" />
        </a-col>
        <a-col :xs="24" :lg="5">
          <a-input v-model:value="filters.q" placeholder="全文关键字 / DID / txHash" />
        </a-col>
        <a-col :xs="24" :lg="3">
          <a-space style="display: flex; justify-content: flex-end">
            <a-button type="primary" :loading="loading" @click="search">查询</a-button>
            <a-button @click="reset">重置</a-button>
          </a-space>
        </a-col>
      </a-row>
    </a-card>

    <a-card class="inner-card" size="small" style="margin-top: 12px">
      <a-table
        size="small"
        :columns="columns"
        :dataSource="logs"
        rowKey="id"
        :pagination="{ pageSize: 10 }"
        :loading="loading"
        :customRow="(record) => ({ onClick: () => setSelected(record) })"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'subjectDid'">
            <span class="mono">{{ shortDid(record.subjectDid) }}</span>
          </template>
          <template v-else-if="column.key === 'txHash'">
            <span class="mono">{{ shortHash(record.txHash) }}</span>
          </template>
          <template v-else-if="column.key === 'gasUsed'">
            {{ record.gasUsed ? record.gasUsed : 'N/A' }}
          </template>
          <template v-else-if="column.key === 'result'">
            <a-tag :color="resultTag(record.result).color">{{ resultTag(record.result).text }}</a-tag>
          </template>
          <template v-else-if="column.key === 'occurredAt'">
            {{ fmt(record.occurredAt) }}
          </template>
        </template>
      </a-table>
    </a-card>

    <a-card class="inner-card" size="small" style="margin-top: 12px" title="事件详情与溯源链路">
      <template v-if="selected">
        <a-row :gutter="12">
          <a-col :xs="24" :lg="12">
            <a-descriptions size="small" :column="1" bordered>
              <a-descriptions-item label="调用方法">{{ selected.method || '-' }}</a-descriptions-item>
              <a-descriptions-item label="合约">{{ selected.contract || '-' }}</a-descriptions-item>
              <a-descriptions-item label="From">{{ selected.from || '-' }}</a-descriptions-item>
              <a-descriptions-item label="主体DID"><span class="mono">{{ selected.subjectDid || '-' }}</span></a-descriptions-item>
              <a-descriptions-item label="区块高度">{{ selected.blockHeight ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="交易哈希"><span class="mono">{{ selected.txHash || '-' }}</span></a-descriptions-item>
              <a-descriptions-item label="状态">{{ selected.result }}</a-descriptions-item>
              <a-descriptions-item label="时间">{{ fmt(selected.occurredAt) }}</a-descriptions-item>
            </a-descriptions>
          </a-col>
          <a-col :xs="24" :lg="12">
            <a-card class="soft-card" size="small" title="事件详情">
              <a-descriptions size="small" :column="1" bordered>
                <a-descriptions-item label="模块">{{ selected.module || '-' }}</a-descriptions-item>
                <a-descriptions-item label="动作">{{ selected.action || '-' }}</a-descriptions-item>
                <a-descriptions-item label="执行人">{{ selected.operator || '-' }}</a-descriptions-item>
                <a-descriptions-item label="消息">{{ selected.message || '-' }}</a-descriptions-item>
                <a-descriptions-item label="Gas">{{ selected.gasUsed ? selected.gasUsed : 'N/A' }}</a-descriptions-item>
              </a-descriptions>

              <a-divider style="margin: 12px 0" />

              <template v-if="detailPairs.length">
                <a-table
                  size="small"
                  :dataSource="detailPairs"
                  :columns="[{ title: '字段', dataIndex: 'k', key: 'k', width: 160 }, { title: '值', dataIndex: 'v', key: 'v' }]"
                  :pagination="false"
                  rowKey="k"
                />
              </template>
              <template v-else>
                <a-empty description="无结构化详情" />
              </template>
            </a-card>
          </a-col>
        </a-row>
      </template>
      <template v-else>
        <a-empty description="点击上方列表中的一条事件查看详情" />
      </template>
    </a-card>
  </a-card>
</template>

<style scoped>
.mono {
  font-family: var(--mono);
  color: #344054;
}
</style>
