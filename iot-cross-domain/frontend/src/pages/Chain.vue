<script setup>
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import * as echarts from 'echarts'
import { apiRequest } from '../api/client'

const info = ref(null)
const topology = ref(null)
const blocks = ref([])
const loadingInfo = ref(false)
const loadingTopo = ref(false)
const loadingBlocks = ref(false)

const verifyForm = ref({ mode: 'tx', value: '' })
const verifyLoading = ref(false)
const verifyResult = ref(null)

const blockDetailVisible = ref(false)
const blockDetail = ref(null)
const blockDetailLoading = ref(false)

const hashChain = ref([])
const loadingHashChain = ref(false)

const tamperState = ref({ tamperedCount: 0, items: [] })
const tamperLoading = ref(false)
const tamperSelectedAnchorId = ref(null)
const tamperBizRef = ref('')
const isAdmin = ref(false)

const chartRef = ref(null)
let chart = null
let timer = null

async function loadInfo() {
  loadingInfo.value = true
  try {
    const res = await apiRequest('/api/chain/info')
    if (res.code === 0) info.value = res.data
  } finally {
    loadingInfo.value = false
  }
}

async function loadTopology() {
  loadingTopo.value = true
  try {
    const res = await apiRequest('/api/chain/topology')
    if (res.code === 0) topology.value = res.data
  } finally {
    loadingTopo.value = false
  }
  await nextTick()
  renderTopology()
}

async function loadBlocks() {
  loadingBlocks.value = true
  try {
    const res = await apiRequest('/api/chain/blocks?limit=30')
    if (res.code === 0) blocks.value = res.data || []
  } finally {
    loadingBlocks.value = false
  }
}

async function loadHashChain() {
  loadingHashChain.value = true
  try {
    const res = await apiRequest('/api/chain/hashchain?limit=8')
    if (res.code === 0) hashChain.value = res.data?.blocks || []
  } finally {
    loadingHashChain.value = false
  }
}

async function loadTamperStatus() {
  if (!isAdmin.value) return
  const res = await apiRequest('/api/chain/demo/status')
  if (res.code === 0) tamperState.value = res.data
}

async function refreshAll() {
  await Promise.all([loadInfo(), loadTopology(), loadBlocks(), loadHashChain(), loadTamperStatus()])
}

async function detectRole() {
  const res = await apiRequest('/api/auth/me')
  if (res.code === 0) isAdmin.value = res.data?.role === 'ADMIN'
}

async function tamperAnchor() {
  if (!tamperSelectedAnchorId.value) {
    antdMessage.warning('请输入要篡改的 anchorId')
    return
  }
  tamperLoading.value = true
  try {
    const res = await apiRequest('/api/chain/demo/tamper', { method: 'POST', body: { anchorId: Number(tamperSelectedAnchorId.value) } })
    if (res.code === 0) {
      antdMessage.warning('已模拟篡改数据库，请用 BizRef 模式触发链上读验证查看结果')
      tamperBizRef.value = ''
      await Promise.all([loadTamperStatus(), loadBlocks()])
      // Auto-verify by anchor's bizRef if we can find it
      const anchor = blocks.value.flatMap((b) => b.anchors || []).find((a) => a.id === Number(tamperSelectedAnchorId.value))
      if (anchor) {
        verifyForm.value = { mode: 'biz', value: anchor.bizRef }
        await runVerify()
      }
    } else {
      antdMessage.error(res.message || '篡改失败')
    }
  } finally {
    tamperLoading.value = false
  }
}

