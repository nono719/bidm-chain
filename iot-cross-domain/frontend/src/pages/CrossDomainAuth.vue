<script setup>
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import { apiRequest, getUser } from '../api/client'

const loading = ref(false)

const form = reactive({
  deviceDid: '',
  toDomainCode: 'domain-b',
  resource: 'profile',
  permission: 'READ',
  ttlSeconds: 1800,
  credential: 'dev-secret-001'
})

const requestId = ref('')
const challenge = ref('')
const signature = ref('')

const status = ref('')
const statusDetail = ref(null)

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const tokenTtlText = computed(() => {
  const exp = token.value?.expiresAt || receipt.value?.expiresAt || statusDetail.value?.expiresAt
  if (!exp) return '约 30 分钟'
  const diff = (new Date(exp) - new Date()) / 1000
  if (diff <= 0) return '已过期'
  const m = Math.floor(diff / 60)
  const s = Math.floor(diff % 60)
  return `${m} 分 ${s} 秒`
})
const deviceLoading = ref(false)
const deviceOptions = ref([])
const deviceMap = ref({})
const historyLoading = ref(false)
const historyOptions = ref([])
const historyMap = ref({})
const domainLoading = ref(false)
const domainOptions = ref([])
const statusRefreshing = ref(false)
const pollTimer = ref(null)
const pollingBusy = ref(false)

const token = ref(null)
const receipt = ref(null)
const oracleStatus = ref(null)

const stepState = ref({ submit: 'wait', resolve: 'wait', policy: 'wait', state: 'wait', issue: 'wait' })

function resetFlow() {
  requestId.value = ''
  challenge.value = ''
  signature.value = ''
  token.value = null
  receipt.value = null
  status.value = ''
  statusDetail.value = null
  stepState.value = { submit: 'wait', resolve: 'wait', policy: 'wait', state: 'wait', issue: 'wait' }
}

const timelineItems = computed(() => {
  const mapToColor = (s) => {
    if (s === 'finish') return 'green'
    if (s === 'process') return 'blue'
    if (s === 'error') return 'red'
    return 'gray'
  }
  return [
    { key: 'submit', label: '提交认证请求', color: mapToColor(stepState.value.submit) },
    { key: 'resolve', label: 'DID解析与合法性校验', color: mapToColor(stepState.value.resolve) },
    { key: 'policy', label: '域间信任策略校验', color: mapToColor(stepState.value.policy) },
    { key: 'state', label: '链上状态读取与新鲜度校验', color: mapToColor(stepState.value.state) },
    { key: 'issue', label: '签发 AuthToken 并写链锚定', color: mapToColor(stepState.value.issue) }
  ]
})

const selectedOperationMeta = computed(() => {
  const op = protectedForm.operation
  return protectedCatalog.value.find((it) => it.operation === op) || null
})

async function doRequest() {
  if (!form.deviceDid) {
    antdMessage.warning('请先选择请求设备DID')
    return
  }
  resetFlow()
  loading.value = true
  stepState.value.submit = 'process'
  try {
    const res = await apiRequest('/api/cross/request', {
      method: 'POST',
      body: {
        deviceDid: form.deviceDid,
        toDomainCode: form.toDomainCode,
        resource: form.resource,
        permission: form.permission,
        ttlSeconds: form.ttlSeconds
      }
    })
    if (res.code !== 0) {
      stepState.value.submit = 'error'
      antdMessage.error(res.message || '请求失败')
      return
    }
    requestId.value = res.data.requestId
    challenge.value = res.data.challenge
    status.value = 'PENDING_SIGNATURE'
    stepState.value.submit = 'finish'
    stepState.value.resolve = 'finish'
    stepState.value.policy = 'finish'
    stepState.value.state = 'process'
    antdMessage.success('已生成挑战值，请签名后提交验签')
    await loadHistory()
  } finally {
    loading.value = false
  }
}

