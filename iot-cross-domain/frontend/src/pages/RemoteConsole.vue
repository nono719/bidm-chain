<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import {
  ReloadOutlined,
  PoweroffOutlined,
  SettingOutlined,
  CloudUploadOutlined,
  ProfileOutlined,
  AreaChartOutlined,
  AuditOutlined,
  StopOutlined
} from '@ant-design/icons-vue'
import { apiRequest, getBaseURL, getToken } from '../api/client'

const route = useRoute()
const router = useRouter()

const deviceDid = ref(route.query.deviceDid || '')
const targetDomain = ref(route.query.targetDomain || '')

const session = ref(null)
const sessionLoading = ref(false)
const ttlSec = ref(0)
let ttlTimer = null

const profile = ref(null)
const telemetry = ref([])
const auditTrail = ref([])
const opHistory = ref([])
const lastReceipt = ref(null)

const tab = ref('read')
const opLoading = ref(false)

const configForm = ref({ samplingInterval: 30, mode: 'normal' })
const firmwareForm = ref({ version: '' })

async function loadSession() {
  if (!deviceDid.value || !targetDomain.value) {
    antdMessage.warning('缺少 deviceDid 或 targetDomain，请从跨域认证页进入')
    return
  }
  sessionLoading.value = true
  try {
    const res = await apiRequest(`/api/cross/active-session?deviceDid=${encodeURIComponent(deviceDid.value)}&targetDomain=${encodeURIComponent(targetDomain.value)}`)
    if (res.code === 0) {
      session.value = res.data
      ttlSec.value = res.data?.ttlSeconds || 0
    }
  } finally {
    sessionLoading.value = false
  }
}

const tokenActive = computed(() => session.value?.active === true && ttlSec.value > 0)

function startTtlTimer() {
  if (ttlTimer) clearInterval(ttlTimer)
  ttlTimer = setInterval(() => {
    if (ttlSec.value > 0) {
      ttlSec.value -= 1
    } else if (session.value?.active) {
      session.value = { ...session.value, active: false, reason: 'ttl expired locally' }
    }
  }, 1000)
}

const ttlFormatted = computed(() => {
  if (ttlSec.value <= 0) return '00:00'
  const m = Math.floor(ttlSec.value / 60)
  const s = ttlSec.value % 60
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})

const ttlPct = computed(() => {
  if (!session.value?.expiresAt || !session.value?.verifiedAt) return 100
  const total = (new Date(session.value.expiresAt) - new Date(session.value.verifiedAt)) / 1000
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((ttlSec.value / total) * 100)))
})

