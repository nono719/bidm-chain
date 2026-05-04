<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts'
import { apiRequest, getUser } from '../api/client'

const dashboard = ref(null)
const aggregations = ref([])
const detail = ref(null)
const detailVisible = ref(false)
const detailLoading = ref(false)

const loading = ref(false)
const aggLoading = ref(false)

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const simForm = ref({ deviceDid: '', noiseRate: 25 })
const simLoading = ref(false)
const simResult = ref(null)
const simStep = ref(0) // 0 idle, 1 collect, 2 vote, 3 anchor, 4 done
const deviceOptions = ref([])

const topoChartRef = ref(null)
const algoChartRef = ref(null)
const partChartRef = ref(null)
let topoChart = null
let algoChart = null
let partChart = null
let pollTimer = null

async function loadDashboard() {
  loading.value = true
  try {
    const res = await apiRequest('/api/oracle/dashboard')
    if (res.code === 0) {
      dashboard.value = res.data
      await nextTick()
      renderTopology()
      renderAlgo()
      renderParticipation()
    }
  } finally {
    loading.value = false
  }
}

async function loadAggregations() {
  aggLoading.value = true
  try {
    const res = await apiRequest('/api/oracle/aggregations?limit=30')
    if (res.code === 0) aggregations.value = res.data || []
  } finally {
    aggLoading.value = false
  }
}

async function loadDevices() {
  const res = await apiRequest('/api/devices?limit=50')
  if (res.code === 0) {
    deviceOptions.value = (res.data || []).map((d) => ({ value: d.deviceDid, label: `${d.deviceDid} · ${d.domainCode}` }))
    if (!simForm.value.deviceDid && deviceOptions.value.length) {
      simForm.value.deviceDid = deviceOptions.value[0].value
    }
  }
}

function renderTopology() {
  if (!topoChartRef.value || !dashboard.value) return
  if (topoChart && topoChart.getDom() !== topoChartRef.value) {
    try { topoChart.dispose() } catch (_) {}
    topoChart = null
  }
  if (!topoChart) topoChart = echarts.init(topoChartRef.value)

  const nodes = dashboard.value.nodes || []
  const data = [
    { id: 'aggregator', name: `聚合器\n(门限 ${dashboard.value.threshold}/${nodes.length})`, symbolSize: 80, itemStyle: { color: '#182078' }, label: { color: '#fff', fontWeight: 700 }, x: 0, y: 0 }
  ]
  const links = []
  nodes.forEach((n, i) => {
    const angle = (i * 2 * Math.PI) / Math.max(1, nodes.length)
    data.push({
      id: `node:${n.id}`,
      name: `${n.nodeName}\n${n.algorithm}`,
      symbolSize: 56,
      itemStyle: { color: healthColor(n.health) },
      label: { color: '#1f2937', fontSize: 11, fontWeight: 600 },
      x: Math.cos(angle) * 200,
      y: Math.sin(angle) * 200
    })
    links.push({
      source: `node:${n.id}`,
      target: 'aggregator',
      lineStyle: { width: Math.max(1.5, Math.min(5, n.participation * 0.3)), color: healthColor(n.health), opacity: 0.7 }
    })
  })

  topoChart.setOption({
    backgroundColor: 'transparent',
    tooltip: {
      formatter: (params) => {
        if (params.dataType !== 'node') return ''
        if (params.data.id === 'aggregator') return '中心聚合器'
        const n = nodes.find((x) => `node:${x.id}` === params.data.id)
        if (!n) return ''
        return `<b>${n.nodeName}</b><br>算法: ${n.algorithm}<br>健康度: ${n.health}<br>7天参与: ${n.participation}<br>多数派次数: ${n.majorityCount}<br>最近上报: ${n.lastSeen ? dayjs(n.lastSeen).format('MM-DD HH:mm') : '从未'}`
      }
    },
    series: [{
      type: 'graph',
      layout: 'none',
      roam: true,
      data,
      links,
      label: { show: true, position: 'bottom' }
    }]
  })
}