async function autoSign() {
  if (!challenge.value) {
    antdMessage.warning('请先发起认证请求')
    return
  }
  const raw = `${challenge.value}:${form.credential}`
  const encoder = new TextEncoder()
  const data = encoder.encode(raw)
  const hashBuffer = await crypto.subtle.digest('SHA-256', data)
  const hashArray = Array.from(new Uint8Array(hashBuffer))
  signature.value = hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  antdMessage.success('签名已生成')
}

async function doVerify() {
  if (!requestId.value || !signature.value) {
    antdMessage.warning('请填写请求ID与签名')
    return
  }
  loading.value = true
  try {
    const res = await apiRequest('/api/cross/verify', {
      method: 'POST',
      body: { requestId: requestId.value, signature: signature.value }
    })
    if (res.code !== 0) {
      stepState.value.issue = 'error'
      stepState.value.state = 'error'
      antdMessage.error(res.message || '验签失败')
      return
    }
    status.value = res.data.status
    stepState.value.state = 'finish'
    if (res.data.status === 'VERIFIED') {
      stepState.value.issue = 'finish'
      antdMessage.success('签名校验通过，已直接签发认证令牌')
    } else {
      stepState.value.issue = 'process'
      antdMessage.success('签名校验通过，等待管理员审批')
    }
    await refreshStatus()
    await loadHistory()
  } finally {
    loading.value = false
  }
}

async function refreshStatus() {
  if (!requestId.value || statusRefreshing.value) return
  statusRefreshing.value = true
  const res = await apiRequest(`/api/cross/status/${encodeURIComponent(requestId.value)}`)
  try {
    if (res.code !== 0) {
      return
    }
    statusDetail.value = res.data
    status.value = res.data.status
    oracleStatus.value = {
      allowed: !!res.data.oracleAllowed,
      reason: res.data.oracleReason || '',
      detail: res.data.oracle || null
    }
    if (res.data.status === 'VERIFIED') {
      stepState.value.issue = 'finish'
      receipt.value = {
        requestId: res.data.requestId,
        status: res.data.status,
        verifiedAt: res.data.verifiedAt,
        expiresAt: res.data.expiresAt,
        txHash: res.data.txHash,
        blockHeight: res.data.blockHeight
      }
      token.value = {
        subject: res.data.deviceDid,
        srcDomain: res.data.fromDomainCode,
        dstDomain: res.data.toDomainCode,
        resource: res.data.resource,
        permission: res.data.permission,
        issuedAt: res.data.verifiedAt,
        expiresAt: res.data.expiresAt,
        requestId: res.data.requestId,
        approvedBy: res.data.approvedBy
      }
      await loadProtectedCatalog()
      await loadProtectedHistory()
    }
    if (res.data.status === 'REJECTED') {
      stepState.value.issue = 'error'
      protectedCatalog.value = []
      protectedHistory.value = []
    }
  } finally {
    statusRefreshing.value = false
  }
}

async function loadHistory() {
  historyLoading.value = true
  try {
    const res = await apiRequest('/api/cross/history')
    if (res.code !== 0) return
    const rows = Array.isArray(res.data) ? res.data : []
    const map = {}
    historyOptions.value = rows.map((r) => {
      map[r.requestId] = r
      return {
        value: r.requestId,
        label: `${r.requestId}｜${r.status}｜${r.deviceDid}`
      }
    })
    historyMap.value = map
  } finally {
    historyLoading.value = false
  }
}

function stopPolling() {
  if (pollTimer.value) {
    clearTimeout(pollTimer.value)
    pollTimer.value = null
  }
}

function shouldPoll() {
  return true
}

function syncPolling() {
  if (!shouldPoll()) {
    stopPolling()
    return
  }
  if (pollTimer.value || pollingBusy.value) return
  pollTimer.value = setTimeout(async () => {
    pollTimer.value = null
    if (pollingBusy.value) return
    pollingBusy.value = true
    try {
      if (requestId.value) {
        await refreshStatus()
      }
      await loadHistory()
    } finally {
      pollingBusy.value = false
    }
    syncPolling()
  }, 3000)
}

