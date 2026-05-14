<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts'
import {
  ReloadOutlined,
  PoweroffOutlined,
  SettingOutlined,
  CloudUploadOutlined,
  ProfileOutlined,
  AreaChartOutlined,
  AuditOutlined,
  StopOutlined,
  ApiOutlined,
  VideoCameraOutlined,
  LockOutlined,
  ThunderboltOutlined,
  CarOutlined,
  ControlOutlined,
  HomeOutlined
} from '@ant-design/icons-vue'
import { apiRequest, getBaseURL, getToken } from '../api/client'

const route = useRoute()
const router = useRouter()

const deviceDid = ref(route.query.deviceDid || '')      // SOURCE device (session.deviceDid)
const targetDomain = ref(route.query.targetDomain || '')
const targetDeviceDid = ref('')                          // OPERATION target (a device in targetDomain)
const targetDevices = ref([])                            // devices available in targetDomain

const session = ref(null)
const sessionLoading = ref(false)

const sessionPicker = ref({ visible: false, options: [] })

async function autoPickFromHistory() {
  const res = await apiRequest('/api/cross/history')
  if (res.code !== 0) return false
  const items = (res.data || []).filter((s) => s.status === 'VERIFIED' && s.expiresAt && new Date(s.expiresAt).getTime() > Date.now())
  if (items.length === 0) return false
  if (items.length === 1) {
    deviceDid.value = items[0].deviceDid
    targetDomain.value = items[0].toDomainCode
    return true
  }
  sessionPicker.value = { visible: true, options: items }
  return false
}

function chooseSessionFromPicker(s) {
  deviceDid.value = s.deviceDid
  targetDomain.value = s.toDomainCode
  sessionPicker.value.visible = false
  loadSession().then(() => {
    if (tokenActive.value) {
      Promise.all([readProfile(), readAudit()])
    }
  })
}
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

// ===== Flying packet animation (visualizes cross-domain operations) =====
const flyingPackets = ref([])  // [{id, kind, label, color}]
let packetSeq = 0
function flyPacket(kind, label, color) {
  const id = ++packetSeq
  flyingPackets.value.push({ id, kind, label, color })
  setTimeout(() => {
    flyingPackets.value = flyingPackets.value.filter((p) => p.id !== id)
  }, 1800)
}

// ===== Scenario-aware device profile (based on deviceType) =====
const SCENARIO_MAP = {
  '传感器':       { key: 'sensor',         icon: AreaChartOutlined,    color: '#10b981', name: '温湿度传感器', readLabel: '读取实时遥测', metricLabels: ['温度 (°C)', '湿度 (%)'] },
  'sensor':       { key: 'sensor',         icon: AreaChartOutlined,    color: '#10b981', name: '温湿度传感器', readLabel: '读取实时遥测', metricLabels: ['温度 (°C)', '湿度 (%)'] },
  'SENSOR':       { key: 'sensor',         icon: AreaChartOutlined,    color: '#10b981', name: '温湿度传感器', readLabel: '读取实时遥测', metricLabels: ['温度 (°C)', '湿度 (%)'] },
  '环境监测设备':  { key: 'env',            icon: AreaChartOutlined,    color: '#06b6d4', name: '环境监测', readLabel: '读取空气质量', metricLabels: ['PM2.5 (μg/m³)', 'CO₂ (ppm)'] },
  '执行器':       { key: 'actuator',       icon: ControlOutlined,      color: '#f59e0b', name: '执行器', readLabel: '读取当前状态', metricLabels: ['工作模式', '功率 (%)'] },
  '网关':         { key: 'gateway',        icon: ApiOutlined,          color: '#3b82f6', name: '物联网网关', readLabel: '读取连接设备', metricLabels: ['连接数', '吞吐 (KB/s)'] },
  '摄像头':       { key: 'camera',         icon: VideoCameraOutlined,  color: '#8b5cf6', name: '摄像头', readLabel: '抓拍当前帧', metricLabels: ['帧率', '分辨率'] },
  '门禁设备':     { key: 'access',         icon: LockOutlined,         color: '#ef4444', name: '门禁设备', readLabel: '读取通行记录', metricLabels: ['今日通行', '未授权尝试'] },
  '智能电表':     { key: 'smart-meter',    icon: ThunderboltOutlined,  color: '#f97316', name: '智能电表', readLabel: '读取实时功率', metricLabels: ['功率 (kW)', '今日电量 (kWh)'] },
  '智能家电':     { key: 'appliance',      icon: HomeOutlined,         color: '#22c55e', name: '智能家电', readLabel: '读取运行状态', metricLabels: ['工作模式', '功率 (W)'] },
  '工业控制器':   { key: 'plc',            icon: ControlOutlined,      color: '#0ea5e9', name: 'PLC 控制器', readLabel: '读取寄存器', metricLabels: ['寄存器值', '运行周期 (ms)'] },
  '车载终端':     { key: 'vehicle',        icon: CarOutlined,          color: '#a855f7', name: '车载终端', readLabel: '读取车辆数据', metricLabels: ['车速 (km/h)', '油量 (%)'] }
}
const DEFAULT_SCENARIO = { key: 'device', icon: ProfileOutlined, color: '#64748b', name: '通用设备', readLabel: '读取设备数据', metricLabels: ['数值A', '数值B'] }