async function callRemote(path, method = 'GET', body) {
  if (!tokenActive.value) {
    antdMessage.error('AuthToken 不可用，请重新发起跨域认证')
    return null
  }
  const url = `${getBaseURL()}${path}`
  // HTTP headers must be ISO-8859-1; URL-encode so Chinese values pass through.
  const headers = {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${getToken()}`,
    'X-Device-DID': encodeURIComponent(deviceDid.value),
    'X-Target-Domain': encodeURIComponent(targetDomain.value)
  }
  const r = await fetch(url, { method, headers, body: body ? JSON.stringify(body) : undefined })
  return await r.json().catch(() => null)
}

async function readProfile() {
  opLoading.value = true
  try {
    const res = await callRemote('/api/operations/remote/profile')
    if (res?.code === 0) {
      profile.value = res.data
      antdMessage.success('已读取设备档案')
    } else {
      antdMessage.error(res?.message || '读取失败')
    }
  } finally {
    opLoading.value = false
  }
}

async function readTelemetry() {
  opLoading.value = true
  try {
    const res = await callRemote('/api/operations/remote/telemetry?limit=20')
    if (res?.code === 0) {
      telemetry.value = res.data?.items || []
      antdMessage.success(`已读取 ${res.data?.count || 0} 条遥测`)
    }
  } finally {
    opLoading.value = false
  }
}

async function readAudit() {
  opLoading.value = true
  try {
    const res = await callRemote('/api/operations/remote/audit-trail?limit=30')
    if (res?.code === 0) {
      auditTrail.value = res.data?.items || []
      antdMessage.success(`已读取 ${res.data?.count || 0} 条链上记录`)
    }
  } finally {
    opLoading.value = false
  }
}

async function executeOp(operation, payload) {
  opLoading.value = true
  try {
    const body = {
      deviceDid: deviceDid.value,
      domainCode: targetDomain.value,
      operation,
      payload: typeof payload === 'string' ? payload : JSON.stringify(payload || {})
    }
    const res = await callRemote('/api/operations/protected', 'POST', body)
    if (res?.code === 0) {
      lastReceipt.value = res.data
      opHistory.value.unshift({ ...res.data, runAt: Date.now() })
      antdMessage.success(`✓ ${operation} 已执行${res.data?.chain?.txHash ? '并上链' : ''}`)
      // Auto-refresh device view
      await Promise.all([readProfile(), readAudit()])
    } else {
      antdMessage.error(res?.message || '执行失败')
    }
  } finally {
    opLoading.value = false
  }
}

async function revokeToken() {
  if (!session.value?.requestId) return
  const res = await apiRequest('/api/cross/revoke', { method: 'POST', body: { requestId: session.value.requestId, reason: '远程运维台手动撤销' } })
  if (res.code === 0) {
    antdMessage.warning('AuthToken 已撤销，操作权限已收回')
    if (res.data?.chain) lastReceipt.value = { chain: res.data.chain, operation: 'TOKEN_REVOKE' }
    await loadSession()
  } else {
    antdMessage.error(res.message || '撤销失败')
  }
}

function fmtTime(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}
function shortHash(v) {
  if (!v) return '-'
  return `${v.slice(0, 10)}...${v.slice(-8)}`
}

function bizColor(t) {
  if (t === 'cross_auth') return 'blue'
  if (t === 'device_register') return 'green'
  if (t === 'oracle_report') return 'orange'
  if (t === 'protected_op') return 'magenta'
  if (t === 'token_revoke') return 'red'
  return 'default'
}

function runtimeColor(v) {
  if (v === 'TRUSTED') return 'green'
  if (v === 'RESTARTING') return 'orange'
  if (v === 'UPGRADING') return 'red'
  if (v === 'UNKNOWN') return 'default'
  return 'blue'
}

const protocolSteps = computed(() => {
  const ok = tokenActive.value
  return [
    { key: 'src', label: `源域 ${session.value?.fromDomain || '-'}`, ok },
    { key: 'gate', label: '跨域门禁校验', ok },
    { key: 'policy', label: `信任策略 ${session.value?.permission || ''}/${session.value?.resource || ''}`, ok },
    { key: 'chain', label: '联盟链锚定', ok },
    { key: 'tgt', label: `目标域 ${session.value?.toDomain || '-'}`, ok }
  ]
})

onMounted(async () => {
  await loadSession()
  startTtlTimer()
  if (tokenActive.value) {
    await Promise.all([readProfile(), readAudit()])
  }
})

onUnmounted(() => {
  if (ttlTimer) clearInterval(ttlTimer)
})

const telemetryColumns = [
  { title: '时间', dataIndex: 'createdAt', key: 'createdAt', width: 160 },
  { title: '状态', dataIndex: 'stateLabel', key: 'stateLabel', width: 100 },
  { title: '在线', dataIndex: 'online', key: 'online', width: 60 },
  { title: '评分', dataIndex: 'score', key: 'score', width: 60 },
  { title: '消息', dataIndex: 'message', key: 'message' },
  { title: 'TxHash', dataIndex: 'txHash', key: 'txHash', width: 150 }
]

const auditColumns = [
  { title: '区块', dataIndex: 'blockHeight', key: 'blockHeight', width: 70 },
  { title: '业务', dataIndex: 'bizType', key: 'bizType', width: 130 },
  { title: 'TxHash', dataIndex: 'txHash', key: 'txHash', width: 170 },
  { title: '时间', dataIndex: 'createdAt', key: 'createdAt', width: 160 }
]

const historyColumns = [
  { title: '操作', dataIndex: 'operation', key: 'operation', width: 200 },
  { title: '链上回执', key: 'chain' },
  { title: '时间', dataIndex: 'runAt', key: 'runAt', width: 80 }
]
</script>

<template>
  <a-row :gutter="14" class="rc-root">
    <!-- LEFT: token + protocol + last receipt -->
    <a-col :xs="24" :lg="6" class="rc-col">
      <a-card class="rc-card token-card" :loading="sessionLoading">
        <template #title>
          <span>AuthToken 通行证</span>
        </template>
        <template #extra>
          <a-tag v-if="tokenActive" color="green">VERIFIED</a-tag>
          <a-tag v-else color="red">{{ session?.reason || 'INACTIVE' }}</a-tag>
        </template>
        <div v-if="tokenActive">
          <div class="ttl-block">
            <div class="ttl-label">剩余有效期</div>
            <div class="ttl-value mono">{{ ttlFormatted }}</div>
            <a-progress :percent="ttlPct" :showInfo="false" :strokeColor="ttlPct > 30 ? '#16a34a' : '#dc2626'" size="small" />
          </div>
          <a-descriptions :column="1" size="small" bordered class="token-desc">
            <a-descriptions-item label="源域">{{ session?.fromDomain }}</a-descriptions-item>
            <a-descriptions-item label="目标域">{{ session?.toDomain }}</a-descriptions-item>
            <a-descriptions-item label="设备 DID"><span class="mono">{{ session?.deviceDid }}</span></a-descriptions-item>
            <a-descriptions-item label="权限">
              <a-tag color="blue">{{ session?.permission || '-' }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="资源">
              <a-tag color="cyan">{{ session?.resource || '*' }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="审批者">{{ session?.approvedBy || '-' }}</a-descriptions-item>
            <a-descriptions-item label="过期时间" :contentStyle="{fontSize:'11px'}">{{ fmtTime(session?.expiresAt) }}</a-descriptions-item>
          </a-descriptions>
          <a-button danger block size="small" style="margin-top: 8px" @click="revokeToken"><stop-outlined /> 撤销并上链</a-button>
        </div>
        <a-empty v-else description="无有效跨域凭证">
          <a-button type="primary" @click="router.push('/auth/cross-domain')">前往发起跨域认证</a-button>
        </a-empty>
      </a-card>

      <a-card class="rc-card protocol-card" title="协议链（每个节点已通过）">
        <div class="protocol-strip">
          <template v-for="(s, i) in protocolSteps" :key="s.key">
            <div class="protocol-node" :class="{ 'protocol-ok': s.ok }">
              <div class="dot"></div>
              <div class="protocol-label">{{ s.label }}</div>
            </div>
            <div v-if="i < protocolSteps.length - 1" class="protocol-arrow" :class="{ 'protocol-ok': s.ok }">→</div>
          </template>
        </div>
      </a-card>

      <a-card class="rc-card receipt-card" title="上次操作链上回执">
        <a-empty v-if="!lastReceipt?.chain" description="尚未发起任何写操作" />
        <div v-else>
          <div class="section-line">
            <span class="section-label">操作</span>
            <a-tag color="magenta">{{ lastReceipt.operation }}</a-tag>
          </div>
          <div class="section-line">
            <span class="section-label">区块</span>
            <span class="mono">#{{ lastReceipt.chain.blockHeight ?? '-' }}</span>
          </div>
          <div class="section-line">
            <span class="section-label">TxHash</span>
            <span class="mono small">{{ shortHash(lastReceipt.chain.txHash) }}</span>
          </div>
          <div v-if="lastReceipt.chain.endorsers?.length" class="endorsers-mini">
            <a-tag v-for="(e, i) in lastReceipt.chain.endorsers" :key="i" :color="e.mspId === 'Org1MSP' ? 'green' : (e.mspId === 'Org2MSP' ? 'cyan' : 'blue')" style="margin-bottom: 4px">
              ✓ {{ e.mspId }}
            </a-tag>
          </div>
        </div>
      </a-card>
    </a-col>

    <!-- MIDDLE: operations panel -->
    <a-col :xs="24" :lg="9" class="rc-col">
      <a-card class="rc-card ops-card" title="操作面板">
        <a-tabs v-model:activeKey="tab">
          <a-tab-pane key="read" tab="读取目标域数据">
            <div class="ops-grid">
              <div class="ops-row" @click="readProfile">
                <profile-outlined class="ops-icon" />
                <div class="ops-text">
                  <div class="ops-title">读设备档案</div>
                  <div class="ops-desc">从目标域读 device 表 + lifecycle + 最新 oracle 状态</div>
                </div>
                <a-tag color="blue">READ profile</a-tag>
              </div>
              <div class="ops-row" @click="readTelemetry">
                <area-chart-outlined class="ops-icon" />
                <div class="ops-text">
                  <div class="ops-title">读遥测数据</div>
                  <div class="ops-desc">device_state_updates 最新 20 条（带链上 TxHash 校验）</div>
                </div>
                <a-tag color="blue">READ telemetry</a-tag>
              </div>
              <div class="ops-row" @click="readAudit">
                <audit-outlined class="ops-icon" />
                <div class="ops-text">
                  <div class="ops-title">读链上审计轨迹</div>
                  <div class="ops-desc">该设备在 Fabric 上的所有锚定记录（按区块倒序）</div>
                </div>
                <a-tag color="blue">READ chain</a-tag>
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="write" tab="对目标设备执行操作">
            <a-alert
              type="warning"
              showIcon
              style="margin-bottom: 10px"
              message="所有写操作都会：① 改目标域设备真实状态 ② 通过 anchorcc 链码上链留证 ③ 返回 Org1+Org2 双方背书"
            />
            <div class="ops-grid">
              <div class="ops-row write-row" @click="executeOp('RESTART_DEVICE', { resource: 'control', action: 'restart' })">
                <poweroff-outlined class="ops-icon" />
                <div class="ops-text">
                  <div class="ops-title">重启设备</div>
                  <div class="ops-desc">RuntimeState → RESTARTING 5s → TRUSTED</div>
                </div>
                <a-tag color="orange">WRITE control</a-tag>
              </div>

              <div class="ops-row write-row vertical">
                <div style="display:flex;align-items:center;gap:10px;width:100%">
                  <setting-outlined class="ops-icon" />
                  <div class="ops-text">
                    <div class="ops-title">下发设备配置</div>
                    <div class="ops-desc">将 patch 合并进 device.MetadataJSON</div>
                  </div>
                  <a-tag color="orange">WRITE config</a-tag>
                </div>
                <div style="display:flex;gap:6px;margin-top:8px;width:100%;flex-wrap:wrap">
                  <a-input-number v-model:value="configForm.samplingInterval" addon-before="采样" addon-after="秒" :min="1" :max="600" />
                  <a-select v-model:value="configForm.mode" style="width: 130px">
                    <a-select-option value="normal">normal</a-select-option>
                    <a-select-option value="lowpower">low-power</a-select-option>
                    <a-select-option value="precision">precision</a-select-option>
                  </a-select>
                  <a-button type="primary" @click="executeOp('WRITE_DEVICE_CONFIG', { resource: 'config', patch: configForm })">下发</a-button>
                </div>
              </div>

              <div class="ops-row write-row vertical">
                <div style="display:flex;align-items:center;gap:10px;width:100%">
                  <cloud-upload-outlined class="ops-icon" />
                  <div class="ops-text">
                    <div class="ops-title">固件升级</div>
                    <div class="ops-desc">RuntimeState → UPGRADING 10s → TRUSTED；自动 bump 版本</div>
                  </div>
                  <a-tag color="red">ADMIN firmware</a-tag>
                </div>
                <div style="display:flex;gap:6px;margin-top:8px;width:100%">
                  <a-input v-model:value="firmwareForm.version" placeholder="可留空（自动 bump 版本）" />
                  <a-button danger @click="executeOp('ADMIN_FIRMWARE_UPGRADE', { resource: 'firmware', firmware: firmwareForm.version || undefined })">升级</a-button>
                </div>
              </div>
            </div>
          </a-tab-pane>

          <a-tab-pane key="history" :tab="`本会话历史 (${opHistory.length})`">
            <a-empty v-if="!opHistory.length" description="尚未执行操作" />
            <a-table v-else :columns="historyColumns" :dataSource="opHistory" :pagination="false" size="small" rowKey="id">
              <template #bodyCell="{ column, record }">
                <template v-if="column.key === 'operation'">
                  <a-tag color="magenta">{{ record.operation }}</a-tag>
                  <span style="margin-left:6px">{{ record.effect?.description || '' }}</span>
                </template>
                <template v-else-if="column.key === 'chain'">
                  <div v-if="record.chain?.txHash">
                    <span class="mono">#{{ record.chain.blockHeight }} {{ shortHash(record.chain.txHash) }}</span>
                    <a-tag v-for="(e, i) in record.chain.endorsers || []" :key="i" :color="e.mspId === 'Org1MSP' ? 'green' : 'cyan'" style="margin-left:4px">{{ e.mspId }}</a-tag>
                  </div>
                  <span v-else style="color:#94a3b8">未上链</span>
                </template>
                <template v-else-if="column.key === 'runAt'">
                  {{ dayjs(record.runAt).format('HH:mm:ss') }}
                </template>
              </template>
            </a-table>
          </a-tab-pane>
        </a-tabs>
      </a-card>
    </a-col>

    <!-- RIGHT: target device view -->
    <a-col :xs="24" :lg="9" class="rc-col">
      <a-card class="rc-card device-card" title="目标域设备实时视图">
        <template #extra>
          <a-button type="link" size="small" :loading="opLoading" @click="readProfile"><reload-outlined /> 刷新</a-button>
        </template>
        <a-empty v-if="!profile" description="点击「读设备档案」获取" />
        <div v-else>
          <a-descriptions size="small" :column="2" bordered>
            <a-descriptions-item label="DID" :span="2"><span class="mono">{{ profile.device?.deviceDid }}</span></a-descriptions-item>
            <a-descriptions-item label="名称">{{ profile.device?.displayName }}</a-descriptions-item>
            <a-descriptions-item label="类型">{{ profile.device?.deviceType }}</a-descriptions-item>
            <a-descriptions-item label="所属域">
              <a-tag color="geekblue">{{ profile.device?.domainCode }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="生命周期">
              <a-tag :color="profile.device?.lifecycle === 'ACTIVE' ? 'green' : 'orange'">{{ profile.device?.lifecycle }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="运行态" :span="2">
              <a-tag :color="runtimeColor(profile.device?.runtimeState)" style="font-weight:600;font-size:13px">{{ profile.device?.runtimeState || 'UNKNOWN' }}</a-tag>
              <span v-if="['RESTARTING','UPGRADING'].includes(profile.device?.runtimeState)" class="state-anim"></span>
            </a-descriptions-item>
            <a-descriptions-item label="链上锚定数">{{ profile.anchorCount }}</a-descriptions-item>
            <a-descriptions-item label="最新评分">{{ profile.lastState?.score ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="Metadata" :span="2">
              <pre class="meta-pre">{{ profile.device?.metadataJson || '{}' }}</pre>
            </a-descriptions-item>
          </a-descriptions>
        </div>
      </a-card>

      <a-card class="rc-card" title="遥测数据" v-if="telemetry.length">
        <a-table :columns="telemetryColumns" :dataSource="telemetry" :pagination="{ pageSize: 5 }" size="small" rowKey="id">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'createdAt'">{{ fmtTime(record.createdAt) }}</template>
            <template v-else-if="column.key === 'online'">
              <a-tag :color="record.online ? 'green' : 'red'">{{ record.online ? 'ON' : 'OFF' }}</a-tag>
            </template>
            <template v-else-if="column.key === 'stateLabel'">
              <a-tag :color="record.stateLabel === 'TRUSTED' ? 'green' : 'orange'">{{ record.stateLabel }}</a-tag>
            </template>
            <template v-else-if="column.key === 'txHash'">
              <span class="mono">{{ shortHash(record.txHash) }}</span>
            </template>
          </template>
        </a-table>
      </a-card>

      <a-card class="rc-card" title="链上审计轨迹">
        <template #extra>
          <a-button type="link" size="small" @click="readAudit"><reload-outlined /></a-button>
        </template>
        <a-empty v-if="!auditTrail.length" description="点击「读链上审计轨迹」加载" />
        <a-table v-else :columns="auditColumns" :dataSource="auditTrail" :pagination="{ pageSize: 6 }" size="small" rowKey="id">
          <template #bodyCell="{ column, record }">
            <template v-if="column.key === 'bizType'">
              <a-tag :color="bizColor(record.bizType)">{{ record.bizType }}</a-tag>
            </template>
            <template v-else-if="column.key === 'txHash'">
              <span class="mono">{{ shortHash(record.txHash) }}</span>
            </template>
            <template v-else-if="column.key === 'createdAt'">{{ fmtTime(record.createdAt) }}</template>
          </template>
        </a-table>
      </a-card>
    </a-col>
  </a-row>
</template>

<style scoped>
.rc-root {
  height: 100%;
}

.rc-col {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.rc-card {
  border-radius: 12px;
}

.token-card {
  background: linear-gradient(180deg, #ffffff 0%, #eef2ff 100%);
}

.ttl-block {
  margin-bottom: 8px;
}

.ttl-label {
  font-size: 11px;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.4px;
}

.ttl-value {
  font-size: 28px;
  font-weight: 700;
  color: #182078;
  letter-spacing: 1px;
  margin: 2px 0 6px;
}

.token-desc :deep(.ant-descriptions-item-label) {
  width: 88px;
}

.protocol-strip {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.protocol-node {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  border-radius: 8px;
  background: #f1f5f9;
}

.protocol-node .dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: #cbd5e1;
}

.protocol-node.protocol-ok {
  background: #dcfce7;
}

.protocol-node.protocol-ok .dot {
  background: #16a34a;
}

.protocol-label {
  font-size: 12px;
  color: #1e293b;
}

.protocol-arrow {
  color: #cbd5e1;
  text-align: center;
  font-weight: 700;
  margin-left: 14px;
}

.protocol-arrow.protocol-ok {
  color: #16a34a;
}

.section-line {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.section-label {
  width: 60px;
  color: #64748b;
  font-size: 11px;
}

.endorsers-mini {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.ops-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.ops-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 12px;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #fafbff;
  cursor: pointer;
  transition: all 0.15s;
}

.ops-row:hover {
  background: #eef2ff;
  border-color: #a5b4fc;
}

.ops-row.write-row {
  background: #fff7ed;
  border-color: #fed7aa;
}

.ops-row.write-row:hover {
  background: #ffedd5;
}

.ops-row.vertical {
  flex-direction: column;
  align-items: flex-start;
  cursor: default;
}

.ops-icon {
  font-size: 22px;
  color: #4f46e5;
}

.ops-row.write-row .ops-icon {
  color: #ea580c;
}

.ops-text {
  flex: 1;
}

.ops-title {
  font-weight: 600;
  font-size: 13px;
  color: #1e293b;
}

.ops-desc {
  font-size: 11px;
  color: #64748b;
  margin-top: 2px;
}

.device-card :deep(pre.meta-pre) {
  background: #f8fafc;
  padding: 8px;
  border-radius: 6px;
  font-size: 11px;
  color: #1e293b;
  white-space: pre-wrap;
  word-break: break-all;
  margin: 0;
}

.state-anim {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: #f59e0b;
  margin-left: 8px;
  animation: pulse 1s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.2); }
}

.mono {
  font-family: var(--mono);
  color: #344054;
  word-break: break-all;
}
.mono.small { font-size: 11px; }
</style>