function applyHistoryRequest(v) {
  const row = historyMap.value[v]
  if (!row) return
  requestId.value = row.requestId
  form.deviceDid = row.deviceDid || form.deviceDid
  form.toDomainCode = row.toDomainCode || form.toDomainCode
  form.resource = row.resource || form.resource
  form.permission = row.permission || form.permission
  form.ttlSeconds = row.ttlSeconds || form.ttlSeconds
  status.value = row.status || ''
  refreshStatus()
}

async function approveNow(approve) {
  const res = await apiRequest('/api/cross/approve', {
    method: 'POST',
    body: { requestId: requestId.value, approve }
  })
  if (res.code !== 0) {
    antdMessage.error(res.message || '审批失败')
    return
  }
  if (approve) antdMessage.success('已审批通过并签发')
  else antdMessage.info('已拒绝')
  await refreshStatus()
  await loadHistory()
}

async function revokeNow() {
  if (!requestId.value) return
  const res = await apiRequest('/api/cross/revoke', {
    method: 'POST',
    body: { requestId: requestId.value, reason: '管理员手动撤销' }
  })
  if (res.code !== 0) {
    antdMessage.error(res.message || '撤销失败')
    return
  }
  antdMessage.success('已撤销该认证凭证')
  await refreshStatus()
  await loadHistory()
}

const protectedLoading = ref(false)
const protectedResp = ref(null)
const protectedCatalogLoading = ref(false)
const protectedCatalog = ref([])
const protectedHistoryLoading = ref(false)
const protectedHistory = ref([])
const protectedForm = reactive({
  operation: '',
  payload: '{}'
})

async function runProtected() {
  if (!form.deviceDid || !form.toDomainCode) {
    antdMessage.warning('请先选择设备和目标域')
    return
  }
  if (!protectedForm.operation) {
    antdMessage.warning('请先选择可执行操作')
    return
  }
  protectedLoading.value = true
  try {
    const res = await apiRequest('/api/operations/protected', {
      method: 'POST',
      headers: { 'X-Device-DID': encodeURIComponent(form.deviceDid), 'X-Target-Domain': encodeURIComponent(form.toDomainCode) },
      body: {
        deviceDid: form.deviceDid,
        domainCode: form.toDomainCode,
        operation: protectedForm.operation,
        payload: protectedForm.payload
      }
    })
    protectedResp.value = res
    if (res.code === 0) {
      antdMessage.success('受保护操作执行成功')
      await loadProtectedHistory()
    } else {
      antdMessage.error(res.message || '受保护操作失败')
    }
  } finally {
    protectedLoading.value = false
  }
}

async function loadProtectedCatalog() {
  if (!form.deviceDid || !form.toDomainCode || status.value !== 'VERIFIED') {
    protectedCatalog.value = []
    protectedForm.operation = ''
    protectedForm.payload = '{}'
    return
  }
  protectedCatalogLoading.value = true
  try {
    const qs = new URLSearchParams({
      deviceDid: form.deviceDid,
      targetDomain: form.toDomainCode
    }).toString()
    const res = await apiRequest(`/api/operations/catalog?${qs}`)
    if (res.code !== 0) {
      protectedCatalog.value = []
      protectedForm.operation = ''
      return
    }
    const items = Array.isArray(res.data?.items) ? res.data.items : []
    protectedCatalog.value = items
    if (!items.some((it) => it.operation === protectedForm.operation)) {
      protectedForm.operation = items[0]?.operation || ''
      protectedForm.payload = items[0]?.payloadTemplate || '{}'
    }
  } finally {
    protectedCatalogLoading.value = false
  }
}