// Aliases / fuzzy matches so devices with non-standard type strings still
// pick a sensible scenario template instead of falling back to "unknown".
const SCENARIO_ALIAS = [
  // [matcher (string or regex tested case-insensitively against deviceType), mapped key]
  // Order matters: more-specific patterns first so generic ones don't
  // steal the match (e.g. "air-conditioner" must hit aircon, not /air/).
  [/temp|humid|湿|温/i,                              '传感器'],
  [/sensor/i,                                        '传感器'],
  [/aircon|air-cond|空调|fridge|冰箱|tv|wash|洗衣/i, '智能家电'],
  [/appliance|家电/i,                                '智能家电'],
  [/light|灯|lamp|bulb/i,                            '智能家电'],
  [/(?:^|[^a-z])air(?:[^a-z]|$)|pm2|pm10|环境|空气|aqi?/i, '环境监测设备'],
  [/charger|充电桩|充电/i,                           '智能电表'],
  [/meter|电表|功率|电能/i,                          '智能电表'],
  [/gateway|网关|hub|路由|router/i,                  '网关'],
  [/camera|摄像/i,                                   '摄像头'],
  [/lock|门禁|锁/i,                                  '门禁设备'],
  [/actuator|执行/i,                                 '执行器'],
  [/plc|工业|controller|control/i,                   '工业控制器'],
  [/vehicle|obd|car|车/i,                            '车载终端']
]

function resolveScenarioKey(dt) {
  if (!dt) return null
  if (SCENARIO_MAP[dt]) return dt
  const s = String(dt).trim()
  for (const [pat, mapped] of SCENARIO_ALIAS) {
    if (pat.test(s)) return mapped
  }
  return null
}

const scenario = computed(() => {
  const dt = profile.value?.device?.deviceType || ''
  const key = resolveScenarioKey(dt)
  if (key && SCENARIO_MAP[key]) return SCENARIO_MAP[key]
  return DEFAULT_SCENARIO
})

// Thermometer fill percentage for sensor scenario (maps temperature to 0-100%).
const thermoPct = computed(() => {
  const t = typeof liveMetricA.value === 'number' ? liveMetricA.value : 25
  // Map -10°C..40°C to 0..100%
  return Math.max(5, Math.min(95, Math.round((t + 10) * 2)))
})

// ===== Mock sensor data (driven by real device_state_updates) =====
// Generates "as if streaming" gauge values from the latest oracle score.
const liveMetricA = ref(0)
const liveMetricB = ref(0)
let liveTimer = null

function refreshLiveMetrics() {
  const score = profile.value?.lastState?.score ?? 80
  const sk = scenario.value.key
  // Deterministic-ish: vary metric around a base influenced by score so the
  // gauge moves but stays plausible.
  const jitter = () => (Math.random() - 0.5) * 6
  if (sk === 'sensor' || sk === 'env') {
    liveMetricA.value = +(22 + (score / 100) * 8 + jitter()).toFixed(1)
    liveMetricB.value = +(55 + (score / 100) * 20 + jitter()).toFixed(1)
  } else if (sk === 'smart-meter') {
    liveMetricA.value = +(0.4 + (score / 100) * 2 + jitter() * 0.1).toFixed(2)
    liveMetricB.value = +(50 + (score / 100) * 100 + jitter()).toFixed(1)
  } else if (sk === 'gateway') {
    liveMetricA.value = Math.max(0, Math.round(3 + (score / 100) * 12 + jitter()))
    liveMetricB.value = Math.max(0, +((score / 100) * 200 + jitter()).toFixed(1))
  } else if (sk === 'vehicle') {
    liveMetricA.value = Math.max(0, Math.round(40 + (score / 100) * 60 + jitter()))
    liveMetricB.value = Math.max(0, Math.min(100, Math.round((score / 100) * 90 + jitter())))
  } else if (sk === 'camera') {
    liveMetricA.value = 25 + Math.round(jitter())  // fps
    liveMetricB.value = profile.value?.device?.runtimeState === 'TRUSTED' ? '1080p' : '480p'
  } else if (sk === 'access') {
    liveMetricA.value = Math.max(0, Math.round(12 + (score / 100) * 8 + jitter()))
    liveMetricB.value = Math.max(0, Math.round(jitter() + 1))
  } else if (sk === 'actuator' || sk === 'appliance') {
    liveMetricA.value = score >= 70 ? '运行中' : '待机'
    liveMetricB.value = Math.max(0, Math.round(40 + (score / 100) * 50 + jitter()))
  } else if (sk === 'plc') {
    liveMetricA.value = Math.max(0, Math.round(1024 + (score / 100) * 3072 + jitter() * 20))
    liveMetricB.value = Math.max(1, Math.round(5 + jitter()))
  } else {
    liveMetricA.value = +(score + jitter()).toFixed(1)
    liveMetricB.value = +(50 + jitter()).toFixed(1)
  }
}