function renderAlgo() {
  if (!algoChartRef.value || !dashboard.value) return
  if (!algoChart) algoChart = echarts.init(algoChartRef.value)
  const dist = dashboard.value.algorithmDist || {}
  const items = Object.keys(dist).map((k) => ({ name: k, value: dist[k] }))
  algoChart.setOption({
    tooltip: { trigger: 'item' },
    legend: { bottom: 0, type: 'plain', textStyle: { fontSize: 11 } },
    series: [{
      type: 'pie',
      radius: ['45%', '70%'],
      label: { formatter: '{b}: {c}', fontSize: 11 },
      data: items
    }]
  })
}

function renderParticipation() {
  if (!partChartRef.value || !dashboard.value) return
  if (!partChart) partChart = echarts.init(partChartRef.value)
  const nodes = dashboard.value.nodes || []
  partChart.setOption({
    tooltip: { trigger: 'axis' },
    grid: { top: 10, right: 10, bottom: 30, left: 40 },
    xAxis: { type: 'category', data: nodes.map((n) => n.nodeName), axisLabel: { fontSize: 10, rotate: 20 } },
    yAxis: { type: 'value' },
    series: [
      { name: '总参与', type: 'bar', stack: 'p', data: nodes.map((n) => Math.max(0, n.participation - n.majorityCount)), itemStyle: { color: '#fcd34d' } },
      { name: '多数派', type: 'bar', stack: 'p', data: nodes.map((n) => n.majorityCount), itemStyle: { color: '#16a34a' } }
    ]
  })
}

function healthColor(h) {
  if (h === 'HEALTHY') return '#16a34a'
  if (h === 'LAGGING') return '#f59e0b'
  if (h === 'STALE') return '#dc2626'
  if (h === 'IDLE') return '#94a3b8'
  if (h === 'INACTIVE' || h === 'REVOKED') return '#6b7280'
  return '#94a3b8'
}

function statusColor(s) {
  if (s === 'TRUSTED') return 'green'
  if (s === 'RISKY') return 'orange'
  return 'default'
}

function fmtTime(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}
function shortHash(v) {
  if (!v) return '-'
  return `${v.slice(0, 10)}...${v.slice(-6)}`
}

async function openDetail(aggId) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const res = await apiRequest(`/api/oracle/aggregations/${aggId}`)
    if (res.code === 0) detail.value = res.data
  } finally {
    detailLoading.value = false
  }
}

async function runDemo() {
  if (!simForm.value.deviceDid) {
    antdMessage.warning('请选择设备')
    return
  }
  simLoading.value = true
  simResult.value = null
  simStep.value = 1
  try {
    // step 1: collect submissions (visual delay)
    await new Promise((r) => setTimeout(r, 600))
    simStep.value = 2 // vote
    const res = await apiRequest('/api/oracle/demo/simulate', { method: 'POST', body: simForm.value })
    if (res.code !== 0) {
      antdMessage.error(res.message || '演示失败')
      simStep.value = 0
      return
    }
    await new Promise((r) => setTimeout(r, 500))
    simStep.value = 3 // anchor
    await new Promise((r) => setTimeout(r, 500))
    simStep.value = 4
    simResult.value = res.data
    antdMessage.success(`✓ ${res.data.participating} 节点参与，门限 ${res.data.threshold}，${res.data.reachedThreshold ? '已上链' : '未达门限'}`)
    await Promise.all([loadDashboard(), loadAggregations()])
  } finally {
    simLoading.value = false
  }
}

const aggCols = [
  { title: '#', dataIndex: 'id', key: 'id', width: 60 },
  { title: '设备', dataIndex: 'deviceDid', key: 'deviceDid', ellipsis: { showTitle: true } },
  { title: '参与/门限', key: 'part', width: 110 },
  { title: '一致性', key: 'consensus', width: 100 },
  { title: '状态', dataIndex: 'stateLabel', key: 'stateLabel', width: 90 },
  { title: '评分', dataIndex: 'score', key: 'score', width: 70 },
  { title: '区块', dataIndex: 'blockHeight', key: 'block', width: 80 },
  { title: '时间', dataIndex: 'createdAt', key: 'createdAt', width: 160 },
  { title: '操作', key: 'op', width: 80 }
]

