<script setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import { apiRequest } from '../api/client'

const router = useRouter()
const loading = ref(false)
const data = ref(null)

const kpi = ref({ devicesTotal: 0, devicesOnline: 0, crossAuthToday: 0, alertsToday: 0 })
const recentCrossAuth = ref([])
const oracle = ref(null)

const chain = ref(null)
const chainTopo = ref(null)
const chainLoading = ref(false)

async function loadChain() {
  chainLoading.value = true
  try {
    const [infoRes, topoRes] = await Promise.all([
      apiRequest('/api/chain/info'),
      apiRequest('/api/chain/topology')
    ])
    if (infoRes.code === 0) chain.value = infoRes.data
    if (topoRes.code === 0) chainTopo.value = topoRes.data
  } finally {
    chainLoading.value = false
  }
}

async function load() {
  loading.value = true
  try {
    const res = await apiRequest('/api/dashboard/overview')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载失败')
      return
    }
    data.value = res.data
    kpi.value = res.data.kpi
    recentCrossAuth.value = res.data.recentCrossAuth || []
    oracle.value = res.data.oracle
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  loadChain()
})

const columns = [
  { title: '时间', dataIndex: 'verifiedAt', key: 'verifiedAt', width: 170 },
  { title: '源DID', dataIndex: 'deviceDid', key: 'deviceDid' },
  { title: '目标域', dataIndex: 'toDomain', key: 'toDomain', width: 120 },
  { title: '结果', dataIndex: 'status', key: 'status', width: 110 },
  { title: 'Tx', dataIndex: 'txHash', key: 'txHash', width: 170 }
]

function fmtTime(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}

function shortHash(v) {
  if (!v) return '-'
  return `${v.slice(0, 10)}...${v.slice(-8)}`
}

function goAudit() {
  router.push('/audit')
}

function goAuth() {
  router.push('/auth/cross-domain')
}

function goMonitor() {
  router.push('/monitor')
}

function goRegister() {
  router.push('/devices/register')
}

function goChain() {
  router.push('/chain')
}
</script>

<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-card class="chain-bar soft-card" :loading="chainLoading" :bodyStyle="{ padding: '14px 18px' }" @click="goChain">
      <div class="chain-bar-row">
        <div class="chain-bar-left">
          <span class="chain-bar-title">联盟链状态</span>
          <a-tag :color="chain?.queryOk ? 'green' : 'red'" style="margin-left: 8px">{{ chain?.queryOk ? '链上读：在线' : '链上读：离线' }}</a-tag>
        </div>
        <div class="chain-bar-cells">
          <div class="chain-cell">
            <div class="cell-label">通道</div>
            <div class="cell-value">{{ chain?.channel || '-' }}</div>
          </div>
          <div class="chain-cell">
            <div class="cell-label">链码</div>
            <div class="cell-value">{{ chain?.chaincode || '-' }} / {{ chain?.anchorFunc || '-' }}</div>
          </div>
          <div class="chain-cell">
            <div class="cell-label">区块高度</div>
            <div class="cell-value mono">{{ chain?.height ?? '-' }}</div>
          </div>
          <div class="chain-cell">
            <div class="cell-label">联盟节点</div>
            <div class="cell-value">
              <a-tag :color="chainTopo?.nodesOnline === chainTopo?.nodesTotal ? 'green' : 'orange'">
                {{ chainTopo?.nodesOnline ?? 0 }} / {{ chainTopo?.nodesTotal ?? 0 }} 在线
              </a-tag>
            </div>
          </div>
          <div class="chain-cell">
            <div class="cell-label">累计上链</div>
            <div class="cell-value mono">{{ chain?.totalAnchors ?? 0 }} 笔</div>
          </div>
        </div>
        <a-button type="link" size="small" @click.stop="goChain">进入浏览器 →</a-button>
      </div>
    </a-card>

    <a-row :gutter="16">
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="kpi soft-card" :loading="loading" @click="goRegister">
          <a-statistic title="已注册设备数" :value="kpi.devicesTotal" />
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="kpi soft-card" :loading="loading" @click="goMonitor">
          <a-statistic title="在线设备数" :value="kpi.devicesOnline" />
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="kpi soft-card" :loading="loading" @click="goAuth">
          <a-statistic title="今日跨域认证数" :value="kpi.crossAuthToday" />
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="kpi soft-card" :loading="loading" @click="goAudit">
          <a-statistic title="异常告警数" :value="kpi.alertsToday" />
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="16">
      <a-col :xs="24" :lg="14">
        <a-card title="近期跨域认证记录" :loading="loading" class="panel-card">
          <a-table
            :columns="columns"
            :dataSource="recentCrossAuth"
            size="small"
            :pagination="false"
            rowKey="requestId"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'verifiedAt'">
                {{ fmtTime(record.verifiedAt) }}
              </template>
              <template v-else-if="column.key === 'status'">
                <a-tag :color="record.status === 'VERIFIED' ? 'green' : 'orange'">{{ record.status }}</a-tag>
              </template>
              <template v-else-if="column.key === 'txHash'">
                <span class="mono">{{ shortHash(record.txHash) }}</span>
              </template>
            </template>
          </a-table>
          <div class="panel-actions">
            <a-button type="link" @click="goAudit">查看审计日志</a-button>
          </div>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="10">
        <a-card title="预言机网络状态" :loading="loading" class="panel-card">
          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="节点规模">
              {{ oracle?.nodes?.total ?? '-' }}（门限 {{ oracle?.nodes?.threshold ?? '-' }}）
            </a-descriptions-item>
            <a-descriptions-item label="聚合成功率">
              <a-progress :percent="Math.round((oracle?.successRate ?? 0) * 100)" size="small" status="active" />
            </a-descriptions-item>
            <a-descriptions-item label="最近上链时间">
              {{ oracle?.lastCommitAt ? fmtTime(oracle.lastCommitAt) : '-' }}
            </a-descriptions-item>
          </a-descriptions>
          <a-alert
            style="margin-top: 12px"
            showIcon
            type="info"
            message="链侧采用 Fabric test-network + 锚定链码（AnchorRecord）"
          />
        </a-card>
      </a-col>
    </a-row>
  </a-space>
</template>

<style scoped>
.kpi {
  cursor: pointer;
  border-radius: 12px;
}

.kpi:hover {
  box-shadow: var(--shadow-sm);
}

.panel-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 10px;
}

.mono {
  font-family: var(--mono);
  color: #344054;
}

.chain-bar {
  cursor: pointer;
  border-radius: 12px;
  background: linear-gradient(90deg, #182078 0%, #233399 60%, #3949d6 100%);
  color: #fff;
  border: none;
}

.chain-bar :deep(.ant-card-body) {
  color: #fff;
}

.chain-bar:hover {
  box-shadow: 0 6px 18px rgba(24, 32, 120, 0.25);
}

.chain-bar-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 16px;
}

.chain-bar-title {
  font-weight: 700;
  letter-spacing: 0.4px;
  font-size: 14px;
  color: #fff;
}

.chain-bar-cells {
  display: flex;
  flex: 1;
  gap: 22px;
  flex-wrap: wrap;
}

.chain-cell {
  min-width: 100px;
}

.cell-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
  letter-spacing: 0.3px;
}

.cell-value {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  margin-top: 2px;
}

.chain-bar :deep(.ant-btn-link) {
  color: #f8c000;
}
</style>