async function restoreAnchor(anchorId) {
  tamperLoading.value = true
  try {
    const res = await apiRequest('/api/chain/demo/restore', { method: 'POST', body: { anchorId: Number(anchorId) } })
    if (res.code === 0) {
      antdMessage.success('已恢复，再次 verify 应显示一致')
      await Promise.all([loadTamperStatus(), loadBlocks()])
      const anchor = blocks.value.flatMap((b) => b.anchors || []).find((a) => a.id === Number(anchorId))
      if (anchor) {
        verifyForm.value = { mode: 'biz', value: anchor.bizRef }
        await runVerify()
      }
    } else {
      antdMessage.error(res.message || '恢复失败')
    }
  } finally {
    tamperLoading.value = false
  }
}

function renderTopology() {
  if (!chartRef.value || !topology.value) return
  if (chart && chart.getDom && chart.getDom() !== chartRef.value) {
    try { chart.dispose() } catch (_) {}
    chart = null
  }
  if (!chart) chart = echarts.init(chartRef.value)

  const t = topology.value.topology
  const channelId = `channel:${t.channel}`
  const ordererId = (o) => `orderer:${o.name}`
  const peerId = (p) => `peer:${p.name}`
  const orgId = (o) => `org:${o.mspId}`

  const nodes = []
  const links = []
  const categories = [
    { name: 'Channel' },
    { name: 'Orderer' },
    { name: 'Org' },
    { name: 'Peer' },
    { name: 'Chaincode' }
  ]

  nodes.push({
    id: channelId,
    name: t.channel,
    category: 0,
    symbolSize: 70,
    itemStyle: { color: '#182078' },
    label: { color: '#fff', fontWeight: 700 }
  })

  ;(t.chaincodes || []).forEach((cc) => {
    const id = `cc:${cc}`
    nodes.push({
      id,
      name: cc,
      category: 4,
      symbolSize: 44,
      itemStyle: { color: '#7f56d9' }
    })
    links.push({ source: channelId, target: id, lineStyle: { color: '#7f56d9', width: 2, type: 'dashed' } })
  })

  ;(t.orderers || []).forEach((o) => {
    const id = ordererId(o)
    nodes.push({
      id,
      name: o.name,
      category: 1,
      symbolSize: 56,
      itemStyle: { color: o.online ? '#f8c000' : '#a0a0a0' },
      label: { color: '#1f2937', fontWeight: 600 }
    })
    links.push({ source: channelId, target: id, lineStyle: { width: 2, color: '#f8c000' } })
  })

  ;(t.orgs || []).forEach((org) => {
    const oid = orgId(org)
    nodes.push({
      id: oid,
      name: `${org.displayName}\n(${org.mspId})`,
      category: 2,
      symbolSize: 60,
      itemStyle: { color: '#0e9384' },
      label: { color: '#fff', fontWeight: 600, fontSize: 11 }
    })
    links.push({ source: channelId, target: oid, lineStyle: { width: 2, color: '#0e9384' } })

    ;(org.peers || []).forEach((p) => {
      const pid = peerId(p)
      nodes.push({
        id: pid,
        name: p.name + (p.online ? '' : '\n(offline)'),
        category: 3,
        symbolSize: 44,
        itemStyle: { color: p.online ? '#16a34a' : '#9ca3af' }
      })
      links.push({ source: oid, target: pid, lineStyle: { color: p.online ? '#16a34a' : '#9ca3af' } })
    })
  })

  chart.setOption({
    backgroundColor: 'transparent',
    tooltip: {},
    legend: [{ data: categories.map((c) => c.name), top: 8 }],
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        draggable: true,
        categories,
        force: { repulsion: 600, edgeLength: 110, gravity: 0.08 },
        label: { show: true, position: 'bottom', fontSize: 11 },
        emphasis: { focus: 'adjacency', lineStyle: { width: 3 } },
        edgeSymbol: ['none', 'none'],
        lineStyle: { width: 1.4, opacity: 0.85 },
        data: nodes,
        links
      }
    ]
  })
}

function shortHash(v) {
  if (!v) return '-'
  return `${v.slice(0, 12)}...${v.slice(-8)}`
}

function fmtTime(v) {
  if (!v) return '-'
  return dayjs(v).format('YYYY-MM-DD HH:mm:ss')
}

