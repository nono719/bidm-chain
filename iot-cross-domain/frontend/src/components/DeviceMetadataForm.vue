<script setup>
// Form-based device metadata editor. Fields shown switch with deviceType;
// values are serialized back to a JSON string compatible with the existing
// `metadataJson` column. A 「JSON 原始模式」 toggle is preserved for any
// keys outside the template (so we never silently drop data on type change).
import { computed, reactive, ref, watch } from 'vue'

const props = defineProps({
  modelValue: { type: String, default: '' },
  deviceType: { type: String, default: '' }
})
const emit = defineEmits(['update:modelValue'])

// ===== Field schemas per device type =====
const COMMON_HEAD = [
  { key: 'model',    label: '型号',     type: 'text',   placeholder: '如 TH-01 / IPC-300' },
  { key: 'firmware', label: '固件版本', type: 'text',   placeholder: '如 1.0.0', default: '1.0.0' }
]

const SCHEMAS = {
  '传感器': [
    ...COMMON_HEAD,
    { key: 'samplingRate', label: '采样频率 (Hz)', type: 'number', min: 0.1, default: 1 },
    { key: 'unit',         label: '测量单位',     type: 'text',   placeholder: '如 °C / %RH / Pa' },
    { key: 'rangeMin',     label: '量程下限',     type: 'number' },
    { key: 'rangeMax',     label: '量程上限',     type: 'number' }
  ],
  '执行器': [
    ...COMMON_HEAD,
    { key: 'maxPower', label: '最大功率 (W)', type: 'number', min: 0, default: 100 },
    { key: 'mode',     label: '默认工作模式', type: 'select', options: ['normal','eco','turbo','custom'], default: 'normal' }
  ],
  '网关': [
    ...COMMON_HEAD,
    { key: 'maxClients', label: '最大连接设备', type: 'number', min: 1, default: 32 },
    { key: 'protocol',   label: '通信协议',   type: 'select', options: ['MQTT','CoAP','Modbus','HTTP','WebSocket','Zigbee','LoRaWAN'], default: 'MQTT' },
    { key: 'uplinkMbps', label: '上行带宽 (Mbps)', type: 'number', min: 0, default: 100 }
  ],
  '摄像头': [
    ...COMMON_HEAD,
    { key: 'resolution',  label: '分辨率', type: 'select', options: ['720p','1080p','2K','4K'], default: '1080p' },
    { key: 'fps',         label: '帧率',  type: 'number', min: 1, max: 120, default: 25 },
    { key: 'nightVision', label: '夜视',  type: 'bool',   default: true },
    { key: 'audioInput',  label: '内置麦克风', type: 'bool', default: false }
  ],
  '门禁设备': [
    ...COMMON_HEAD,
    { key: 'authMethod', label: '认证方式', type: 'select', options: ['刷卡','指纹','人脸识别','密码','刷卡+密码','人脸+密码'], default: '刷卡' },
    { key: 'maxUsers',   label: '最大用户数', type: 'number', min: 1, default: 1000 },
    { key: 'logRetentionDays', label: '记录保留天数', type: 'number', min: 1, default: 90 }
  ],
  '环境监测设备': [
    ...COMMON_HEAD,
    { key: 'sensors',      label: '监测项', type: 'multi-select',
      options: ['PM2.5','PM10','CO₂','TVOC','温度','湿度','噪声','光照','气压'],
      default: ['PM2.5','CO₂','温度','湿度'] },
    { key: 'samplingRate', label: '采样间隔 (秒)', type: 'number', min: 1, default: 30 }
  ],
  '工业控制器': [
    ...COMMON_HEAD,
    { key: 'protocol',   label: '通信协议', type: 'select', options: ['Modbus-RTU','Modbus-TCP','Profibus','Ethernet/IP','OPC-UA','S7'], default: 'Modbus-TCP' },
    { key: 'regCount',   label: '寄存器数量', type: 'number', min: 1, default: 256 },
    { key: 'scanCycleMs', label: '扫描周期 (ms)', type: 'number', min: 1, default: 10 }
  ],
  '智能电表': [
    ...COMMON_HEAD,
    { key: 'maxCurrent', label: '额定电流 (A)', type: 'number', min: 1, default: 60 },
    { key: 'phase',      label: '相位',       type: 'select', options: ['单相','三相'], default: '单相' },
    { key: 'tariffMode', label: '计费模式',   type: 'select', options: ['单一电价','分时电价','阶梯电价','峰平谷三时段'], default: '单一电价' }
  ],
  '智能家电': [
    ...COMMON_HEAD,
    { key: 'category',   label: '家电类型', type: 'select', options: ['智能灯','空调','冰箱','洗衣机','电视','音箱','扫地机器人','其他'], default: '智能灯' },
    { key: 'ratedPower', label: '额定功率 (W)', type: 'number', min: 0, default: 50 }
  ],
  '车载终端': [
    ...COMMON_HEAD,
    { key: 'vehicleType', label: '车辆类型', type: 'select', options: ['轿车','SUV','卡车','客车','电动车','摩托车','其他'], default: '轿车' },
    { key: 'protocol',    label: '通信协议', type: 'select', options: ['OBD-II','CAN','Bluetooth','4G/LTE','5G'], default: 'OBD-II' },
    { key: 'hasGps',      label: '内置 GPS', type: 'bool', default: true }
  ]
}

