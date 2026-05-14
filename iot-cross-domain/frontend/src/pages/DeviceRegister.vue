<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { message as antdMessage } from 'ant-design-vue'
import { apiRequest, getUser } from '../api/client'
import { DEVICE_TYPE_OPTIONS } from '../constants/deviceTypes'
import DeviceMetadataForm from '../components/DeviceMetadataForm.vue'

const step = ref(0)
const submitting = ref(false)
const created = ref(null)

const form = reactive({
  deviceDid: '',
  domainCode: 'domain-a',
  displayName: '温湿度传感器',
  deviceType: '传感器',
  credential: 'dev-secret-001',
  metadataJson: '{"model":"TH-01","firmware":"1.0.0"}'
})

const me = ref(getUser())
const isAdmin = computed(() => me.value?.role === 'ADMIN')
const domainOptions = ref([])
const domainLoading = ref(false)
if (me.value?.domainCode && !isAdmin.value) {
  form.domainCode = me.value.domainCode
}
ensureDid()
watch(
  () => [form.domainCode, form.deviceType],
  () => {
    form.deviceDid = buildDid()
  }
)

async function loadDomains() {
  domainLoading.value = true
  try {
    const res = await apiRequest('/api/domains')
    if (res.code !== 0) {
      antdMessage.error(res.message || '加载域列表失败')
      return
    }
    const rows = Array.isArray(res.data) ? res.data : []
    domainOptions.value = rows.map((d) => ({ value: d.code, label: `${d.name}（${d.code}）` }))
    if (isAdmin.value) {
      const exists = domainOptions.value.some((o) => o.value === form.domainCode)
      if (!exists && domainOptions.value.length > 0) {
        form.domainCode = domainOptions.value[0].value
      }
    }
  } finally {
    domainLoading.value = false
  }
}
onMounted(loadDomains)

const pubKey = ref(null)
const privKey = ref(null)
const pubKeyJwk = ref('')

function randomHex(bytes) {
  const buf = new Uint8Array(bytes)
  crypto.getRandomValues(buf)
  return Array.from(buf)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
}

function deviceTypeToDidType(deviceType) {
  const mapping = {
    传感器: 'sensor',
    执行器: 'actuator',
    网关: 'gateway',
    摄像头: 'camera',
    门禁设备: 'access-control',
    环境监测设备: 'env-monitor',
    工业控制器: 'plc',
    智能电表: 'smart-meter',
    智能家电: 'smart-appliance',
    车载终端: 'vehicle-terminal',
    其他: 'other'
  }
  return mapping[deviceType] || 'device'
}

function buildDid() {
  const domain = String(form.domainCode || 'domain').trim().toLowerCase()
  const type = deviceTypeToDidType(form.deviceType)
  const id = `${Date.now().toString(36)}${randomHex(4)}`
  return `did:iot:${domain}:${type}:${id}`
}

function ensureDid() {
  if (form.deviceDid) return
  form.deviceDid = buildDid()
}

function regenerateDid() {
  form.deviceDid = buildDid()
  antdMessage.success('已自动生成新的设备DID')
}

async function genKeyPair() {
  ensureDid()
  const keys = await crypto.subtle.generateKey(
    { name: 'ECDSA', namedCurve: 'P-256' },
    true,
    ['sign', 'verify']
  )
  pubKey.value = keys.publicKey
  privKey.value = keys.privateKey
  const jwk = await crypto.subtle.exportKey('jwk', keys.publicKey)
  pubKeyJwk.value = JSON.stringify(jwk, null, 2)
  antdMessage.success('密钥对已生成（私钥仅保留在浏览器内存中）')
}