function bizColor(t) {
  if (t === 'cross_auth') return 'blue'
  if (t === 'device_register') return 'green'
  if (t === 'oracle_report') return 'orange'
  return 'default'
}

async function openBlock(bh) {
  blockDetailVisible.value = true
  blockDetailLoading.value = true
  blockDetail.value = null
  try {
    const res = await apiRequest(`/api/chain/blocks/${bh}`)
    if (res.code === 0) blockDetail.value = res.data
  } finally {
    blockDetailLoading.value = false
  }
}

async function runVerify() {
  if (!verifyForm.value.value) {
    antdMessage.warning('请输入 TxHash 或业务参考号')
    return
  }
  verifyLoading.value = true
  verifyResult.value = null
  try {
    const body =
      verifyForm.value.mode === 'tx'
        ? { txHash: verifyForm.value.value.trim() }
        : { bizRef: verifyForm.value.value.trim() }
    const res = await apiRequest('/api/chain/verify', { method: 'POST', body })
    if (res.code === 0) {
      verifyResult.value = res.data
    } else {
      antdMessage.error(res.message || '验证失败')
    }
  } finally {
    verifyLoading.value = false
  }
}

const blockColumns = [
  { title: '区块号', dataIndex: 'blockHeight', key: 'blockHeight', width: 90 },
  { title: '交易数', dataIndex: 'txCount', key: 'txCount', width: 80 },
  { title: '业务类型', key: 'biz', width: 220 },
  { title: '首笔时间', dataIndex: 'firstAt', key: 'firstAt', width: 180 },
  { title: '操作', key: 'action', width: 120 }
]

const onlineRatio = computed(() => {
  if (!topology.value) return 0
  if (!topology.value.nodesTotal) return 0
  return Math.round((topology.value.nodesOnline / topology.value.nodesTotal) * 100)
})

onMounted(async () => {
  await detectRole()
  await refreshAll()
  timer = setInterval(refreshAll, 12000)
  window.addEventListener('resize', resizeChart)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  window.removeEventListener('resize', resizeChart)
  if (chart) {
    chart.dispose()
    chart = null
  }
})

function resizeChart() {
  if (chart) chart.resize()
}
</script>