// Generic fallback schema for "其他" or unknown deviceType.
const FALLBACK = [
  ...COMMON_HEAD,
  { key: 'description', label: '描述', type: 'textarea', placeholder: '设备额外说明（可选）' }
]

const schema = computed(() => SCHEMAS[props.deviceType] || FALLBACK)

// ===== Internal form state =====
const form = reactive({})       // { key: value, ... }
const extraJson = ref('')       // any keys outside the schema → preserved here
const advanced = ref(false)     // raw JSON mode toggle
const rawJson = ref('')

function tryParse(json) {
  if (!json || typeof json !== 'string') return {}
  try {
    const v = JSON.parse(json)
    return v && typeof v === 'object' && !Array.isArray(v) ? v : {}
  } catch {
    return {}
  }
}

// Initialize form / extraJson from incoming JSON.
function rebuildFromValue() {
  const obj = tryParse(props.modelValue)
  const known = new Set(schema.value.map((f) => f.key))
  const extras = {}
  // Reset all schema keys to either obj value or default
  for (const f of schema.value) {
    if (obj[f.key] !== undefined) {
      form[f.key] = obj[f.key]
    } else if (f.default !== undefined) {
      form[f.key] = JSON.parse(JSON.stringify(f.default))
    } else {
      form[f.key] = f.type === 'bool' ? false : (f.type === 'multi-select' ? [] : '')
    }
  }
  // Remove keys no longer in schema
  for (const k of Object.keys(form)) {
    if (!known.has(k)) delete form[k]
  }
  for (const [k, v] of Object.entries(obj)) {
    if (!known.has(k)) extras[k] = v
  }
  extraJson.value = JSON.stringify(extras)
  rawJson.value = props.modelValue || ''
}

watch(() => props.deviceType, rebuildFromValue, { immediate: true })

// First-time init if value provided after mount.
watch(() => props.modelValue, (n) => {
  if (advanced.value) {
    rawJson.value = n || ''
    return
  }
  // Only rebuild if the value changed externally (not from this component's emit).
  const current = serializeOut()
  if (n !== current) rebuildFromValue()
}, { immediate: false })

function serializeOut() {
  const merged = { ...tryParse(extraJson.value) }
  for (const f of schema.value) {
    const v = form[f.key]
    if (v === '' || v === null || v === undefined) continue
    if (Array.isArray(v) && v.length === 0) continue
    merged[f.key] = v
  }
  return JSON.stringify(merged)
}

function emitChange() {
  if (advanced.value) {
    emit('update:modelValue', rawJson.value)
  } else {
    emit('update:modelValue', serializeOut())
  }
}

watch(form, emitChange, { deep: true })
watch(rawJson, () => { if (advanced.value) emitChange() })

function toggleAdvanced() {
  if (!advanced.value) {
    // entering advanced → seed rawJson with current serialized form
    rawJson.value = serializeOut()
    advanced.value = true
  } else {
    // leaving advanced → parse rawJson back into form
    advanced.value = false
    const obj = tryParse(rawJson.value)
    // Use rawJson's keys + values as new source of truth
    emit('update:modelValue', JSON.stringify(obj))
    rebuildFromValue()
  }
}

