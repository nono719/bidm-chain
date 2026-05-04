<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts'
import { apiRequest } from '../api/client'
import { DEVICE_TYPE_FILTER_OPTIONS } from '../constants/deviceTypes'

const loading = ref(false)
const chartEl = ref(null)
let chart = null

const overview = ref({ series: [], markers: [] })
const devices = ref([])
const alerts = ref([])

const filters = reactive({ domain: 'ALL', state: 'ALL', type: 'ALL' })
const selectedDevice = ref(null)

function shortDid(v) {
  if (!v) return '-'
  if (v.length <= 22) return v
  return `${v.slice(0, 14)}...${v.slice(-6)}`
}

function fmt(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}

async function loadAll() {
  loading.value = true
  try {
    const [o, d, a] = await Promise.all([
      apiRequest('/api/monitor/overview?hours=24'),
      apiRequest('/api/monitor/devices'),
      apiRequest('/api/monitor/alerts?limit=60')
    ])
    if (o.code !== 0) {
      antdMessage.error(o.message || '加载监控数据失败')
    } else {
      overview.value = o.data
    }
    if (d.code === 0) devices.value = d.data
    if (a.code === 0) alerts.value = a.data
    renderChart()
  } finally {
    loading.value = false
  }
}

function renderChart() {
  if (!chartEl.value) return
  if (!chart) {
    chart = echarts.init(chartEl.value)
  }
  const xs = (overview.value.series || []).map((r) => r.bucket)
  const ys = (overview.value.series || []).map((r) => r.online)
  const alertYs = (overview.value.series || []).map((r) => r.alerts)
  const markerSet = new Set(overview.value.markers || [])

  chart.setOption({
    backgroundColor: 'transparent',
    grid: { left: 32, right: 12, top: 24, bottom: 26 },
    tooltip: { trigger: 'axis' },
    xAxis: {
      type: 'category',
      data: xs,
      axisLabel: { color: '#667085', formatter: (v) => dayjs(v).format('HH:mm') },
      axisLine: { lineStyle: { color: 'rgba(16,24,40,0.14)' } }
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#667085' },
      splitLine: { lineStyle: { color: 'rgba(16,24,40,0.08)' } }
    },
    series: [
      {
        name: '在线设备数',
        type: 'line',
        data: ys,
        smooth: true,
        symbolSize: 8,
        itemStyle: {
          color: '#22c55e'
        },
        markPoint: {
          data: xs
            .map((x, idx) => (markerSet.has(x) ? { coord: [x, ys[idx]], value: '告警' } : null))
            .filter(Boolean),
          itemStyle: { color: '#ef4444' }
        },
        areaStyle: { color: 'rgba(34,197,94,0.12)' }
      },
      {
        name: '告警次数',
        type: 'bar',
        data: alertYs,
        itemStyle: { color: 'rgba(239,68,68,0.35)' }
      }
    ]
  })
}

function onResize() {
  if (chart) chart.resize()
}

onMounted(() => {
  window.addEventListener('resize', onResize)
  loadAll()
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', onResize)
  if (chart) {
    chart.dispose()
    chart = null
  }
})

watch(
  () => overview.value,
  () => {
    renderChart()
  }
)

const kpi = computed(() => {
  const total = devices.value.length
  const online = devices.value.filter((d) => d.lastReport && d.lastReport.online).length
  const risky = devices.value.filter((d) => d.runtimeState !== 'TRUSTED').length
  const alertCount = alerts.value.length
  return { total, online, risky, alertCount }
})

const filteredDevices = computed(() => {
  return devices.value.filter((d) => {
    if (filters.domain !== 'ALL' && d.domainCode !== filters.domain) return false
    if (filters.state !== 'ALL' && d.runtimeState !== filters.state) return false
    if (filters.type !== 'ALL' && d.deviceType !== filters.type) return false
    return true
  })
})

const deviceColumns = [
  { title: '设备', dataIndex: 'displayName', key: 'displayName' },
  { title: 'DID', dataIndex: 'deviceDid', key: 'deviceDid' },
  { title: '域', dataIndex: 'domainCode', key: 'domainCode', width: 110 },
  { title: '状态', dataIndex: 'runtimeState', key: 'runtimeState', width: 110 },
  { title: '在线', dataIndex: 'online', key: 'online', width: 90 },
  { title: '评分', dataIndex: 'score', key: 'score', width: 90 },
  { title: '最近上报', dataIndex: 'lastAt', key: 'lastAt', width: 170 }
]

const alertColumns = [
  { title: '时间', dataIndex: 'createdAt', key: 'createdAt', width: 170 },
  { title: '严重等级', dataIndex: 'severity', key: 'severity', width: 120 },
  { title: '设备DID', dataIndex: 'deviceDid', key: 'deviceDid' },
  { title: '描述', dataIndex: 'message', key: 'message' }
]

function severityTag(s) {
  if (s >= 2) return { color: 'red', text: 'CRITICAL' }
  if (s === 1) return { color: 'orange', text: 'WARNING' }
  return { color: 'green', text: 'OK' }
}

function setSelected(record) {
  selectedDevice.value = record
}
</script>

