<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import dayjs from 'dayjs'
import { apiRequest, getUser } from '../api/client'
import { DEVICE_TYPE_FILTER_OPTIONS, DEVICE_TYPE_OPTIONS } from '../constants/deviceTypes'
import DeviceMetadataForm from '../components/DeviceMetadataForm.vue'

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const loading = ref(false)
const devices = ref([])
const keyword = ref('')
const typeFilter = ref('ALL')

const editOpen = ref(false)
const editLoading = ref(false)
const editTarget = ref(null)
const resolveOpen = ref(false)
const resolveInfo = ref(null)
const editForm = reactive({
  displayName: '',
  deviceType: '传感器',
  metadataJson: ''
})

function fmt(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

function typeToReadable(v) {
  const mapping = {
    sensor: '传感器',
    actuator: '执行器',
    gateway: '网关',
    camera: '摄像头',
    'access-control': '门禁设备',
    'env-monitor': '环境监测设备',
    plc: '工业控制器',
    'smart-meter': '智能电表',
    'smart-appliance': '智能家电',
    'vehicle-terminal': '车载终端',
    other: '其他'
  }
  return mapping[v] || v || '-'
}

function didReadable(v) {
  if (!v) return '-'
  const parts = String(v).split(':')
  if (parts.length < 6 || parts[0] !== 'did' || parts[1] !== 'iot') return '-'
  return `物联网：${parts[2]}：${typeToReadable(parts[3])}：${parts.slice(4).join(':')}`
}

async function loadDevices() {
  loading.value = true
  try {
    const params = new URLSearchParams()
    if (keyword.value.trim()) params.set('keyword', keyword.value.trim())
    if (typeFilter.value !== 'ALL') params.set('deviceType', typeFilter.value)
    const query = params.toString()
    const res = await apiRequest(`/api/devices${query ? `?${query}` : ''}`)
    if (res.code !== 0) {
      antdMessage.error(res.message || '设备查询失败')
      return
    }
    devices.value = res.data
  } finally {
    loading.value = false
  }
}

function openEdit(record) {
  editTarget.value = record
  editForm.displayName = record.displayName || ''
  editForm.deviceType = record.deviceType || '传感器'
  editForm.metadataJson = record.metadataJson || ''
  editOpen.value = true
}

async function saveEdit() {
  if (!editTarget.value) return
  editLoading.value = true
  try {
    const res = await apiRequest(`/api/devices/${editTarget.value.id}`, {
      method: 'PUT',
      body: {
        displayName: editForm.displayName,
        deviceType: editForm.deviceType,
        metadataJson: editForm.metadataJson
      }
    })
    if (res.code !== 0) {
      antdMessage.error(res.message || '更新失败')
      return
    }
    antdMessage.success('设备信息已更新')
    editOpen.value = false
    await loadDevices()
  } finally {
    editLoading.value = false
  }
}

async function removeDevice(record) {
  const res = await apiRequest(`/api/devices/${record.id}`, { method: 'DELETE' })
  if (res.code !== 0) {
    antdMessage.error(res.message || '删除失败')
    return
  }
  antdMessage.success('设备已删除')
  await loadDevices()
}

async function updateLifecycle(record, action) {
  const res = await apiRequest(`/api/devices/${record.id}/lifecycle`, { method: 'POST', body: { action } })
  if (res.code !== 0) {
    antdMessage.error(res.message || '生命周期更新失败')
    return
  }
  antdMessage.success(`设备已${action}`)
  await loadDevices()
}

async function resolveDid(record) {
  const res = await apiRequest(`/api/devices/resolve?did=${encodeURIComponent(record.deviceDid)}`)
  if (res.code !== 0) {
    antdMessage.error(res.message || 'DID 解析失败')
    return
  }
  resolveInfo.value = res.data
  resolveOpen.value = true
}

const columns = [
  { title: '设备名称', dataIndex: 'displayName', key: 'displayName', width: 180 },
  { title: '设备DID', dataIndex: 'deviceDid', key: 'deviceDid', width: 240 },
  { title: '设备类型', dataIndex: 'deviceType', key: 'deviceType', width: 140 },
  { title: '所属域', dataIndex: 'domainCode', key: 'domainCode', width: 120 },
  { title: '生命周期', dataIndex: 'lifecycle', key: 'lifecycle', width: 120 },
  { title: '状态', dataIndex: 'runtimeState', key: 'runtimeState', width: 120 },
  { title: '创建时间', dataIndex: 'createdAt', key: 'createdAt', width: 180 },
  { title: '操作', key: 'action', width: 160 }
]

onMounted(loadDevices)
</script>

<template>
  <a-card class="panel-card" title="设备管理">
    <a-space style="margin-bottom: 12px; width: 100%" wrap>
      <a-input v-model:value="keyword" placeholder="搜索设备名称 / DID" style="width: 280px" @pressEnter="loadDevices" />
      <a-select v-model:value="typeFilter" :options="DEVICE_TYPE_FILTER_OPTIONS" style="width: 220px" />
      <a-button type="primary" :loading="loading" @click="loadDevices">查询</a-button>
    </a-space>

    <a-card class="inner-card" size="small">
      <a-table :columns="columns" :dataSource="devices" rowKey="id" size="small" :loading="loading" :pagination="{ pageSize: 10 }">
        <template #bodyCell="{ column, record }">
          <template v-if="column.key === 'deviceDid'">
            <div class="did-cell">
              <span class="mono raw">{{ record.deviceDid }}</span>
              <span class="readable">{{ didReadable(record.deviceDid) }}</span>
            </div>
          </template>
          <template v-else-if="column.key === 'domainCode'">
            <a-tag color="geekblue">{{ record.domainCode }}</a-tag>
          </template>
          <template v-else-if="column.key === 'lifecycle'">
            <a-tag :color="record.lifecycle === 'ACTIVE' ? 'green' : record.lifecycle === 'FROZEN' ? 'orange' : 'red'">
              {{ record.lifecycle || 'ACTIVE' }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'runtimeState'">
            <a-tag :color="record.runtimeState === 'TRUSTED' ? 'green' : record.runtimeState === 'RISKY' ? 'orange' : 'default'">
              {{ record.runtimeState }}
            </a-tag>
          </template>
          <template v-else-if="column.key === 'createdAt'">
            {{ fmt(record.createdAt) }}
          </template>
          <template v-else-if="column.key === 'action'">
            <a-space>
              <a-button size="small" @click="resolveDid(record)">DID解析</a-button>
              <a-button size="small" @click="openEdit(record)">编辑</a-button>
              <a-button size="small" :disabled="record.lifecycle === 'FROZEN'" @click="updateLifecycle(record, 'FREEZE')">冻结</a-button>
              <a-button size="small" :disabled="record.lifecycle === 'ACTIVE'" @click="updateLifecycle(record, 'REACTIVATE')">激活</a-button>
              <a-button size="small" danger :disabled="record.lifecycle === 'REVOKED'" @click="updateLifecycle(record, 'REVOKE')">吊销</a-button>
              <a-popconfirm title="确认删除该设备？" ok-text="删除" cancel-text="取消" @confirm="removeDevice(record)">
                <a-button size="small" danger>删除</a-button>
              </a-popconfirm>
            </a-space>
          </template>
        </template>
      </a-table>
    </a-card>

    <a-modal
      v-model:open="editOpen"
      title="编辑设备"
      ok-text="保存"
      cancel-text="取消"
      :confirmLoading="editLoading"
      @ok="saveEdit"
    >
      <a-form layout="vertical">
        <a-form-item label="设备名称">
          <a-input v-model:value="editForm.displayName" />
        </a-form-item>
        <a-form-item label="设备类型">
          <a-select v-model:value="editForm.deviceType" :options="DEVICE_TYPE_OPTIONS" />
        </a-form-item>
        <a-form-item label="设备元数据">
          <DeviceMetadataForm v-model="editForm.metadataJson" :device-type="editForm.deviceType" />
        </a-form-item>
        <a-alert v-if="!isAdmin" type="info" showIcon message="域管理员仅能管理本域设备" />
      </a-form>
    </a-modal>

    <a-modal v-model:open="resolveOpen" title="DID 解析结果" :footer="null">
      <a-descriptions size="small" :column="1" bordered>
        <a-descriptions-item label="设备DID"><span class="mono">{{ resolveInfo?.deviceDid || '-' }}</span></a-descriptions-item>
        <a-descriptions-item label="所属域">{{ resolveInfo?.domainCode || '-' }}</a-descriptions-item>
        <a-descriptions-item label="设备类型">{{ resolveInfo?.deviceType || '-' }}</a-descriptions-item>
        <a-descriptions-item label="生命周期">{{ resolveInfo?.lifecycle || '-' }}</a-descriptions-item>
        <a-descriptions-item label="运行状态">{{ resolveInfo?.runtimeState || '-' }}</a-descriptions-item>
        <a-descriptions-item label="注册时间">{{ fmt(resolveInfo?.registerAt) }}</a-descriptions-item>
        <a-descriptions-item label="注册交易哈希"><span class="mono">{{ resolveInfo?.registerTxHash || '-' }}</span></a-descriptions-item>
      </a-descriptions>
    </a-modal>
  </a-card>
</template>

<style scoped>
.mono {
  font-family: var(--mono);
}
.did-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.raw {
  color: #111827;
}
.readable {
  color: #667085;
  font-size: 12px;
}
</style>