<template>
  <a-space direction="vertical" size="middle" style="width: 100%">
    <a-row :gutter="16">
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="chain-kpi" :loading="loadingInfo">
          <a-statistic title="区块高度（链上实时）" :value="info?.height ?? '-'" />
          <div class="kpi-sub">通道：{{ info?.channel || '-' }}</div>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="chain-kpi" :loading="loadingInfo">
          <a-statistic title="累计上链记录" :value="info?.totalAnchors ?? 0" />
          <div class="kpi-sub">链码：{{ info?.chaincode || '-' }} / {{ info?.anchorFunc || '-' }}</div>
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="chain-kpi" :loading="loadingTopo">
          <a-statistic title="联盟节点在线" :value="topology?.nodesOnline ?? 0" :suffix="`/ ${topology?.nodesTotal ?? 0}`" />
          <a-progress :percent="onlineRatio" size="small" :showInfo="false" :strokeColor="onlineRatio === 100 ? '#16a34a' : '#f59e0b'" />
        </a-card>
      </a-col>
      <a-col :xs="24" :sm="12" :lg="6">
        <a-card class="chain-kpi" :loading="loadingInfo">
          <a-statistic title="最近上链时间" :value="info?.lastAnchorAt ? fmtTime(info.lastAnchorAt) : '-'" :valueStyle="{ fontSize: '14px' }" />
          <div class="kpi-sub mono">{{ shortHash(info?.lastTxHash) }}</div>
        </a-card>
      </a-col>
    </a-row>

    <a-card title="联盟拓扑（Channel ↔ Orderer ↔ Org ↔ Peer ↔ Chaincode）" class="panel-card">
      <template #extra>
        <a-tag v-if="info?.queryOk" color="green">链上读：在线</a-tag>
        <a-tag v-else color="red">链上读：失败</a-tag>
        <a-button type="link" size="small" @click="refreshAll" :loading="loadingTopo">刷新</a-button>
      </template>
      <a-spin :spinning="loadingTopo && !topology">
        <div ref="chartRef" class="topo-chart"></div>
      </a-spin>
      <div class="topo-legend">
        <a-space wrap size="small">
          <span><span class="dot" style="background:#182078"></span>Channel</span>
          <span><span class="dot" style="background:#f8c000"></span>Orderer</span>
          <span><span class="dot" style="background:#0e9384"></span>Org（联盟成员）</span>
          <span><span class="dot" style="background:#16a34a"></span>Peer（在线）</span>
          <span><span class="dot" style="background:#9ca3af"></span>Peer（离线）</span>
          <span><span class="dot" style="background:#7f56d9"></span>Chaincode</span>
        </a-space>
      </div>
    </a-card>

    <a-card title="区块哈希链（每块的 PreviousHash 指向前一块的 DataHash，构成不可断链）" class="panel-card">
      <template #extra>
        <a-tag color="purple">{{ hashChain.length }} 个最新区块</a-tag>
      </template>
      <a-spin :spinning="loadingHashChain && !hashChain.length">
        <div class="hashchain-strip">
          <template v-for="(b, i) in hashChain" :key="b.number">
            <div class="hashchain-block">
              <div class="hashchain-num">#{{ b.number }}</div>
              <div class="hashchain-tx">{{ b.txCount }} tx</div>
              <div class="hashchain-row">
                <span class="hashchain-label">DataHash</span>
                <span class="hashchain-hash mono">{{ shortHash(b.dataHash) }}</span>
              </div>
              <div class="hashchain-row">
                <span class="hashchain-label">PrevHash</span>
                <span class="hashchain-hash mono prev">{{ shortHash(b.previousHash) }}</span>
              </div>
            </div>
            <div v-if="i < hashChain.length - 1" class="hashchain-arrow" :title="'PrevHash 指向 #' + hashChain[i+1].number">←</div>
          </template>
          <div v-if="!hashChain.length && !loadingHashChain" class="hashchain-empty">暂无区块</div>
        </div>
      </a-spin>
    </a-card>

    <a-row :gutter="16">
      <a-col :xs="24" :lg="14">
        <a-card title="区块列表（按 BlockHeight 倒序，含每块的业务锚定）" :loading="loadingBlocks" class="panel-card">
          <a-table
            :columns="blockColumns"
            :dataSource="blocks"
            size="small"
            :pagination="{ pageSize: 10 }"
            rowKey="blockHeight"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.key === 'biz'">
                <a-space wrap size="small">
                  <a-tag v-for="(a, idx) in record.anchors" :key="idx" :color="bizColor(a.bizType)" style="cursor:pointer" :title="'anchorId=' + a.id" @click="isAdmin && (tamperSelectedAnchorId = a.id)">{{ a.bizType }} #{{ a.id }}</a-tag>
                </a-space>
              </template>
              <template v-else-if="column.key === 'firstAt'">{{ fmtTime(record.firstAt) }}</template>
              <template v-else-if="column.key === 'action'">
                <a-button type="link" size="small" @click="openBlock(record.blockHeight)">查看</a-button>
              </template>
            </template>
          </a-table>
        </a-card>
      </a-col>

      <a-col :xs="24" :lg="10">
        <a-card title="链上读验证（数据库 vs Fabric Gateway）" class="panel-card">
          <a-alert
            type="info"
            showIcon
            style="margin-bottom: 12px"
            message="输入交易哈希或业务参考号，后端会同时从数据库读副本、从 Fabric qscc 读链上原始记录，对比一致性。"
          />
          <a-radio-group v-model:value="verifyForm.mode" buttonStyle="solid" size="small" style="margin-bottom: 8px">
            <a-radio-button value="tx">按 TxHash</a-radio-button>
            <a-radio-button value="biz">按 BizRef</a-radio-button>
          </a-radio-group>
          <a-input
            v-model:value="verifyForm.value"
            :placeholder="verifyForm.mode === 'tx' ? '64 位 hex 交易哈希' : '业务参考号（如 DID、RequestID）'"
            allowClear
          />
          <div style="margin-top: 10px; display: flex; gap: 8px">
            <a-button type="primary" :loading="verifyLoading" @click="runVerify">开始验证</a-button>
            <a-button :disabled="!info?.lastTxHash" @click="verifyForm.mode='tx'; verifyForm.value=info?.lastTxHash; runVerify()">用最新交易试一下</a-button>
          </div>

          <a-divider style="margin: 16px 0 12px" />

          <a-empty v-if="!verifyResult" description="暂无验证结果" />
          <div v-else>
            <a-alert
              :type="verifyResult.consistent ? 'success' : 'error'"
              :message="verifyResult.consistent ? '✓ 一致：数据库副本与链上记录吻合' : (verifyResult.found ? '✗ 不一致：链上记录与数据库不匹配（链上为准）' : '未找到对应锚定记录')"
              showIcon
              style="margin-bottom: 10px"
            />
            <a-descriptions v-if="verifyResult.anchor" size="small" :column="1" bordered>
              <a-descriptions-item label="业务类型">{{ verifyResult.anchor.bizType }}</a-descriptions-item>
              <a-descriptions-item label="业务参考">
                <span class="mono">{{ verifyResult.anchor.bizRef }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="区块高度">{{ verifyResult.anchor.blockHeight }}</a-descriptions-item>
              <a-descriptions-item label="TxHash"><span class="mono">{{ verifyResult.anchor.txHash }}</span></a-descriptions-item>
              <a-descriptions-item v-if="verifyResult.chainTx" label="链上验证码">
                <a-tag :color="verifyResult.chainTx.valid ? 'green' : 'red'">{{ verifyResult.chainTx.validationMessage }}</a-tag>
                <span style="margin-left:6px">code={{ verifyResult.chainTx.validationCode }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="DB digest">
                <span class="mono" :style="verifyResult.digestMatch ? '' : 'color:#dc2626;font-weight:600'">{{ verifyResult.digestInDB || '-' }}</span>
              </a-descriptions-item>
              <a-descriptions-item label="链上 digest">
                <span class="mono" :style="verifyResult.digestMatch ? 'color:#16a34a' : 'color:#16a34a;font-weight:600'">{{ verifyResult.digestOnChain || '-' }}</span>
              </a-descriptions-item>
              <a-descriptions-item v-if="verifyResult.chainError" label="链上读错误">
                <span style="color:#dc2626">{{ verifyResult.chainError }}</span>
              </a-descriptions-item>
            </a-descriptions>

            <div v-if="verifyResult.chainTx?.endorsers?.length" style="margin-top: 12px">
              <div class="section-title">多组织背书 <a-tag color="purple" style="margin-left:6px">{{ verifyResult.chainTx.endorsers.length }} 个 Org 共同签名</a-tag></div>
              <div class="endorser-list">
                <div v-for="(e, i) in verifyResult.chainTx.endorsers" :key="i" class="endorser-item">
                  <div class="endorser-msp">
                    <a-tag :color="e.mspId === 'Org1MSP' ? 'green' : (e.mspId === 'Org2MSP' ? 'cyan' : 'blue')">{{ e.mspId }}</a-tag>
                    <span class="endorser-cn">{{ e.commonName || '(unknown)' }}</span>
                  </div>
                  <div class="endorser-meta">
                    <span class="endorser-meta-item">CA: {{ e.issuer || '-' }}</span>
                    <span class="endorser-meta-item">证书序列号: {{ e.serial?.slice(0, 12) }}…</span>
                  </div>
                  <div class="endorser-sig mono">签名: {{ e.sigShort || '-' }}</div>
                </div>
              </div>
              <div v-if="verifyResult.chainTx.creator" class="creator-info">
                提案发起者: <a-tag color="blue">{{ verifyResult.chainTx.creator.mspId }}</a-tag>
                <span class="mono" style="margin-left:4px">{{ verifyResult.chainTx.creator.commonName }}</span>
              </div>
            </div>
          </div>
        </a-card>

        <a-card v-if="isAdmin" title="🧪 篡改对比演示（证明链上不可篡改）" class="panel-card" style="margin-top: 16px">
          <a-alert
            type="warning"
            showIcon
            style="margin-bottom: 12px"
            :message="`已篡改 anchor 数：${tamperState.tamperedCount}（仅修改数据库，链上不动）`"
          />
          <div class="tamper-help">
            选择一个区块列表中的 anchorId（左侧表格"查看"里能看到 ID），点"模拟篡改"会偷偷把数据库 digest 改掉，链上不变；然后自动 verify 显示"✗ 不一致"。再点"恢复"还原。
          </div>
          <div style="display: flex; gap: 8px; margin-top: 8px; flex-wrap: wrap">
            <a-input-number v-model:value="tamperSelectedAnchorId" placeholder="anchorId" style="width: 130px" :min="1" />
            <a-button danger :loading="tamperLoading" @click="tamperAnchor">模拟篡改数据库</a-button>
          </div>
          <a-divider v-if="tamperState.items?.length" style="margin: 12px 0" />
          <div v-if="tamperState.items?.length">
            <div class="section-title">当前已被篡改的记录</div>
            <a-table
              size="small"
              :pagination="false"
              :dataSource="tamperState.items"
              :columns="[
                { title: 'AnchorID', dataIndex: 'anchorId', width: 90 },
                { title: '原 digest', dataIndex: 'originalDigest', ellipsis: true },
                { title: '操作', key: 'op', width: 90 }
              ]"
              rowKey="anchorId"
            >
              <template #bodyCell="{ column, record }">
                <template v-if="column.dataIndex === 'originalDigest'">
                  <span class="mono">{{ shortHash(record.originalDigest) }}</span>
                </template>
                <template v-else-if="column.key === 'op'">
                  <a-button type="link" size="small" :loading="tamperLoading" @click="restoreAnchor(record.anchorId)">恢复</a-button>
                </template>
              </template>
            </a-table>
          </div>
        </a-card>
      </a-col>
    </a-row>

    <a-modal
      v-model:open="blockDetailVisible"
      :title="blockDetail ? `区块 #${blockDetail.blockHeight}` : '区块详情'"
      width="720px"
      :footer="null"
    >
      <a-spin :spinning="blockDetailLoading">
        <div v-if="blockDetail">
          <a-descriptions size="small" :column="2" bordered>
            <a-descriptions-item label="区块号">{{ blockDetail.blockHeight }}</a-descriptions-item>
            <a-descriptions-item label="链上交易数">{{ blockDetail.block?.txCount ?? '-' }}</a-descriptions-item>
            <a-descriptions-item label="DataHash" :span="2"><span class="mono">{{ blockDetail.block?.dataHash || '-' }}</span></a-descriptions-item>
            <a-descriptions-item label="PrevHash" :span="2"><span class="mono">{{ blockDetail.block?.previousHash || '-' }}</span></a-descriptions-item>
            <a-descriptions-item v-if="blockDetail.queryError" label="链上读错误" :span="2">
              <span style="color:#dc2626">{{ blockDetail.queryError }}</span>
            </a-descriptions-item>
          </a-descriptions>

          <h4 style="margin-top:16px">本区块的业务锚定</h4>
          <a-table
            :columns="[
              { title: '业务类型', dataIndex: 'bizType', width: 130 },
              { title: '业务参考', dataIndex: 'bizRef' },
              { title: '摘要', dataIndex: 'digest', width: 220 },
              { title: 'TxHash', dataIndex: 'txHash', width: 220 }
            ]"
            :dataSource="blockDetail.anchors || []"
            size="small"
            :pagination="false"
            rowKey="id"
          >
            <template #bodyCell="{ column, record }">
              <template v-if="column.dataIndex === 'bizType'">
                <a-tag :color="bizColor(record.bizType)">{{ record.bizType }}</a-tag>
              </template>
              <template v-else-if="column.dataIndex === 'digest' || column.dataIndex === 'txHash'">
                <span class="mono">{{ shortHash(record[column.dataIndex]) }}</span>
              </template>
            </template>
          </a-table>
        </div>
      </a-spin>
    </a-modal>
  </a-space>