const previewObj = computed(() => {
  let meta = null
  try {
    meta = JSON.parse(form.metadataJson)
  } catch {
    meta = { raw: form.metadataJson }
  }
  let pub = null
  try {
    pub = pubKeyJwk.value ? JSON.parse(pubKeyJwk.value) : null
  } catch {
    pub = pubKeyJwk.value
  }
  return {
    did: form.deviceDid,
    domain: form.domainCode,
    displayName: form.displayName,
    deviceType: form.deviceType,
    publicKey: pub,
    metadata: meta
  }
})

const metadataItems = computed(() => {
  try {
    const obj = JSON.parse(form.metadataJson)
    if (!obj || typeof obj !== 'object') return []
    return Object.keys(obj).map((k) => ({ k, v: String(obj[k]) }))
  } catch {
    return []
  }
})

const pubKeyInfo = computed(() => {
  try {
    const jwk = pubKeyJwk.value ? JSON.parse(pubKeyJwk.value) : null
    if (!jwk) return null
    return {
      kty: jwk.kty,
      crv: jwk.crv,
      x: jwk.x,
      y: jwk.y
    }
  } catch {
    return null
  }
})

async function submitRegister() {
  if (!form.deviceDid || !form.domainCode || !form.displayName || !form.credential) {
    antdMessage.warning('请完整填写基本信息')
    return
  }
  submitting.value = true
  try {
    const res = await apiRequest('/api/devices', {
      method: 'POST',
      body: {
        deviceDid: form.deviceDid,
        domainCode: form.domainCode,
        displayName: form.displayName,
        credential: form.credential,
        deviceType: form.deviceType,
        publicKeyJwk: pubKeyJwk.value || '',
        metadataJson: form.metadataJson
      }
    })
    if (res.code !== 0) {
      antdMessage.error(res.message || '注册失败')
      return
    }
    created.value = res.data
    step.value = 3
    antdMessage.success('链上锚定完成，设备注册成功')
  } finally {
    submitting.value = false
  }
}

const steps = [
  { title: '基本信息' },
  { title: '密钥对生成' },
  { title: '链上注册' },
  { title: '完成确认' }
]

function next() {
  if (step.value === 0) {
    ensureDid()
  }
  if (step.value === 1 && !pubKey.value) {
    antdMessage.warning('请先生成密钥对')
    return
  }
  step.value = Math.min(3, step.value + 1)
}

function prev() {
  step.value = Math.max(0, step.value - 1)
}
</script>

