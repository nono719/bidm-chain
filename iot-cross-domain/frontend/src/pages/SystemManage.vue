<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import dayjs from 'dayjs'
import { message as antdMessage } from 'ant-design-vue'
import { apiRequest, getUser } from '../api/client'

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')

const loading = ref(false)
const rows = ref([])

const nodeForm = reactive({
  nodeName: '',
  publicKey: ''
})
const rotateOpen = ref(false)
const rotateForm = reactive({
  id: 0,
  nodeName: '',
  publicKey: ''
})

const thresholdLoading = ref(false)
const threshold = ref(3)

function isPEMPublicKey(v) {
  const s = (v || '').trim()
  return s.startsWith('-----BEGIN PUBLIC KEY-----') && s.includes('-----END PUBLIC KEY-----')
}

async function generatePemPublicKey() {
  const keyPair = await crypto.subtle.generateKey(
    { name: 'ECDSA', namedCurve: 'P-256' },
    true,
    ['sign', 'verify']
  )
  const spki = await crypto.subtle.exportKey('spki', keyPair.publicKey)
  const base64 = btoa(String.fromCharCode(...new Uint8Array(spki)))
  const chunks = base64.match(/.{1,64}/g) || []
  return `-----BEGIN PUBLIC KEY-----\n${chunks.join('\n')}\n-----END PUBLIC KEY-----`
}

function fmt(t) {
  if (!t) return '-'
  return dayjs(t).format('YYYY-MM-DD HH:mm:ss')
}

async function loadNodes() {
  if (!isAdmin.value) return
  loading.value = true
  try {
    const res = await apiRequest('/api/system/oracle/nodes')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载节点失败')
      return
    }
    rows.value = Array.isArray(res.data) ? res.data : []
  } finally {
    loading.value = false
  }
}

async function addNode() {
  if (!nodeForm.nodeName.trim() || !nodeForm.publicKey.trim()) {
    antdMessage.warning('请填写节点名称和公钥')
    return
  }
  if (!isPEMPublicKey(nodeForm.publicKey)) {
    antdMessage.warning('公钥需为 PEM 公钥格式（BEGIN/END PUBLIC KEY）')
    return
  }
  const res = await apiRequest('/api/system/oracle/nodes', { method: 'POST', body: nodeForm })
  if (res.code !== 0) {
    antdMessage.error(res.message || '注册失败')
    return
  }
  antdMessage.success('预言机节点注册成功')
  nodeForm.nodeName = ''
  nodeForm.publicKey = ''
  await loadNodes()
}

async function fillNodePublicKey() {
  try {
    nodeForm.publicKey = await generatePemPublicKey()
    antdMessage.success('已生成示例 PEM 公钥，请保存对应私钥用于真实节点签名')
  } catch {
    antdMessage.error('浏览器不支持自动生成公钥，请手动粘贴 PEM 公钥')
  }
}

function openRotate(record) {
  rotateForm.id = record.id
  rotateForm.nodeName = record.nodeName
  rotateForm.publicKey = ''
  rotateOpen.value = true
}

async function doRotate() {
  if (!rotateForm.publicKey.trim()) {
    antdMessage.warning('请输入新公钥')
    return
  }
  if (!isPEMPublicKey(rotateForm.publicKey)) {
    antdMessage.warning('新公钥需为 PEM 公钥格式（BEGIN/END PUBLIC KEY）')
    return
  }
  const res = await apiRequest(`/api/system/oracle/nodes/${rotateForm.id}/rotate-key`, {
    method: 'POST',
    body: { publicKey: rotateForm.publicKey }
  })
  if (res.code !== 0) {
    antdMessage.error(res.message || '轮换失败')
    return
  }
  antdMessage.success('密钥轮换成功')
  rotateOpen.value = false
  await loadNodes()
}

async function fillRotatePublicKey() {
  try {
    rotateForm.publicKey = await generatePemPublicKey()
    antdMessage.success('已生成示例 PEM 公钥，请保存对应私钥用于真实节点签名')
  } catch {
    antdMessage.error('浏览器不支持自动生成公钥，请手动粘贴 PEM 公钥')
  }
}

async function loadThreshold() {
  if (!isAdmin.value) return
  thresholdLoading.value = true
  try {
    const res = await apiRequest('/api/system/oracle/threshold')
    if (res.code !== 0) return
    threshold.value = Number(res.data?.threshold || 3)
  } finally {
    thresholdLoading.value = false
  }
}