// ===== ECharts gauge for the primary metric =====
const gaugeRef = ref(null)
let gaugeChart = null

function renderGauge() {
  if (!gaugeRef.value) return
  if (!gaugeChart) gaugeChart = echarts.init(gaugeRef.value)
  const sk = scenario.value.key
  const isNumeric = typeof liveMetricA.value === 'number'
  if (!isNumeric) {
    // For non-numeric metrics, render a status dial.
    gaugeChart.setOption({
      series: [{
        type: 'gauge', radius: '95%', min: 0, max: 100,
        progress: { show: true, width: 12 },
        axisLine: { lineStyle: { width: 12, color: [[1, scenario.value.color]] } },
        axisTick: { show: false }, splitLine: { show: false }, axisLabel: { show: false },
        pointer: { show: false },
        anchor: { show: false },
        title: { show: false },
        detail: { valueAnimation: true, fontSize: 22, color: scenario.value.color, formatter: () => liveMetricA.value },
        data: [{ value: 70 }]
      }]
    })
    return
  }
  // Sensible per-scenario max for the gauge.
  const maxByKey = { sensor: 50, env: 200, 'smart-meter': 5, gateway: 50, vehicle: 200, plc: 4096, access: 30, default: 100 }
  const max = maxByKey[sk] || maxByKey.default
  gaugeChart.setOption({
    series: [{
      type: 'gauge', radius: '95%', min: 0, max,
      progress: { show: true, width: 14, itemStyle: { color: scenario.value.color } },
      axisLine: { lineStyle: { width: 14, color: [[1, '#e5e7eb']] } },
      pointer: { show: false }, anchor: { show: false },
      axisTick: { show: false }, splitLine: { show: false },
      axisLabel: { color: '#94a3b8', fontSize: 9, distance: -22 },
      title: { offsetCenter: [0, '70%'], fontSize: 11, color: '#64748b' },
      detail: { valueAnimation: true, fontSize: 24, color: scenario.value.color, offsetCenter: [0, '0%'], formatter: '{value}' },
      data: [{ value: liveMetricA.value, name: scenario.value.metricLabels[0] }]
    }]
  })
}

watch([liveMetricA, scenario, () => profile.value?.device?.runtimeState], async () => {
  await nextTick()
  renderGauge()
}, { flush: 'post' })