</template>

<style scoped>
.chain-kpi {
  border-radius: 12px;
}

.kpi-sub {
  margin-top: 6px;
  color: var(--muted);
  font-size: 12px;
}

.panel-card {
  border-radius: 12px;
}

.topo-chart {
  width: 100%;
  height: 460px;
  background: linear-gradient(180deg, #f8fafc 0%, #eef2ff 100%);
  border-radius: 8px;
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

.mono {
  font-family: var(--mono);
  color: #344054;
  word-break: break-all;
}

.section-title {
  font-size: 13px;
  font-weight: 600;
  color: #1f2937;
  margin: 4px 0 8px;
}

.endorser-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.endorser-item {
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 8px 10px;
  background: linear-gradient(180deg, #fafbff 0%, #f3f5fb 100%);
}

.endorser-msp {
  display: flex;
  align-items: center;
  gap: 6px;
}

.endorser-cn {
  font-size: 13px;
  color: #1e293b;
  font-weight: 500;
}

.endorser-meta {
  display: flex;
  gap: 12px;
  font-size: 11px;
  color: #64748b;
  margin-top: 2px;
}

.endorser-sig {
  font-size: 11px;
  color: #475569;
  margin-top: 2px;
}

.creator-info {
  margin-top: 10px;
  padding: 6px 8px;
  background: #eef2ff;
  border-radius: 6px;
  font-size: 12px;
}

.tamper-help {
  font-size: 12px;
  color: #475569;
  background: #fff7ed;
  padding: 8px 10px;
  border-radius: 6px;
  border-left: 3px solid #f59e0b;
  line-height: 1.6;
}

.hashchain-strip {
  display: flex;
  align-items: stretch;
  gap: 6px;
  overflow-x: auto;
  padding: 4px 2px 12px;
}

.hashchain-block {
  flex-shrink: 0;
  width: 168px;
  border: 1px solid #c7d2fe;
  border-radius: 10px;
  padding: 10px;
  background: linear-gradient(180deg, #eef2ff 0%, #f5f3ff 100%);
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.hashchain-num {
  font-size: 16px;
  font-weight: 700;
  color: #182078;
}

.hashchain-tx {
  font-size: 11px;
  color: #475569;
  margin-top: -2px;
}

.hashchain-row {
  display: flex;
  flex-direction: column;
  margin-top: 4px;
}

.hashchain-label {
  font-size: 10px;
  color: #64748b;
  letter-spacing: 0.3px;
  text-transform: uppercase;
}

.hashchain-hash {
  font-size: 11px;
  color: #1f2937;
}

.hashchain-hash.prev {
  color: #7c3aed;
}

.hashchain-arrow {
  display: flex;
  align-items: center;
  font-size: 22px;
  color: #7c3aed;
  font-weight: 700;
  user-select: none;
}

.hashchain-empty {
  padding: 20px;
  color: #94a3b8;
  text-align: center;
  width: 100%;
}
</style>
