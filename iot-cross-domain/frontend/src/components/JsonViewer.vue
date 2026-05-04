<script setup>
import { computed } from 'vue'

const props = defineProps({
  value: { type: [Object, Array, String, Number, Boolean, null], default: null },
  height: { type: Number, default: 260 }
})

const text = computed(() => {
  if (typeof props.value === 'string') {
    try {
      const j = JSON.parse(props.value)
      return JSON.stringify(j, null, 2)
    } catch {
      return props.value
    }
  }
  return JSON.stringify(props.value, null, 2)
})
</script>

<template>
  <pre class="json" :style="{ maxHeight: `${height}px` }">{{ text }}</pre>
</template>

<style scoped>
.json {
  margin: 0;
  padding: 12px;
  border-radius: 10px;
  border: 1px solid rgba(24, 32, 120, 0.18);
  background: #0b1220;
  color: rgba(248, 250, 252, 0.96);
  overflow: auto;
  font-size: 12px;
  line-height: 1.45;
}
</style>