async function loadProtectedHistory() {
  if (!form.deviceDid || !form.toDomainCode || status.value !== 'VERIFIED') {
    protectedHistory.value = []
    return
  }
  protectedHistoryLoading.value = true
  try {
    const qs = new URLSearchParams({
      deviceDid: form.deviceDid,
      targetDomain: form.toDomainCode,
      limit: '8'
    }).toString()
    const res = await apiRequest(`/api/operations/history?${qs}`)
    if (res.code !== 0) {
      protectedHistory.value = []
      return
    }
    protectedHistory.value = Array.isArray(res.data) ? res.data : []
  } finally {
    protectedHistoryLoading.value = false
  }
}

function fmt(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}

async function loadDeviceOptions() {
  deviceLoading.value = true
  try {
    const res = await apiRequest('/api/devices')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载设备列表失败')
      return
    }
    const rows = Array.isArray(res.data) ? res.data : []
    const map = {}
    deviceOptions.value = rows.map((d) => {
      map[d.deviceDid] = d
      return {
        value: d.deviceDid,
        label: `${d.displayName || '未命名设备'}（${d.domainCode || '-'}）`,
        did: d.deviceDid
      }
    })
    deviceMap.value = map
    if (!form.deviceDid && deviceOptions.value.length > 0) {
      form.deviceDid = deviceOptions.value[0].value
    }
  } finally {
    deviceLoading.value = false
  }
}

async function refreshOracleCheck() {
  if (!form.deviceDid) {
    oracleStatus.value = null
    return
  }
  const res = await apiRequest(`/api/oracle/check/${encodeURIComponent(form.deviceDid)}`)
  if (res.code !== 0) {
    oracleStatus.value = null
    return
  }
  oracleStatus.value = res.data
}

async function runOracleReportNow() {
  if (!form.deviceDid) {
    antdMessage.warning('请先选择设备')
    return
  }
  const res = await apiRequest(`/api/oracle/report/${encodeURIComponent(form.deviceDid)}`, { method: 'POST' })
  if (res.code !== 0) {
    antdMessage.error(res.message || '预言机上报失败')
    return
  }
  antdMessage.success('预言机状态已更新')
  await refreshOracleCheck()
}

async function loadDomains() {
  domainLoading.value = true
  try {
    const res = await apiRequest('/api/domains')
    if (res.code !== 0) return
    const rows = Array.isArray(res.data) ? res.data : []
    domainOptions.value = rows.map((d) => ({ value: d.code, label: `${d.name}（${d.code}）` }))
    if (domainOptions.value.length > 0 && !domainOptions.value.some((o) => o.value === form.toDomainCode)) {
      form.toDomainCode = domainOptions.value[0].value
    }
  } finally {
    domainLoading.value = false
  }
}

watch(
  () => form.deviceDid,
  (v) => {
    const d = deviceMap.value[v]
    if (d?.credential) form.credential = d.credential
    refreshOracleCheck()
  },
  { immediate: true }
)

watch(
  () => [status.value, form.deviceDid, form.toDomainCode],
  async ([s]) => {
    if (s === 'VERIFIED') {
      await loadProtectedCatalog()
      await loadProtectedHistory()
      return
    }
    protectedCatalog.value = []
    protectedHistory.value = []
    protectedForm.operation = ''
    protectedForm.payload = '{}'
  }
)

watch(
  () => protectedForm.operation,
  (op) => {
    const selected = protectedCatalog.value.find((it) => it.operation === op)
    if (selected?.payloadTemplate) {
      protectedForm.payload = selected.payloadTemplate
    }
  }
)

onMounted(loadDeviceOptions)
onMounted(loadHistory)
onMounted(loadDomains)
watch(() => [requestId.value, status.value], syncPolling, { immediate: true })
onUnmounted(stopPolling)
</script>