function resetToTemplate() {
  for (const f of schema.value) {
    if (f.default !== undefined) {
      form[f.key] = JSON.parse(JSON.stringify(f.default))
    } else {
      form[f.key] = f.type === 'bool' ? false : (f.type === 'multi-select' ? [] : '')
    }
  }
  extraJson.value = '{}'
}
</script>

<template>
  <div class="meta-form">
    <div class="meta-form-head">
      <span class="meta-form-label">
        <template v-if="SCHEMAS[deviceType]">
          按「{{ deviceType }}」模板填写
        </template>
        <template v-else>
          通用元数据
        </template>
      </span>
      <a-space size="small">
        <a-button size="small" type="link" @click="resetToTemplate">重置为模板默认</a-button>
        <a-button size="small" type="link" @click="toggleAdvanced">
          {{ advanced ? '返回表单模式' : '切换至 JSON 原始模式' }}
        </a-button>
      </a-space>
    </div>

    <!-- Form mode -->
    <a-row v-if="!advanced" :gutter="12" class="meta-form-grid">
      <a-col v-for="field in schema" :key="field.key" :xs="24" :sm="12">
        <a-form-item :label="field.label">
          <template v-if="field.type === 'text'">
            <a-input v-model:value="form[field.key]" :placeholder="field.placeholder" allow-clear />
          </template>
          <template v-else-if="field.type === 'textarea'">
            <a-textarea v-model:value="form[field.key]" :rows="2" :placeholder="field.placeholder" />
          </template>
          <template v-else-if="field.type === 'number'">
            <a-input-number
              v-model:value="form[field.key]"
              :min="field.min"
              :max="field.max"
              style="width: 100%"
            />
          </template>
          <template v-else-if="field.type === 'select'">
            <a-select v-model:value="form[field.key]" allow-clear>
              <a-select-option v-for="o in field.options" :key="o" :value="o">{{ o }}</a-select-option>
            </a-select>
          </template>
          <template v-else-if="field.type === 'multi-select'">
            <a-select
              v-model:value="form[field.key]"
              mode="multiple"
              allow-clear
              style="width: 100%"
            >
              <a-select-option v-for="o in field.options" :key="o" :value="o">{{ o }}</a-select-option>
            </a-select>
          </template>
          <template v-else-if="field.type === 'bool'">
            <a-switch v-model:checked="form[field.key]" />
          </template>
        </a-form-item>
      </a-col>
    </a-row>

    <!-- Advanced mode -->
    <div v-else>
      <a-textarea v-model:value="rawJson" :rows="6" placeholder='{"key":"value"}' />
      <div class="meta-form-hint">JSON 原始模式：所有字段以 JSON 对象形式直接编辑（适合高级用户或需保留模板外字段的场景）</div>
    </div>

    <!-- Schema-out preview -->
    <div v-if="!advanced" class="meta-form-preview">
      <span class="meta-form-preview-label">生成的 JSON：</span>
      <span class="meta-form-preview-value">{{ modelValue || '{}' }}</span>
    </div>
  </div>
</template>

<style scoped>
.meta-form {
  padding: 8px 4px 0;
}

.meta-form-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 6px;
}

.meta-form-label {
  font-size: 12px;
  color: #475569;
  background: #eef2ff;
  border-radius: 6px;
  padding: 2px 10px;
}

.meta-form-grid :deep(.ant-form-item) {
  margin-bottom: 10px;
}

.meta-form-grid :deep(.ant-form-item-label) > label {
  font-size: 12px;
  color: #475569;
  height: 22px;
}

.meta-form-hint {
  margin-top: 6px;
  font-size: 11px;
  color: #94a3b8;
}

.meta-form-preview {
  margin-top: 8px;
  font-size: 11px;
  color: #64748b;
  background: #f8fafc;
  border-radius: 6px;
  padding: 6px 10px;
  word-break: break-all;
}

.meta-form-preview-label {
  font-weight: 600;
  margin-right: 4px;
  color: #475569;
}

.meta-form-preview-value {
  font-family: var(--mono);
  color: #334155;
}
</style>