async function saveThreshold() {
  const res = await apiRequest('/api/system/oracle/threshold', { method: 'POST', body: { threshold: threshold.value } })
  if (res.code !== 0) {
    antdMessage.error(res.message || '保存失败')
    return
  }
  antdMessage.success('门限参数已更新')
  await loadThreshold()
}

onMounted(async () => {
  await loadNodes()
  await loadThreshold()
})
</script>

<template>
  <a-card class="panel-card" title="系统管理">
    <template v-if="!isAdmin">
      <a-result status="403" title="无权限" sub-title="仅管理员可访问系统管理能力" />
    </template>
    <template v-else>
      <a-row :gutter="16">
        <a-col :xs="24" :lg="12">
          <a-card class="inner-card" size="small" title="预言机节点注册">
            <a-form layout="vertical">
              <a-form-item label="节点名称">
                <a-input v-model:value="nodeForm.nodeName" placeholder="oracle-node-01" />
              </a-form-item>
              <a-form-item label="节点公钥">
                <a-textarea v-model:value="nodeForm.publicKey" :rows="4" placeholder="请输入 PEM 公钥（-----BEGIN PUBLIC KEY-----）" />
              </a-form-item>
            </a-form>
            <a-space>
              <a-button type="primary" @click="addNode">注册节点</a-button>
              <a-button @click="fillNodePublicKey">生成示例公钥</a-button>
              <a-button :loading="loading" @click="loadNodes">刷新列表</a-button>
            </a-space>
            <a-alert style="margin-top: 10px" type="info" show-icon message="节点公钥需为 PKIX PEM 格式，示例：BEGIN/END PUBLIC KEY。" />
          </a-card>
        </a-col>

        <a-col :xs="24" :lg="12">
          <a-card class="inner-card" size="small" title="门限参数调整">
            <a-form layout="vertical">
              <a-form-item label="门限值（Threshold）">
                <a-input-number v-model:value="threshold" :min="1" :max="100" style="width: 100%" />
              </a-form-item>
            </a-form>
            <a-space>
              <a-button type="primary" :loading="thresholdLoading" @click="saveThreshold">保存门限</a-button>
              <a-button :loading="thresholdLoading" @click="loadThreshold">刷新</a-button>
            </a-space>
          </a-card>
        </a-col>
      </a-row>

      <a-card class="inner-card" size="small" title="预言机节点列表" style="margin-top: 16px;">
        <a-table :dataSource="rows" rowKey="id" :loading="loading" size="small" :pagination="{ pageSize: 10 }">
          <a-table-column title="节点名称" dataIndex="nodeName" key="nodeName" />
          <a-table-column title="状态" key="status">
            <template #default="{ record }">
              <a-tag :color="record.status === 'ACTIVE' ? 'green' : 'default'">{{ record.status }}</a-tag>
            </template>
          </a-table-column>
          <a-table-column title="最后在线" key="lastSeen">
            <template #default="{ record }">
              {{ fmt(record.lastSeen) }}
            </template>
          </a-table-column>
          <a-table-column title="创建时间" key="createdAt">
            <template #default="{ record }">
              {{ fmt(record.createdAt) }}
            </template>
          </a-table-column>
          <a-table-column title="操作" key="action" width="140">
            <template #default="{ record }">
              <a-button size="small" @click="openRotate(record)">轮换密钥</a-button>
            </template>
          </a-table-column>
        </a-table>
      </a-card>
    </template>

    <a-modal
      v-model:open="rotateOpen"
      title="轮换预言机节点密钥"
      ok-text="提交"
      cancel-text="取消"
      @ok="doRotate"
    >
      <a-form layout="vertical">
        <a-form-item label="节点">
          <a-input :value="rotateForm.nodeName" disabled />
        </a-form-item>
        <a-form-item label="新公钥">
          <a-textarea v-model:value="rotateForm.publicKey" :rows="4" placeholder="请输入 PEM 公钥（-----BEGIN PUBLIC KEY-----）" />
        </a-form-item>
      </a-form>
      <a-space>
        <a-button @click="fillRotatePublicKey">生成示例公钥</a-button>
      </a-space>
    </a-modal>
  </a-card>
</template>