async function loadSession() {
  if (!deviceDid.value || !targetDomain.value) {
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
  // When the operator has picked a specific device in the target domain,
  // pass it so the backend operates on THAT device (not the source one).
  if (targetDeviceDid.value && targetDeviceDid.value !== deviceDid.value) {
    headers['X-Target-Device-DID'] = encodeURIComponent(targetDeviceDid.value)
  }
  const r = await fetch(url, { method, headers, body: body ? JSON.stringify(body) : undefined })
  return await r.json().catch(() => null)
}

async function loadTargetDevices() {
  if (!deviceDid.value || !targetDomain.value) return
  const url = `/api/cross/target-devices?deviceDid=${encodeURIComponent(deviceDid.value)}&targetDomain=${encodeURIComponent(targetDomain.value)}`
  const res = await apiRequest(url)
  if (res?.code === 0) {
    targetDevices.value = res.data?.items || []
    // Default to first ACTIVE target device (if any); fall back to source device.
    if (!targetDeviceDid.value && targetDevices.value.length > 0) {
      targetDeviceDid.value = targetDevices.value[0].deviceDid
    } else if (!targetDeviceDid.value) {
      targetDeviceDid.value = deviceDid.value
    }
  }
}

async function switchTargetDevice(did) {
  targetDeviceDid.value = did
  // Reset displayed data + immediately refetch for the new target.
  profile.value = null
  telemetry.value = []
  auditTrail.value = []
  await Promise.all([readProfile(), readAudit()])
}

async function readProfile() {
  opLoading.value = true
  flyPacket('read', '读设备档案', '#3b82f6')
  try {
    const res = await callRemote('/api/operations/remote/profile')
    if (res?.code === 0) {
      profile.value = res.data
      refreshLiveMetrics()
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
  flyPacket('read', scenario.value.readLabel, '#06b6d4')
  try {
    const res = await callRemote('/api/operations/remote/telemetry?limit=20')
    if (res?.code === 0) {
      telemetry.value = res.data?.items || []
      refreshLiveMetrics()
      antdMessage.success(`已读取 ${res.data?.count || 0} 条遥测`)
    }
  } finally {
    opLoading.value = false
  }
}

async function readAudit() {
  opLoading.value = true
  flyPacket('read', '读链上审计', '#7c3aed')
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
  const opColors = { RESTART_DEVICE: '#f59e0b', WRITE_DEVICE_CONFIG: '#ec4899', ADMIN_FIRMWARE_UPGRADE: '#dc2626' }
  const opLabels = { RESTART_DEVICE: '重启设备', WRITE_DEVICE_CONFIG: '下发配置', ADMIN_FIRMWARE_UPGRADE: '固件升级' }
  flyPacket('write', opLabels[operation] || operation, opColors[operation] || '#f59e0b')
  try {
    // body.deviceDid carries the operation-target DID — the same value
    // the X-Target-Device-DID header carries — so the backend payload
    // and header agree on which device is being mutated.
    const opDID = targetDeviceDid.value || deviceDid.value
    const body = {
      deviceDid: opDID,
      domainCode: targetDomain.value,
      operation,
      payload: typeof payload === 'string' ? payload : JSON.stringify(payload || {})
    }
    const res = await callRemote('/api/operations/protected', 'POST', body)
    if (res?.code === 0) {
      lastReceipt.value = res.data
      opHistory.value.unshift({ ...res.data, runAt: Date.now() })
      antdMessage.success(`✓ ${opLabels[operation] || operation} 已执行${res.data?.chain?.txHash ? '并上链' : ''}`)
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
  if (!deviceDid.value || !targetDomain.value) {
    const ok = await autoPickFromHistory()
    if (!ok && !sessionPicker.value.visible) {
      // No active sessions at all — show empty token card with CTA
      session.value = { active: false, reason: '当前没有任何有效的跨域凭证' }
      sessionLoading.value = false
      return
    }
    if (!ok) return // picker is visible, wait for user to choose
  }
  await loadSession()
  startTtlTimer()
  if (tokenActive.value) {
    await loadTargetDevices()
    await Promise.all([readProfile(), readAudit()])
    refreshLiveMetrics()
  }
  // Live metrics tick — gives the gauge a "real-time streaming" feel.
  liveTimer = setInterval(() => {
    if (profile.value && tokenActive.value) refreshLiveMetrics()
  }, 2500)
})

onUnmounted(() => {
  if (ttlTimer) clearInterval(ttlTimer)
  if (liveTimer) clearInterval(liveTimer)
  if (gaugeChart) { try { gaugeChart.dispose() } catch (_) {} gaugeChart = null }
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
  <a-modal
    v-model:open="sessionPicker.visible"
    title="选择一个活跃的跨域凭证"
    :footer="null"
    width="640px"
  >
    <a-alert
      type="info"
      showIcon
      style="margin-bottom: 12px"
      message="您当前有多个活跃凭证，请选择一个进入运维台"
    />
    <a-list :dataSource="sessionPicker.options" size="small" bordered>
      <template #renderItem="{ item }">
        <a-list-item>
          <a-list-item-meta>
            <template #title>
              <span class="mono" style="font-size:13px">{{ item.deviceDid }}</span>
            </template>
            <template #description>
              <a-tag color="geekblue">{{ item.fromDomainCode }}</a-tag>
              <span style="margin: 0 4px">→</span>
              <a-tag color="cyan">{{ item.toDomainCode }}</a-tag>
              <a-tag color="blue" style="margin-left: 6px">{{ item.permission }}</a-tag>
              <a-tag color="purple">{{ item.resource }}</a-tag>
            </template>
          </a-list-item-meta>
          <template #actions>
            <a-button type="primary" size="small" @click="chooseSessionFromPicker(item)">进入</a-button>
          </template>
        </a-list-item>
      </template>
    </a-list>
  </a-modal>

  <!-- ========== Cross-domain channel banner (flying-packet animation) ========== -->
  <div v-if="tokenActive" class="cross-channel">
    <div class="cross-end cross-end-src">
      <div class="cross-end-label">源域</div>
      <div class="cross-end-value">{{ session?.fromDomain || '-' }}</div>
      <div class="cross-end-sub">操作员 {{ session?.requestedBy || '-' }}</div>
    </div>
    <div class="cross-track">
      <div class="cross-track-line"></div>
      <div class="cross-token-pill">
        <span class="cross-token-dot"></span>
        AuthToken · {{ session?.permission }} / {{ session?.resource }} · ⏱ {{ ttlFormatted }}
      </div>
      <transition-group name="packet" tag="div" class="cross-packets">
        <div
          v-for="p in flyingPackets"
          :key="p.id"
          class="packet"
          :class="'packet-' + p.kind"
          :style="{ background: p.color, boxShadow: `0 0 12px ${p.color}` }"
        >
          {{ p.kind === 'read' ? '↘' : '↗' }} {{ p.label }}
        </div>
      </transition-group>
    </div>
    <div class="cross-end cross-end-tgt">
      <div class="cross-end-label">目标域</div>
      <div class="cross-end-value">{{ session?.toDomain || '-' }}</div>
      <div class="cross-end-sub mono">{{ targetDevices.length }} 台可访问设备</div>
    </div>
  </div>

  <!-- Target device picker (only when token active and > 1 device exists) -->
  <a-card v-if="tokenActive && targetDevices.length" class="target-picker-card" size="small">
    <div class="target-picker">
      <div class="target-picker-left">
        <div class="target-picker-label">当前操作目标设备</div>
        <a-select
          :value="targetDeviceDid"
          @update:value="switchTargetDevice"
          :options="targetDevices.map(d => ({ value: d.deviceDid, label: `${d.displayName || '未命名'} · ${d.deviceType}`, ...d }))"
          style="width: 100%; max-width: 520px"
          show-search
          optionFilterProp="label"
        >
          <template #option="option">
            <div style="display: flex; flex-direction: column; gap: 2px;">
              <span><a-tag :color="option.deviceType?.includes('环境') ? 'cyan' : 'blue'" style="margin-right: 4px">{{ option.deviceType }}</a-tag>{{ option.displayName || '未命名设备' }}</span>
              <span class="mono" style="font-size: 11px; color: #94a3b8;">{{ option.deviceDid }}</span>
            </div>
          </template>
        </a-select>
      </div>
      <div class="target-picker-right">
        <a-tag :color="targetDeviceDid && targetDeviceDid !== deviceDid ? 'green' : 'orange'">
          {{ targetDeviceDid && targetDeviceDid !== deviceDid
              ? '✓ 已切换到目标域设备'
              : '⚠ 当前在操作源设备本身' }}
        </a-tag>
        <a-button type="link" size="small" @click="loadTargetDevices">刷新设备列表</a-button>
      </div>
    </div>
  </a-card>

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
                <profile-outlined class="ops-icon" :style="{ color: scenario.color }" />
                <div class="ops-text">
                  <div class="ops-title">读设备档案</div>
                  <div class="ops-desc">{{ scenario.name }} · 元数据 / 生命周期 / 最新评分</div>
                </div>
                <a-tag color="blue">READ profile</a-tag>
              </div>
              <div class="ops-row" @click="readTelemetry">
                <component :is="scenario.icon" class="ops-icon" :style="{ color: scenario.color }" />
                <div class="ops-text">
                  <div class="ops-title">{{ scenario.readLabel }}</div>
                  <div class="ops-desc">实时拉取 {{ scenario.metricLabels[0] }} / {{ scenario.metricLabels[1] }} 等指标</div>
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
            <a-descriptions-item label="类型">
              <component :is="scenario.icon" :style="{ color: scenario.color, marginRight: '4px' }" />
              {{ profile.device?.deviceType }}
            </a-descriptions-item>
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
          </a-descriptions>
        </div>
      </a-card>

      <!-- ========== Scenario dashboard (changes shape per device type) ========== -->
      <a-card v-if="profile" class="rc-card scenario-card">
        <template #title>
          <component :is="scenario.icon" :style="{ color: scenario.color, marginRight: '6px' }" />
          业务场景实时面板 · {{ scenario.name }}
        </template>
        <template #extra>
          <a-tag :color="profile?.device?.runtimeState === 'TRUSTED' ? 'green' : 'orange'">
            {{ profile?.device?.runtimeState }}
          </a-tag>
        </template>
        <div class="scenario-body">
          <!-- Gauge for primary metric -->
          <div class="scenario-gauge-wrap">
            <div ref="gaugeRef" class="scenario-gauge"></div>
            <div class="scenario-metric-name">{{ scenario.metricLabels[0] }}</div>
          </div>
          <!-- Secondary metric + scenario-specific extras -->
          <div class="scenario-info">
            <div class="scenario-metric">
              <div class="scenario-metric-label">{{ scenario.metricLabels[1] }}</div>
              <div class="scenario-metric-value" :style="{ color: scenario.color }">{{ liveMetricB }}</div>
            </div>
            <!-- Scenario-specific extras -->
            <div v-if="scenario.key === 'camera'" class="scenario-camera">
              <div class="camera-frame">
                <video-camera-outlined :style="{ fontSize: '36px', color: scenario.color }" />
                <div class="camera-rec">● REC</div>
              </div>
              <div class="scenario-mini-hint">实时画面 (mock)</div>
            </div>
            <div v-else-if="scenario.key === 'access'" class="scenario-door">
              <div class="door" :class="{ 'door-open': profile?.device?.runtimeState === 'TRUSTED' }">
                <lock-outlined :style="{ fontSize: '32px', color: scenario.color }" />
              </div>
              <div class="scenario-mini-hint">
                {{ profile?.device?.runtimeState === 'TRUSTED' ? '门禁可放行' : '门禁已锁定' }}
              </div>
            </div>
            <div v-else-if="scenario.key === 'actuator' || scenario.key === 'appliance'" class="scenario-switch">
              <a-switch :checked="profile?.device?.runtimeState === 'TRUSTED'" disabled />
              <div class="scenario-mini-hint">{{ profile?.device?.runtimeState === 'TRUSTED' ? '设备运行中' : '设备待机' }}</div>
            </div>
            <div v-else-if="scenario.key === 'vehicle'" class="scenario-vehicle">
              <car-outlined :style="{ fontSize: '36px', color: scenario.color }" />
              <div class="scenario-mini-hint">车载终端 · 在线</div>
            </div>

            <!-- Sensor: vertical thermometer bar -->
            <div v-else-if="scenario.key === 'sensor'" class="scenario-sensor">
              <div class="thermo">
                <div class="thermo-stem">
                  <div class="thermo-fill" :style="{ height: thermoPct + '%', background: scenario.color }"></div>
                </div>
                <div class="thermo-bulb" :style="{ background: scenario.color }"></div>
              </div>
              <div class="scenario-mini-hint">温度计 · {{ liveMetricA }}°C</div>
            </div>

            <!-- Environment monitor: 4 micro indicators (PM2.5/CO₂/Temp/Hum) -->
            <div v-else-if="scenario.key === 'env'" class="scenario-env">
              <div class="env-grid">
                <div class="env-cell">
                  <div class="env-cell-label">PM2.5</div>
                  <div class="env-cell-value" :style="{ color: scenario.color }">{{ liveMetricA }}</div>
                </div>
                <div class="env-cell">
                  <div class="env-cell-label">CO₂</div>
                  <div class="env-cell-value" :style="{ color: scenario.color }">{{ liveMetricB }}</div>
                </div>
                <div class="env-cell">
                  <div class="env-cell-label">温度</div>
                  <div class="env-cell-value">{{ (20 + (profile?.lastState?.score || 80) / 10).toFixed(1) }}°</div>
                </div>
                <div class="env-cell">
                  <div class="env-cell-label">湿度</div>
                  <div class="env-cell-value">{{ Math.round(40 + (profile?.lastState?.score || 80) / 3) }}%</div>
                </div>
              </div>
              <div class="scenario-mini-hint">空气质量四维实时</div>
            </div>

            <!-- Smart meter: animated power flow lane -->
            <div v-else-if="scenario.key === 'smart-meter'" class="scenario-meter">
              <div class="meter-flow">
                <div class="meter-end">⚡</div>
                <div class="meter-lane">
                  <div class="meter-dot" v-for="i in 5" :key="i" :style="{ background: scenario.color, animationDelay: (i*0.3) + 's' }"></div>
                </div>
                <thunderbolt-outlined :style="{ fontSize: '24px', color: scenario.color }" />
              </div>
              <div class="scenario-mini-hint">{{ liveMetricA }} kW · 计费中</div>
            </div>

            <!-- Gateway: hub + 4 sub-device dots -->
            <div v-else-if="scenario.key === 'gateway'" class="scenario-gateway">
              <div class="gw-topology">
                <div class="gw-hub" :style="{ background: scenario.color }">
                  <api-outlined :style="{ fontSize: '18px', color: '#fff' }" />
                </div>
                <div v-for="i in 4" :key="i" class="gw-leaf" :class="'gw-leaf-' + i">
                  <div class="gw-leaf-dot" :style="{ background: scenario.color }"></div>
                </div>
                <svg class="gw-lines" viewBox="0 0 120 120">
                  <line v-for="i in 4" :key="i"
                        :x1="60" :y1="60"
                        :x2="60 + 50 * Math.cos((i-1) * Math.PI / 2 + Math.PI/4)"
                        :y2="60 + 50 * Math.sin((i-1) * Math.PI / 2 + Math.PI/4)"
                        :stroke="scenario.color" stroke-width="1" stroke-dasharray="2 2" opacity="0.5" />
                </svg>
              </div>
              <div class="scenario-mini-hint">{{ liveMetricA }} 台子设备已连接</div>
            </div>

            <!-- PLC: register bar visualizer -->
            <div v-else-if="scenario.key === 'plc'" class="scenario-plc">
              <div class="plc-bars">
                <div v-for="i in 8" :key="i" class="plc-bar"
                     :style="{ height: (20 + (((profile?.lastState?.score || 80) + i*7) % 60)) + '%', background: scenario.color }"></div>
              </div>
              <div class="scenario-mini-hint">寄存器 R0-R7 · 周期 {{ liveMetricB }}ms</div>
            </div>

            <!-- Default / unknown -->
            <div v-else class="scenario-pulse">
              <div class="pulse-dot" :style="{ background: scenario.color }"></div>
              <div class="scenario-mini-hint">数据流实时刷新 (每 2.5s)</div>
            </div>
          </div>
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

/* ============== Cross-domain channel banner ============== */
.cross-channel {
  display: flex;
  align-items: stretch;
  gap: 12px;
  padding: 14px 16px;
  margin: 0 0 12px;
  background: linear-gradient(90deg, #1e293b 0%, #1e2a78 50%, #312e81 100%);
  border-radius: 14px;
  color: #fff;
  position: relative;
  overflow: hidden;
}

.cross-end {
  flex: 0 0 220px;
  padding: 10px 14px;
  background: rgba(255, 255, 255, 0.08);
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.cross-end-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.7);
  letter-spacing: 0.4px;
  text-transform: uppercase;
}

.cross-end-value {
  font-size: 18px;
  font-weight: 700;
  margin-top: 4px;
  letter-spacing: 0.5px;
}

.cross-end-sub {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.55);
  margin-top: 2px;
}

.cross-end-tgt {
  text-align: right;
}

.cross-track {
  position: relative;
  flex: 1;
  min-height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.cross-track-line {
  position: absolute;
  left: 0;
  right: 0;
  top: 50%;
  height: 2px;
  background: linear-gradient(90deg, transparent 0%, rgba(248, 192, 0, 0.7) 30%, rgba(248, 192, 0, 0.7) 70%, transparent 100%);
  transform: translateY(-50%);
}

.cross-token-pill {
  position: relative;
  padding: 6px 14px;
  border-radius: 999px;
  background: linear-gradient(135deg, #f8c000 0%, #ffa840 100%);
  color: #1f2937;
  font-size: 12px;
  font-weight: 600;
  box-shadow: 0 4px 14px rgba(248, 192, 0, 0.45);
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.cross-token-dot {
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: #16a34a;
  box-shadow: 0 0 6px #16a34a;
  animation: pulse-dot 1.6s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%, 100% { opacity: 0.5; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.15); }
}

.cross-packets {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.packet {
  position: absolute;
  top: 50%;
  left: 20px;
  transform: translateY(-50%);
  padding: 4px 10px;
  border-radius: 999px;
  color: #fff;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
  animation: fly-right 1.6s ease-in-out forwards;
}

@keyframes fly-right {
  0% { left: 8%; opacity: 0; transform: translateY(-50%) scale(0.6); }
  15% { opacity: 1; transform: translateY(-50%) scale(1); }
  85% { opacity: 1; transform: translateY(-50%) scale(1); }
  100% { left: 92%; opacity: 0; transform: translateY(-50%) scale(0.8); }
}

.packet-write {
  animation-name: fly-right-write;
}

@keyframes fly-right-write {
  0% { left: 8%; opacity: 0; transform: translateY(-30%) scale(0.6); }
  15% { opacity: 1; transform: translateY(-50%) scale(1); }
  85% { opacity: 1; transform: translateY(-50%) scale(1); }
  100% { left: 92%; opacity: 0; transform: translateY(-50%) scale(0.8); }
}

/* ============== Scenario dashboard ============== */
.scenario-card {
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
}

.scenario-body {
  display: flex;
  gap: 16px;
  align-items: center;
}

.scenario-gauge-wrap {
  position: relative;
  width: 180px;
  flex: 0 0 180px;
  text-align: center;
}

.scenario-gauge {
  width: 180px;
  height: 150px;
}

.scenario-metric-name {
  font-size: 11px;
  color: #64748b;
  margin-top: -8px;
}

.scenario-info {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.scenario-metric {
  padding: 8px 12px;
  background: #f8fafc;
  border-radius: 8px;
  border-left: 3px solid #cbd5e1;
}

.scenario-metric-label {
  font-size: 11px;
  color: #64748b;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.scenario-metric-value {
  font-size: 22px;
  font-weight: 700;
  margin-top: 2px;
}

.scenario-mini-hint {
  font-size: 11px;
  color: #94a3b8;
  margin-top: 6px;
  text-align: center;
}

.scenario-camera .camera-frame {
  position: relative;
  height: 80px;
  background: linear-gradient(135deg, #0f172a 0%, #1e293b 100%);
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.camera-rec {
  position: absolute;
  top: 6px;
  right: 8px;
  font-size: 10px;
  color: #f87171;
  font-weight: 700;
  letter-spacing: 0.8px;
  animation: blink 1.4s linear infinite;
}

@keyframes blink {
  0%, 100% { opacity: 0.3; }
  50% { opacity: 1; }
}

.scenario-door .door {
  height: 80px;
  border-radius: 8px;
  background: linear-gradient(180deg, #fef3c7 0%, #fde68a 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px solid #fbbf24;
  transition: all 0.5s ease;
}

.scenario-door .door.door-open {
  background: linear-gradient(180deg, #d1fae5 0%, #a7f3d0 100%);
  border-color: #10b981;
}

.scenario-switch {
  text-align: center;
  padding: 14px 0;
}

.scenario-vehicle {
  text-align: center;
  padding: 14px 0;
}

.scenario-pulse {
  text-align: center;
  padding: 14px 0;
}

.scenario-pulse .pulse-dot {
  width: 16px;
  height: 16px;
  border-radius: 999px;
  margin: 0 auto;
  animation: pulse-dot-big 1.4s ease-in-out infinite;
}

@keyframes pulse-dot-big {
  0%, 100% { opacity: 0.4; transform: scale(0.85); }
  50% { opacity: 1; transform: scale(1.3); }
}

/* Packet transition group fallback */
.packet-enter-active, .packet-leave-active { transition: opacity 0.2s; }
.packet-enter-from, .packet-leave-to { opacity: 0; }

/* ===== Target device picker ===== */
.target-picker-card {
  margin-bottom: 12px;
  border-radius: 12px;
  border-left: 3px solid #4453d9;
}

.target-picker {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.target-picker-left {
  flex: 1;
  min-width: 260px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.target-picker-label {
  font-size: 11px;
  color: #64748b;
  letter-spacing: 0.4px;
  text-transform: uppercase;
}

.target-picker-right {
  display: flex;
  align-items: center;
  gap: 6px;
}

/* ===== Sensor: thermometer ===== */
.scenario-sensor {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.thermo {
  position: relative;
  width: 24px;
  height: 72px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.thermo-stem {
  width: 10px;
  flex: 1;
  border: 2px solid #cbd5e1;
  border-bottom: none;
  border-top-left-radius: 6px;
  border-top-right-radius: 6px;
  background: #f1f5f9;
  position: relative;
  overflow: hidden;
}

.thermo-fill {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  border-radius: 4px 4px 0 0;
  transition: height 0.6s ease;
}

.thermo-bulb {
  width: 22px;
  height: 22px;
  border-radius: 999px;
  margin-top: -4px;
  border: 2px solid #cbd5e1;
}

/* ===== Environment monitor: 2x2 grid ===== */
.scenario-env .env-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
}

.env-cell {
  padding: 6px 8px;
  background: #f8fafc;
  border-radius: 6px;
  border-left: 3px solid #06b6d4;
  text-align: center;
}

.env-cell-label {
  font-size: 10px;
  color: #64748b;
  letter-spacing: 0.3px;
}

.env-cell-value {
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
}

/* ===== Smart meter: animated power flow ===== */
.scenario-meter {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 6px;
}

.meter-flow {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 0;
}

.meter-end {
  font-size: 18px;
}

.meter-lane {
  position: relative;
  width: 100px;
  height: 14px;
  background: linear-gradient(90deg, rgba(249, 115, 22, 0.1), rgba(249, 115, 22, 0.25));
  border-radius: 999px;
  overflow: hidden;
  display: flex;
  align-items: center;
}

.meter-dot {
  position: absolute;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  animation: meter-flow 1.6s linear infinite;
}

@keyframes meter-flow {
  0% { left: -10%; opacity: 0; }
  10% { opacity: 1; }
  90% { opacity: 1; }
  100% { left: 100%; opacity: 0; }
}

/* ===== Gateway: hub + leaves topology ===== */
.scenario-gateway {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.gw-topology {
  position: relative;
  width: 120px;
  height: 120px;
}

.gw-hub {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  display: flex;
  align-items: center;
  justify-content: center;
  transform: translate(-50%, -50%);
  z-index: 2;
  box-shadow: 0 0 14px rgba(59, 130, 246, 0.4);
}

.gw-leaf {
  position: absolute;
  width: 14px;
  height: 14px;
  z-index: 2;
}

.gw-leaf-dot {
  width: 100%;
  height: 100%;
  border-radius: 999px;
  animation: leaf-pulse 1.8s ease-in-out infinite;
}

.gw-leaf-1 { top: 12px;  right: 12px; }
.gw-leaf-2 { bottom: 12px; right: 12px; animation-delay: 0.4s; }
.gw-leaf-3 { bottom: 12px; left: 12px;  animation-delay: 0.8s; }
.gw-leaf-4 { top: 12px;  left: 12px;  animation-delay: 1.2s; }

@keyframes leaf-pulse {
  0%, 100% { opacity: 0.4; transform: scale(0.85); }
  50% { opacity: 1; transform: scale(1.15); }
}

.gw-lines {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

/* ===== PLC: register bar visualizer ===== */
.scenario-plc {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.plc-bars {
  display: flex;
  align-items: flex-end;
  gap: 4px;
  height: 70px;
  padding: 4px;
}

.plc-bar {
  width: 9px;
  border-radius: 2px 2px 0 0;
  transition: height 0.5s ease;
  opacity: 0.85;
}

.plc-bar:nth-child(odd) { opacity: 0.6; }
</style>
