export const DEVICE_TYPE_OPTIONS = [
  { value: '传感器', label: '传感器' },
  { value: '执行器', label: '执行器' },
  { value: '网关', label: '网关' },
  { value: '摄像头', label: '摄像头' },
  { value: '门禁设备', label: '门禁设备' },
  { value: '环境监测设备', label: '环境监测设备' },
  { value: '工业控制器', label: '工业控制器' },
  { value: '智能电表', label: '智能电表' },
  { value: '智能家电', label: '智能家电' },
  { value: '车载终端', label: '车载终端' },
  { value: '其他', label: '其他' }
]

export const DEVICE_TYPE_FILTER_OPTIONS = [{ value: 'ALL', label: '全部类型' }, ...DEVICE_TYPE_OPTIONS]