<template>
  <a-card class="panel-card" title="跨域认证（图5-3）">
    <a-row :gutter="16">
      <a-col :xs="24" :lg="10">
        <a-card class="inner-card" title="认证请求构造" size="small">
          <a-form layout="vertical">
            <a-form-item label="历史申请（可回看）">
              <a-space style="width: 100%">
                <a-select
                  :value="requestId || undefined"
                  :options="historyOptions"
                  :loading="historyLoading"
                  style="min-width: 460px"
                  show-search
                  optionFilterProp="label"
                  placeholder="选择已有跨域认证申请"
                  @update:value="applyHistoryRequest"
                />
                <a-button :loading="historyLoading" @click="loadHistory">刷新</a-button>
              </a-space>
            </a-form-item>
            <a-form-item label="请求设备DID">
              <a-select
                v-model:value="form.deviceDid"
                :options="deviceOptions"
                :loading="deviceLoading"
                show-search
                optionFilterProp="label"
                placeholder="请选择已注册设备"
              >
                <template #option="{ data }">
                  <div style="display: flex; flex-direction: column; gap: 2px;">
                    <span>{{ data.label }}</span>
                    <span class="mono" style="font-size: 12px; color: #667085;">{{ data.did }}</span>
                  </div>
                </template>
              </a-select>
            </a-form-item>
            <a-form-item label="目标管理域">
              <a-select v-model:value="form.toDomainCode" :loading="domainLoading" :options="domainOptions" />
            </a-form-item>
            <a-form-item label="访问资源">
              <a-input v-model:value="form.resource" />
            </a-form-item>
            <a-form-item label="申请权限">
              <a-select v-model:value="form.permission" :options="[{value:'READ',label:'READ'},{value:'WRITE',label:'WRITE'},{value:'ADMIN',label:'ADMIN'}]" />
            </a-form-item>
            <a-form-item label="有效期（秒）">
              <a-input-number v-model:value="form.ttlSeconds" :min="60" :max="86400" style="width: 100%" />
            </a-form-item>
            <a-form-item label="设备凭证（用于签名，演示用）">
              <a-input-password v-model:value="form.credential" />
            </a-form-item>
          </a-form>

          <a-space>
            <a-button type="primary" :loading="loading" @click="doRequest">发起跨域认证</a-button>
            <a-button :disabled="!challenge" @click="autoSign">自动签名</a-button>
            <a-button type="primary" :disabled="!requestId || !signature" :loading="loading" @click="doVerify">提交验签</a-button>
          </a-space>

          <a-divider />
          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="请求ID">{{ requestId || '-' }}</a-descriptions-item>
            <a-descriptions-item label="挑战值">{{ challenge || '-' }}</a-descriptions-item>
            <a-descriptions-item label="签名"> <span class="mono">{{ signature || '-' }}</span> </a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="14">
        <a-card class="inner-card" title="认证流程时间线" size="small">
          <a-timeline>
            <a-timeline-item v-for="it in timelineItems" :key="it.key" :color="it.color">
              {{ it.label }}
            </a-timeline-item>
          </a-timeline>
          <a-alert
            style="margin-top: 10px"
            type="info"
            showIcon
            message="认证通过后，/api/operations/protected 将通过跨域门禁（需在请求头携带 X-Device-DID 与 X-Target-Domain）"
          />
          <a-divider />
          <a-space direction="vertical" style="width: 100%">
            <a-alert
              :type="oracleStatus?.allowed ? 'success' : 'warning'"
              showIcon
              :message="oracleStatus ? `预言机校验：${oracleStatus.allowed ? '允许' : '不允许'}` : '预言机校验：未获取'"
              :description="oracleStatus?.reason || '请先选择设备并点击刷新'"
            />
            <a-descriptions v-if="oracleStatus?.detail" size="small" :column="1" bordered>
              <a-descriptions-item label="门限值">{{ oracleStatus.detail.threshold }}</a-descriptions-item>
              <a-descriptions-item label="活跃节点">{{ oracleStatus.detail.activeNodes }}</a-descriptions-item>
              <a-descriptions-item label="状态标签">{{ oracleStatus.detail.stateLabel || '-' }}</a-descriptions-item>
              <a-descriptions-item label="评分">{{ oracleStatus.detail.score ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="最近上报">{{ oracleStatus.detail.lastReportAt ? fmt(oracleStatus.detail.lastReportAt) : '-' }}</a-descriptions-item>
            </a-descriptions>
            <a-space>
              <a-button size="small" @click="refreshOracleCheck">刷新预言机校验</a-button>
              <a-button size="small" type="primary" @click="runOracleReportNow">触发预言机上报</a-button>
            </a-space>
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="16" style="margin-top: 16px">
      <a-col :xs="24" :lg="12">
        <a-card class="inner-card" title="AuthToken" size="small">
          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="主体DID"><span class="mono">{{ token?.subject || '-' }}</span></a-descriptions-item>
            <a-descriptions-item label="源域">{{ token?.srcDomain || '-' }}</a-descriptions-item>
            <a-descriptions-item label="目标域">{{ token?.dstDomain || '-' }}</a-descriptions-item>
            <a-descriptions-item label="资源">{{ token?.resource || '-' }}</a-descriptions-item>
            <a-descriptions-item label="权限">{{ token?.permission || '-' }}</a-descriptions-item>
            <a-descriptions-item label="签发时间">{{ token?.issuedAt ? fmt(token.issuedAt) : '-' }}</a-descriptions-item>
            <a-descriptions-item label="过期时间">{{ token?.expiresAt ? fmt(token.expiresAt) : '-' }}</a-descriptions-item>
            <a-descriptions-item label="审批人">{{ token?.approvedBy || '-' }}</a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="12">
        <a-card class="inner-card" title="链上交易回执" size="small">
          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="状态">{{ receipt?.status || '-' }}</a-descriptions-item>
            <a-descriptions-item label="认证时间">{{ receipt?.verifiedAt ? fmt(receipt.verifiedAt) : '-' }}</a-descriptions-item>
            <a-descriptions-item label="过期时间">{{ receipt?.expiresAt ? fmt(receipt.expiresAt) : '-' }}</a-descriptions-item>
            <a-descriptions-item label="区块高度">{{ receipt?.blockHeight ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="交易哈希"> <span class="mono">{{ receipt?.txHash || '-' }}</span> </a-descriptions-item>
            <a-descriptions-item label="Gas">N/A</a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-col>
    </a-row>

    <a-card class="inner-card" size="small" style="margin-top: 16px" title="审批状态">
      <a-space direction="vertical" style="width: 100%" size="middle">
        <a-descriptions size="small" :column="2" bordered>
          <a-descriptions-item label="当前状态">{{ status || '-' }}</a-descriptions-item>
          <a-descriptions-item label="申请人">{{ statusDetail?.requestedBy || '-' }}</a-descriptions-item>
          <a-descriptions-item label="签名校验">{{ statusDetail?.signatureVerifiedAt ? fmt(statusDetail.signatureVerifiedAt) : '-' }}</a-descriptions-item>
          <a-descriptions-item label="审批时间">{{ statusDetail?.approvedAt ? fmt(statusDetail.approvedAt) : '-' }}</a-descriptions-item>
        </a-descriptions>

        <a-space>
          <a-button :disabled="!requestId" @click="refreshStatus">刷新状态</a-button>
          <a-button v-if="isAdmin && status === 'PENDING_APPROVAL'" type="primary" @click="approveNow(true)">管理员通过</a-button>
          <a-button v-if="isAdmin && status === 'PENDING_APPROVAL'" danger @click="approveNow(false)">管理员拒绝</a-button>
          <a-button v-if="isAdmin && status === 'VERIFIED'" danger @click="revokeNow">撤销凭证</a-button>
        </a-space>

        <a-alert
          v-if="status === 'VERIFIED'"
          type="success"
          showIcon
          style="margin-top: 12px"
          :message="`AuthToken 已发放（剩余 ${tokenTtlText}），可凭此通行证读取目标域数据或执行操作`"
        >
          <template #action>
            <a-button type="primary" size="middle" @click="$router.push({ path: '/remote-console', query: { deviceDid: form.deviceDid, targetDomain: form.toDomainCode } })">
              进入远程运维台 →
            </a-button>
          </template>
        </a-alert>
      </a-space>
    </a-card>

    <a-row :gutter="16" style="margin-top: 16px">
      <a-col :span="24">
        <a-card class="inner-card" title="受保护操作（门禁验证）" size="small">
          <a-space style="width: 100%" direction="vertical" size="middle">
            <a-row :gutter="12">
              <a-col :xs="24" :lg="6">
                <a-select
                  v-model:value="protectedForm.operation"
                  :loading="protectedCatalogLoading"
                  :options="protectedCatalog"
                  :field-names="{ value: 'operation', label: 'label' }"
                  placeholder="选择可执行操作"
                />
              </a-col>
              <a-col :xs="24" :lg="14">
                <a-input v-model:value="protectedForm.payload" placeholder="载荷 JSON" />
              </a-col>
              <a-col :xs="24" :lg="4">
                <a-button type="primary" block :loading="protectedLoading" :disabled="status !== 'VERIFIED'" @click="runProtected">执行</a-button>
              </a-col>
            </a-row>
            <a-alert
              v-if="status !== 'VERIFIED'"
              type="info"
              showIcon
              message="请先完成跨域认证并进入 VERIFIED 状态，再执行受保护操作"
            />
            <a-descriptions v-if="selectedOperationMeta" size="small" :column="1" bordered>
              <a-descriptions-item label="操作说明">{{ selectedOperationMeta.description }}</a-descriptions-item>
              <a-descriptions-item label="所需权限">{{ selectedOperationMeta.requiredPermission }}</a-descriptions-item>
              <a-descriptions-item label="所需资源">{{ selectedOperationMeta.resource }}</a-descriptions-item>
            </a-descriptions>
            <a-alert v-if="protectedResp" :type="protectedResp.code === 0 ? 'success' : 'error'" showIcon :message="protectedResp.code === 0 ? '执行成功' : (protectedResp.message || '执行失败')" />
            <a-descriptions v-if="protectedResp?.data" size="small" :column="1" bordered>
              <a-descriptions-item label="操作类型">{{ protectedResp.data.operation || '-' }}</a-descriptions-item>
              <a-descriptions-item label="设备DID"><span class="mono">{{ protectedResp.data.deviceDid || '-' }}</span></a-descriptions-item>
              <a-descriptions-item label="目标域">{{ protectedResp.data.domainCode || '-' }}</a-descriptions-item>
              <a-descriptions-item label="执行人">{{ protectedResp.data.createdBy || '-' }}</a-descriptions-item>
              <a-descriptions-item label="时间">{{ protectedResp.data.createdAt ? fmt(protectedResp.data.createdAt) : '-' }}</a-descriptions-item>
            </a-descriptions>
            <a-divider style="margin: 8px 0" />
            <a-space style="width: 100%; justify-content: space-between">
              <span>最近操作历史</span>
              <a-button size="small" :loading="protectedHistoryLoading" :disabled="status !== 'VERIFIED'" @click="loadProtectedHistory">刷新历史</a-button>
            </a-space>
            <a-list
              size="small"
              bordered
              :loading="protectedHistoryLoading"
              :data-source="protectedHistory"
              :locale="{ emptyText: '暂无记录' }"
            >
              <template #renderItem="{ item }">
                <a-list-item>
                  <span>{{ fmt(item.createdAt) }}｜{{ item.operation }}｜{{ item.domainCode }}｜{{ item.createdBy }}</span>
                </a-list-item>
              </template>
            </a-list>
          </a-space>
        </a-card>
      </a-col>
    </a-row>
  </a-card>
</template>

<style scoped>
.mono {
  font-family: var(--mono);
  color: #344054;
}
</style>