watch(() => dashboard.value, () => nextTick(renderTopology))

onMounted(async () => {
  await loadDevices()
  await Promise.all([loadDashboard(), loadAggregations()])
  pollTimer = setInterval(() => {
    loadDashboard()
    loadAggregations()
  }, 15000)
  window.addEventListener('resize', resizeAll)
})

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer)
  window.removeEventListener('resize', resizeAll)
  ;[topoChart, algoChart, partChart].forEach((c) => { if (c) try { c.dispose() } catch (_) {} })
})

function resizeAll() {
  ;[topoChart, algoChart, partChart].forEach((c) => c && c.resize())
}
</script>

<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <!-- Top KPIs -->
    <a-row :gutter="14">
      <a-col :xs="12" :sm="6">
        <a-card class="kpi-card" :loading="loading">
          <a-statistic title="节点总数" :value="dashboard?.nodesTotal ?? 0" :suffix="`(${dashboard?.nodesActive ?? 0} 活跃)`" />
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card class="kpi-card" :loading="loading">
          <a-statistic title="聚合门限 (k)" :value="dashboard?.threshold ?? 1" :suffix="`/ ${dashboard?.nodesTotal ?? 0}`" />
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card class="kpi-card" :loading="loading">
          <a-statistic title="24小时聚合次数" :value="dashboard?.roundsTotal24h ?? 0" />
        </a-card>
      </a-col>
      <a-col :xs="12" :sm="6">
        <a-card class="kpi-card" :loading="loading">
          <a-statistic title="可信比例 (24h)" :value="Math.round((dashboard?.successRate ?? 0) * 100)" suffix="%" />
        </a-card>
      </a-col>
    </a-row>

    <a-row :gutter="14">
      <a-col :xs="24" :lg="14">
        <a-card title="预言机节点拓扑" class="panel-card">
          <template #extra>
            <a-tag color="purple">门限聚合 k-of-n</a-tag>
            <a-button type="link" size="small" @click="loadDashboard">刷新</a-button>
          </template>
          <div ref="topoChartRef" class="topo-chart"></div>
          <div class="topo-legend">
            <a-space wrap size="small">
              <span><span class="dot" style="background:#16a34a"></span>HEALTHY (5min 内有上报)</span>
              <span><span class="dot" style="background:#f59e0b"></span>LAGGING</span>
              <span><span class="dot" style="background:#dc2626"></span>STALE</span>
              <span><span class="dot" style="background:#94a3b8"></span>IDLE</span>
              <span class="hint">连线粗细 = 7 天参与次数</span>
            </a-space>
          </div>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="10">
        <a-card title="算法分布" class="panel-card">
          <div ref="algoChartRef" class="small-chart"></div>
        </a-card>
        <a-card title="节点参与度（7 天）" class="panel-card" style="margin-top: 14px">
          <div ref="partChartRef" class="small-chart"></div>
          <div class="topo-legend">
            <span><span class="dot" style="background:#16a34a"></span>属于多数派</span>
            <span style="margin-left:10px"><span class="dot" style="background:#fcd34d"></span>少数派 / 噪声</span>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-card v-if="isAdmin" title="🧪 实时聚合演示" class="panel-card">
      <a-alert
        type="info"
        showIcon
        style="margin-bottom: 10px"
        message="一键模拟一次完整门限聚合：每个 ACTIVE 节点带噪声独立打分 → 多数派投票 → 达到门限 → 链上锚定 + Org1+Org2 双签。提高 noiseRate 可以制造分歧观察少数派。"
      />
      <a-space wrap>
        <a-select v-model:value="simForm.deviceDid" :options="deviceOptions" style="width: 320px" placeholder="选择设备" />
        <a-input-number v-model:value="simForm.noiseRate" addon-before="噪声" addon-after="%" :min="0" :max="80" style="width: 150px" />
        <a-button type="primary" :loading="simLoading" @click="runDemo">开始演示</a-button>
      </a-space>

      <div v-if="simStep > 0" class="sim-strip">
        <div class="sim-step" :class="{ 'sim-active': simStep >= 1, 'sim-done': simStep > 1 }">① 收集签名</div>
        <div class="sim-arrow">→</div>
        <div class="sim-step" :class="{ 'sim-active': simStep >= 2, 'sim-done': simStep > 2 }">② 多数派投票</div>
        <div class="sim-arrow">→</div>
        <div class="sim-step" :class="{ 'sim-active': simStep >= 3, 'sim-done': simStep > 3 }">③ 校验门限</div>
        <div class="sim-arrow">→</div>
        <div class="sim-step" :class="{ 'sim-active': simStep >= 4 }">④ 链上锚定</div>
      </div>

      <div v-if="simResult" style="margin-top: 12px">
        <a-alert
          :type="simResult.reachedThreshold ? 'success' : 'warning'"
          showIcon
          :message="simResult.reachedThreshold ? `✓ ${simResult.participating} 节点参与（门限 ${simResult.threshold}），最终 ${simResult.state}` : '⚠ 未达到门限，本轮聚合未上链'"
        />
        <a-row :gutter="14" style="margin-top: 10px">
          <a-col :xs="24" :lg="12">
            <a-table
              :columns="[{title:'节点',dataIndex:'nodeName',width:150},{title:'在线',dataIndex:'online',width:60},{title:'固件',dataIndex:'firmwareValid',width:60},{title:'证书',dataIndex:'certValid',width:60},{title:'分',dataIndex:'score',width:60},{title:'多数派',dataIndex:'inMajority',width:80}]"
              :dataSource="simResult.submissions"
              :pagination="false"
              size="small"
              rowKey="id"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.dataIndex === 'online' || column.dataIndex === 'firmwareValid' || column.dataIndex === 'certValid'">
                  <a-tag :color="record[column.dataIndex] ? 'green' : 'red'">{{ record[column.dataIndex] ? '✓' : '✗' }}</a-tag>
                </template>
                <template v-else-if="column.dataIndex === 'inMajority'">
                  <a-tag :color="record.inMajority ? 'green' : 'orange'">{{ record.inMajority ? '是' : '否' }}</a-tag>
                </template>
              </template>
            </a-table>
          </a-col>
          <a-col :xs="24" :lg="12">
            <a-descriptions :column="1" bordered size="small">
              <a-descriptions-item label="聚合状态">
                <a-tag :color="statusColor(simResult.state)" style="font-weight:600">{{ simResult.state }}</a-tag>
              </a-descriptions-item>
              <a-descriptions-item label="聚合分">{{ simResult.aggregated?.score }} <span style="color:#94a3b8;font-size:11px">(平均)</span></a-descriptions-item>
              <a-descriptions-item label="链上区块">#{{ simResult.chain?.blockHeight ?? '-' }}</a-descriptions-item>
              <a-descriptions-item label="TxHash"><span class="mono">{{ shortHash(simResult.chain?.txHash) }}</span></a-descriptions-item>
              <a-descriptions-item v-if="simResult.chain?.endorsers?.length" label="链上背书">
                <a-tag v-for="(e,i) in simResult.chain.endorsers" :key="i" :color="e.mspId === 'Org1MSP' ? 'green' : 'cyan'">✓ {{ e.mspId }}</a-tag>
              </a-descriptions-item>
            </a-descriptions>
          </a-col>
        </a-row>
      </div>
    </a-card>

    <a-card title="聚合时间线（最近 30 笔）" class="panel-card">
      <template #extra>
        <a-button type="link" size="small" :loading="aggLoading" @click="loadAggregations">刷新</a-button>
      </template>
      <a-table
        :columns="aggCols"
        :dataSource="aggregations"
        :pagination="{ pageSize: 10 }"
        size="small"
        rowKey="id"
        :scroll="{ x: 900 }"
      >
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'deviceDid'">
            <span class="mono" :title="record.deviceDid">{{ record.deviceDid }}</span>
          </template>
          <template v-else-if="column.key === 'part'">
            <span class="mono">{{ record.participatingNodes }}</span>
            <span style="margin: 0 4px; color:#94a3b8">/</span>
            <span class="mono">{{ record.thresholdAtAgg }}</span>
            <a-tag v-if="record.participatingNodes >= record.thresholdAtAgg" color="green" style="margin-left:4px">达成</a-tag>
            <a-tag v-else color="red" style="margin-left:4px">未达</a-tag>
          </template>
          <template v-else-if="column.key === 'consensus'">
            <a-tag v-if="record.submissionsTotal" :color="record.submissionsInMaj === record.submissionsTotal ? 'green' : 'orange'">
              {{ record.submissionsInMaj }}/{{ record.submissionsTotal }} 一致
            </a-tag>
          </template>
          <template v-else-if="column.key === 'stateLabel'">
            <a-tag :color="statusColor(record.stateLabel)">{{ record.stateLabel }}</a-tag>
          </template>
          <template v-else-if="column.key === 'block'">
            <span class="mono" v-if="record.blockHeight">#{{ record.blockHeight }}</span>
            <span v-else style="color:#94a3b8">未上链</span>
          </template>
          <template v-else-if="column.key === 'createdAt'">{{ fmtTime(record.createdAt) }}</template>
          <template v-else-if="column.key === 'op'">
            <a-button type="link" size="small" @click="openDetail(record.id)">详情</a-button>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal v-model:open="detailVisible" :title="detail ? `聚合 #${detail.aggregation.id} · ${detail.aggregation.deviceDid}` : '聚合详情'" width="780px" :footer="null">
      <a-spin :spinning="detailLoading">
        <div v-if="detail">
          <a-descriptions size="small" :column="2" bordered>
            <a-descriptions-item label="设备 DID" :span="2"><span class="mono">{{ detail.aggregation.deviceDid }}</span></a-descriptions-item>
            <a-descriptions-item label="聚合状态">
              <a-tag :color="statusColor(detail.aggregation.stateLabel)" style="font-weight:600">{{ detail.aggregation.stateLabel }}</a-tag>
            </a-descriptions-item>
            <a-descriptions-item label="聚合分">{{ detail.aggregation.score }}</a-descriptions-item>
            <a-descriptions-item label="参与节点">{{ detail.aggregation.participatingNodes }} / 门限 {{ detail.aggregation.thresholdAtAgg }}</a-descriptions-item>
            <a-descriptions-item label="区块">{{ detail.aggregation.blockHeight ? '#' + detail.aggregation.blockHeight : '未上链' }}</a-descriptions-item>
            <a-descriptions-item label="TxHash" :span="2"><span class="mono">{{ detail.aggregation.txHash || '-' }}</span></a-descriptions-item>
          </a-descriptions>

          <h4 style="margin-top: 14px">投票分布</h4>
          <div class="vote-grid">
            <div class="vote-cell">
              <div class="vote-label">在线</div>
              <div class="vote-bar"><div class="vote-yes" :style="{ flex: detail.votes.online.yes }">{{ detail.votes.online.yes }}</div><div class="vote-no" :style="{ flex: detail.votes.online.no }">{{ detail.votes.online.no }}</div></div>
            </div>
            <div class="vote-cell">
              <div class="vote-label">固件</div>
              <div class="vote-bar"><div class="vote-yes" :style="{ flex: detail.votes.firmware.yes }">{{ detail.votes.firmware.yes }}</div><div class="vote-no" :style="{ flex: detail.votes.firmware.no }">{{ detail.votes.firmware.no }}</div></div>
            </div>
            <div class="vote-cell">
              <div class="vote-label">证书</div>
              <div class="vote-bar"><div class="vote-yes" :style="{ flex: detail.votes.cert.yes }">{{ detail.votes.cert.yes }}</div><div class="vote-no" :style="{ flex: detail.votes.cert.no }">{{ detail.votes.cert.no }}</div></div>
            </div>
          </div>

          <h4 style="margin-top: 14px">每节点上报详情</h4>
          <a-table
            :columns="[
              { title: '节点', dataIndex: 'nodeName', width: 140 },
              { title: '在线', dataIndex: 'online', width: 60 },
              { title: '固件', dataIndex: 'firmwareValid', width: 60 },
              { title: '证书', dataIndex: 'certValid', width: 60 },
              { title: '分', dataIndex: 'score', width: 60 },
              { title: '多数派', dataIndex: 'inMajority', width: 80 },
              { title: '签名', dataIndex: 'signature' }
            ]"
            :dataSource="detail.submissions"
            :pagination="false"
            size="small"
            rowKey="id"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'online' || column.dataIndex === 'firmwareValid' || column.dataIndex === 'certValid'">
                <a-tag :color="record[column.dataIndex] ? 'green' : 'red'">{{ record[column.dataIndex] ? '✓' : '✗' }}</a-tag>
              </template>
              <template v-else-if="column.dataIndex === 'inMajority'">
                <a-tag :color="record.inMajority ? 'green' : 'orange'">{{ record.inMajority ? '是' : '否' }}</a-tag>
              </template>
              <template v-else-if="column.dataIndex === 'signature'">
                <span class="mono" style="font-size:11px">{{ shortHash(record.signature) }}</span>
              </template>
            </template>
          </a-table>
        </div>
      </a-spin>
    </a-modal>
  </a-space>
