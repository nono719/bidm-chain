<script setup>
import { computed, onMounted, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import { apiRequest, getUser } from '../api/client'

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const loading = ref(false)
const rows = ref([])

const receiptOpen = ref(false)
const receipt = ref(null)

function fmt(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

function shortDid(v) {
  if (!v) return '-'
  if (v.length <= 22) return v
  return `${v.slice(0, 14)}...${v.slice(-6)}`
}

async function load() {
  if (!isAdmin.value) return
  loading.value = true
  try {
    const res = await apiRequest('/api/cross/pending')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载失败')
      return
    }
    rows.value = res.data
  } finally {
    loading.value = false
  }
}

async function decide(record, approve) {
  const res = await apiRequest('/api/cross/approve', {
    method: 'POST',
    body: { requestId: record.requestId, approve }
  })
  if (res.code !== 0) {
    antdMessage.error(res.message || '操作失败')
    return
  }
  if (approve) {
    receipt.value = res.data
    receiptOpen.value = true
    antdMessage.success('已审批并签发凭证')
  } else {
    antdMessage.info('已拒绝')
  }
  await load()
}

const columns = [
  { title: '请求ID', dataIndex: 'requestId', key: 'requestId', width: 180 },
  { title: '设备DID', dataIndex: 'deviceDid', key: 'deviceDid', width: 220 },
  { title: '源域', dataIndex: 'fromDomainCode', key: 'fromDomainCode', width: 120 },
  { title: '目标域', dataIndex: 'toDomainCode', key: 'toDomainCode', width: 120 },
  { title: '资源', dataIndex: 'resource', key: 'resource', width: 140 },
  { title: '权限', dataIndex: 'permission', key: 'permission', width: 120 },
  { title: '申请人', dataIndex: 'requestedBy', key: 'requestedBy', width: 140 },
  { title: '签名校验时间', dataIndex: 'signatureVerifiedAt', key: 'signatureVerifiedAt', width: 180 },
  { title: '操作', key: 'action', width: 160 }
]

onMounted(load)
</script>

<template>
  <a-card class="panel-card" title="跨域审批">
    <template v-if="!isAdmin">
      <a-result status="403" title="无权限" sub-title="仅管理员可审批跨域认证" />
    </template>

    <template v-else>
      <a-space style="margin-bottom: 12px">
        <a-button :loading="loading" @click="load">刷新</a-button>
        <a-tag color="gold">待审批：{{ rows.length }}</a-tag>
      </a-space>

      <a-card class="inner-card" size="small">
        <a-table :columns="columns" :dataSource="rows" rowKey="requestId" size="small" :loading="loading" :pagination="{ pageSize: 10 }">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'deviceDid'">
              <span class="mono">{{ shortDid(record.deviceDid) }}</span>
            </template>
            <template v-else-if="column.key === 'signatureVerifiedAt'">
              {{ fmt(record.signatureVerifiedAt) }}
            </template>
            <template v-else-if="column.key === 'action'">
              <a-space>
                <a-button type="primary" size="small" @click="decide(record, true)">通过</a-button>
                <a-button danger size="small" @click="decide(record, false)">拒绝</a-button>
              </a-space>
            </template>
          </template>
        </a-table>
      </a-card>

      <a-modal v-model:open="receiptOpen" title="审批结果" okText="关闭" cancelText="关闭" @ok="receiptOpen = false" @cancel="receiptOpen = false">
        <a-descriptions size="small" :column="1" bordered>
          <a-descriptions-item label="请求ID">{{ receipt?.requestId || '-' }}</a-descriptions-item>
          <a-descriptions-item label="状态">{{ receipt?.status || '-' }}</a-descriptions-item>
          <a-descriptions-item label="认证时间">{{ receipt?.verifiedAt ? fmt(receipt.verifiedAt) : '-' }}</a-descriptions-item>
          <a-descriptions-item label="过期时间">{{ receipt?.expiresAt ? fmt(receipt.expiresAt) : '-' }}</a-descriptions-item>
          <a-descriptions-item label="区块高度">{{ receipt?.blockHeight ?? '-' }}</a-descriptions-item>
          <a-descriptions-item label="交易哈希"><span class="mono">{{ receipt?.txHash || '-' }}</span></a-descriptions-item>
          <a-descriptions-item label="主体DID">{{ receipt?.token?.subject || '-' }}</a-descriptions-item>
          <a-descriptions-item label="源域">{{ receipt?.token?.srcDomain || '-' }}</a-descriptions-item>
          <a-descriptions-item label="目标域">{{ receipt?.token?.dstDomain || '-' }}</a-descriptions-item>
          <a-descriptions-item label="资源">{{ receipt?.token?.resource || '-' }}</a-descriptions-item>
          <a-descriptions-item label="权限">{{ receipt?.token?.permission || '-' }}</a-descriptions-item>
        </a-descriptions>
      </a-modal>
    </template>
  </a-card>
</template>

<style scoped>
.mono {
  font-family: var(--mono);
}
</style>

