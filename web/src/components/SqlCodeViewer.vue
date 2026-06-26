<script setup lang="ts">
import { computed } from 'vue'
import { VueMonacoEditor } from '@guolao/vue-monaco-editor'

const props = defineProps<{
  modelValue: string
  monacoTheme: string
  monacoOptions: Record<string, any>
  handleBeforeMount: (monacoInstance: any) => void
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const value = computed({
  get: () => props.modelValue,
  set: nextValue => emit('update:modelValue', nextValue),
})
</script>

<template>
  <VueMonacoEditor
    v-model:value="value"
    language="sql"
    :theme="monacoTheme"
    :options="{ ...monacoOptions, readOnly: true }"
    @before-mount="handleBeforeMount"
    class="absolute inset-0"
  />
</template>