</template>

<style scoped>
.kpi-card {
  border-radius: 12px;
}

.panel-card {
  border-radius: 12px;
}

.topo-chart {
  width: 100%;
  height: 380px;
  background: linear-gradient(180deg, #f8fafc 0%, #eef2ff 100%);
  border-radius: 8px;
}

.small-chart {
  width: 100%;
  height: 200px;
}

.topo-legend {
  margin-top: 8px;
  font-size: 12px;
  color: #475569;
}

.dot {
  display: inline-block;
  width: 10px;
  height: 10px;
  border-radius: 999px;
  margin-right: 4px;
  vertical-align: middle;
}

.hint {
  color: #94a3b8;
  margin-left: 6px;
  font-size: 11px;
}

.sim-strip {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: 16px;
  padding: 12px;
  background: #f1f5f9;
  border-radius: 8px;
  flex-wrap: wrap;
}

.sim-step {
  padding: 6px 14px;
  border-radius: 999px;
  background: #e2e8f0;
  font-size: 12px;
  color: #475569;
  font-weight: 600;
  transition: all 0.3s;
}

.sim-step.sim-active {
  background: #3949d6;
  color: #fff;
  box-shadow: 0 0 0 4px rgba(57, 73, 214, 0.18);
}

.sim-step.sim-done {
  background: #16a34a;
  color: #fff;
}

.sim-arrow {
  color: #cbd5e1;
  font-weight: 700;
}

.vote-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.vote-cell {
  display: flex;
  align-items: center;
  gap: 10px;
}

.vote-label {
  width: 50px;
  font-size: 12px;
  color: #475569;
}

.vote-bar {
  flex: 1;
  display: flex;
  height: 22px;
  border-radius: 6px;
  overflow: hidden;
  font-size: 11px;
  color: #fff;
  font-weight: 600;
}

.vote-yes {
  background: #16a34a;
  display: flex;
  align-items: center;
  justify-content: center;
}

.vote-no {
  background: #dc2626;
  display: flex;
  align-items: center;
  justify-content: center;
}

.mono {
  font-family: var(--mono);
  color: #344054;
  word-break: break-all;
}
</style>