<template>
  <a-card class="panel-card" title="设备状态监控（图5-4）" :loading="loading">
    <a-row :gutter="16">
      <a-col :xs="24" :lg="16">
        <a-card class="inner-card" title="近24小时在线设备数" size="small">
          <div ref="chartEl" class="chart" />
        </a-card>
      </a-col>
      <a-col :xs="24" :lg="8">
        <a-card class="inner-card" title="聚合指标" size="small">
          <a-row :gutter="12">
            <a-col :span="12">
              <a-statistic title="设备总数" :value="kpi.total" />
            </a-col>
            <a-col :span="12">
              <a-statistic title="在线设备" :value="kpi.online" />
            </a-col>
            <a-col :span="12" style="margin-top: 10px">
              <a-statistic title="非可信设备" :value="kpi.risky" />
            </a-col>
            <a-col :span="12" style="margin-top: 10px">
              <a-statistic title="告警条目" :value="kpi.alertCount" />
            </a-col>
          </a-row>
          <a-divider />
          <a-space direction="vertical" style="width: 100%">
            <a-select
              v-model:value="filters.domain"
              :options="[
                { value: 'ALL', label: '全部域' },
                { value: 'domain-a', label: '制造商域' },
                { value: 'domain-b', label: '服务商域' }
              ]"
            />
            <a-select
              v-model:value="filters.state"
              :options="[
                { value: 'ALL', label: '全部状态' },
                { value: 'TRUSTED', label: 'TRUSTED' },
                { value: 'RISKY', label: 'RISKY' },
                { value: 'UNKNOWN', label: 'UNKNOWN' }
              ]"
            />
            <a-select
              v-model:value="filters.type"
              :options="DEVICE_TYPE_FILTER_OPTIONS"
            />
          </a-space>
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="16" style="margin-top: 16px">
      <a-col :xs="24" :lg="14">
        <a-card class="inner-card" title="设备列表" size="small">
          <a-table
            size="small"
            :columns="deviceColumns"
            :dataSource="filteredDevices"
            :pagination="{ pageSize: 8 }"
            rowKey="deviceDid"
            :customRow="(record) => ({ onClick: () => setSelected(record) })"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'deviceDid'">
                <span class="mono">{{ shortDid(record.deviceDid) }}</span>
              </template>
              <template v-else-if="column.key === 'runtimeState'">
                <a-tag :color="record.runtimeState === 'TRUSTED' ? 'green' : record.runtimeState === 'RISKY' ? 'orange' : 'default'">
                  {{ record.runtimeState }}
                </a-tag>
              </template>
              <template v-else-if="column.key === 'online'">
                <a-badge :status="record.lastReport?.online ? 'success' : 'default'" :text="record.lastReport?.online ? 'ONLINE' : 'OFFLINE'" />
              </template>
              <template v-else-if="column.key === 'score'">
                {{ record.lastReport?.score ?? '-' }}
              </template>
              <template v-else-if="column.key === 'lastAt'">
                {{ record.lastReport?.createdAt ? fmt(record.lastReport.createdAt) : '-' }}
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="10">
        <a-card class="inner-card" title="异常告警列表" size="small">
          <a-table
            size="small"
            :columns="alertColumns"
            :dataSource="alerts"
            :pagination="{ pageSize: 6 }"
            rowKey="id"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'createdAt'">
                {{ fmt(record.createdAt) }}
              </template>
              <template v-else-if="column.key === 'severity'">
                <a-tag :color="severityTag(record.severity).color">{{ severityTag(record.severity).text }}</a-tag>
              </template>
              <template v-else-if="column.key === 'deviceDid'">
                <span class="mono">{{ shortDid(record.deviceDid) }}</span>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>
    </a-row>

    <a-drawer :open="selectedDevice !== null" title="设备详情" placement="right" :width="520" @close="selectedDevice = null">
      <a-space direction="vertical" style="width: 100%" size="middle">
        <a-descriptions size="small" :column="1" bordered>
          <a-descriptions-item label="设备名称">{{ selectedDevice?.displayName }}</a-descriptions-item>
          <a-descriptions-item label="设备DID"><span class="mono">{{ selectedDevice?.deviceDid }}</span></a-descriptions-item>
          <a-descriptions-item label="所属域">{{ selectedDevice?.domainCode }}</a-descriptions-item>
          <a-descriptions-item label="类型">{{ selectedDevice?.deviceType }}</a-descriptions-item>
          <a-descriptions-item label="状态">{{ selectedDevice?.runtimeState }}</a-descriptions-item>
          <a-descriptions-item label="最近上报">{{ selectedDevice?.lastReport?.createdAt ? fmt(selectedDevice.lastReport.createdAt) : '-' }}</a-descriptions-item>
          <a-descriptions-item label="在线">{{ selectedDevice?.lastReport?.online ? 'ONLINE' : 'OFFLINE' }}</a-descriptions-item>
          <a-descriptions-item label="评分">{{ selectedDevice?.lastReport?.score ?? '-' }}</a-descriptions-item>
          <a-descriptions-item label="固件校验">{{ selectedDevice?.lastReport?.firmwareValid ? 'PASS' : 'FAIL' }}</a-descriptions-item>
          <a-descriptions-item label="证书校验">{{ selectedDevice?.lastReport?.certValid ? 'PASS' : 'FAIL' }}</a-descriptions-item>
          <a-descriptions-item label="告警信息">{{ selectedDevice?.lastReport?.message || '-' }}</a-descriptions-item>
          <a-descriptions-item label="区块高度">{{ selectedDevice?.lastReport?.blockHeight ?? '-' }}</a-descriptions-item>
          <a-descriptions-item label="交易哈希"><span class="mono">{{ selectedDevice?.lastReport?.txHash || '-' }}</span></a-descriptions-item>
        </a-descriptions>
      </a-space>
    </a-drawer>
  </a-card>
</template>

<style scoped>
.chart {
  width: 100%;
  height: 300px;
}

.mono {
  font-family: var(--mono);
  color: #344054;
}
</style>