<template>
  <a-card class="panel-card" title="设备身份注册（图5-2）">
    <a-steps :current="step" :items="steps" />

    <a-row :gutter="16" style="margin-top: 16px">
      <a-col :xs="24" :lg="12">
        <a-card class="inner-card" title="操作区" size="small">
          <template v-if="step === 0">
            <a-form layout="vertical">
              <a-form-item label="设备DID">
                <a-input-group compact>
                  <a-input v-model:value="form.deviceDid" readonly placeholder="系统自动生成" style="width: calc(100% - 110px)" />
                  <a-button style="width: 110px" @click="regenerateDid">重新生成</a-button>
                </a-input-group>
                <div style="margin-top: 6px; color: #667085; font-size: 12px;">
                  DID 格式：did:iot:&lt;domain&gt;:&lt;type&gt;:&lt;id&gt;
                </div>
              </a-form-item>
              <a-form-item label="所属域">
                <a-select
                  v-model:value="form.domainCode"
                  :disabled="!isAdmin"
                  :loading="domainLoading"
                  :options="domainOptions"
                />
              </a-form-item>
              <a-form-item label="设备名称">
                <a-input v-model:value="form.displayName" placeholder="温湿度传感器" />
              </a-form-item>
              <a-form-item label="设备类型">
                <a-select v-model:value="form.deviceType" :options="DEVICE_TYPE_OPTIONS" />
              </a-form-item>
              <a-form-item label="设备凭证（演示用）">
                <a-input v-model:value="form.credential" placeholder="dev-secret-001" />
              </a-form-item>
              <a-form-item label="设备元数据">
                <DeviceMetadataForm v-model="form.metadataJson" :device-type="form.deviceType" />
              </a-form-item>
            </a-form>
          </template>

          <template v-else-if="step === 1">
            <a-space direction="vertical" style="width: 100%" size="middle">
              <a-alert type="info" showIcon message="在客户端生成密钥对；私钥不出浏览器" />
              <a-button type="primary" @click="genKeyPair">生成密钥对</a-button>
              <a-descriptions size="small" :column="1" bordered>
                <a-descriptions-item label="算法">ECDSA / P-256</a-descriptions-item>
                <a-descriptions-item label="公钥类型">{{ pubKeyInfo?.kty || '-' }}</a-descriptions-item>
                <a-descriptions-item label="曲线">{{ pubKeyInfo?.crv || '-' }}</a-descriptions-item>
                <a-descriptions-item label="X"> <span class="mono">{{ pubKeyInfo?.x || '-' }}</span> </a-descriptions-item>
                <a-descriptions-item label="Y"> <span class="mono">{{ pubKeyInfo?.y || '-' }}</span> </a-descriptions-item>
              </a-descriptions>
            </a-space>
          </template>

          <template v-else-if="step === 2">
            <a-space direction="vertical" style="width: 100%" size="middle">
              <a-alert
                type="warning"
                showIcon
                message="将执行：写入数据库 + Fabric 联盟链锚定（用于审计追溯）"
              />
              <a-button type="primary" :loading="submitting" @click="submitRegister">提交链上注册</a-button>
            </a-space>
          </template>

          <template v-else>
            <a-result status="success" title="注册完成">
              <template #subTitle>
                <div class="mono">DeviceDID: {{ created?.device?.deviceDid || created?.deviceDid || form.deviceDid }}</div>
                <div class="mono">TxHash: {{ created?.anchor?.txHash || created?.txHash || '-' }}</div>
                <div class="mono">BlockHeight: {{ created?.anchor?.blockHeight ?? created?.blockHeight ?? '-' }}</div>
              </template>
            </a-result>
          </template>
        </a-card>

        <div class="footer">
          <a-button :disabled="step === 0" @click="prev">上一步</a-button>
          <a-button v-if="step < 2" type="primary" @click="next">下一步</a-button>
          <a-button v-else-if="step === 2" type="primary" :loading="submitting" @click="submitRegister">提交</a-button>
        </div>
      </a-col>

      <a-col :xs="24" :lg="12">
        <a-card class="inner-card" title="上链身份信息预览" size="small">
          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="设备DID"><span class="mono">{{ previewObj.did }}</span></a-descriptions-item>
            <a-descriptions-item label="所属域">{{ previewObj.domain }}</a-descriptions-item>
            <a-descriptions-item label="设备名称">{{ previewObj.displayName }}</a-descriptions-item>
            <a-descriptions-item label="设备类型">{{ previewObj.deviceType }}</a-descriptions-item>
          </a-descriptions>

          <a-divider />

          <a-card class="soft-card" size="small" title="元数据">
            <template v-if="metadataItems.length">
              <a-row :gutter="12">
                <a-col v-for="it in metadataItems" :key="it.k" :xs="24" :lg="12">
                  <a-statistic :title="it.k" :value="it.v" />
                </a-col>
              </a-row>
            </template>
            <template v-else>
              <a-empty description="未解析到可展示的元数据" />
            </template>
          </a-card>

          <a-descriptions size="small" :column="1" bordered>
            <a-descriptions-item label="网络">Fabric test-network</a-descriptions-item>
            <a-descriptions-item label="链码">anchorcc / AnchorRecord</a-descriptions-item>
            <a-descriptions-item label="Gas 预估">N/A（Fabric 无 Gas）</a-descriptions-item>
          </a-descriptions>
        </a-card>
      </a-col>
    </a-row>
  </a-card>
</template>

<style scoped>
.footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 14px;
}

.mono {
  font-family: var(--mono);
  color: #344054;
}
</style>
