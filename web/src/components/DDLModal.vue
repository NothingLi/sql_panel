<script setup lang="ts">
import { computed } from 'vue'
import { Code2, Copy, X } from 'lucide-vue-next'
import SqlCodeViewer from './SqlCodeViewer.vue'

const props = defineProps<{
  show: boolean
  tableName: string
  ddl: string
  monacoTheme: string
  monacoOptions: Record<string, any>
  handleBeforeMount: (monacoInstance: any) => void
}>()

const emit = defineEmits<{
  'update:ddl': [value: string]
  close: []
  copy: []
}>()

const ddlValue = computed({
  get: () => props.ddl,
  set: value => emit('update:ddl', value),
})
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-[110] flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="bg-slate-800 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-3xl overflow-hidden animate-in fade-in zoom-in duration-200">
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50">
        <h3 class="font-bold text-lg flex items-center gap-2">
          <Code2 class="w-5 h-5 text-blue-400" />
          Create Statement: {{ tableName }}
        </h3>
        <button @click="$emit('close')" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>
      
      <div class="p-4 bg-slate-950 h-[400px] relative">
        <SqlCodeViewer
          v-model="ddlValue"
          :monaco-theme="monacoTheme"
          :monaco-options="monacoOptions"
          :handle-before-mount="handleBeforeMount"
        />
      </div>

      <div class="p-4 border-t border-slate-700 flex justify-end gap-3 bg-slate-800/30">
        <button @click="$emit('copy')" 
                class="px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded-lg font-medium transition flex items-center gap-2">
          <Copy class="w-4 h-4" /> Copy Statement
        </button>
        <button @click="$emit('close')"
                class="px-4 py-2 bg-slate-700 hover:bg-slate-600 rounded-lg font-medium transition">
          Close
        </button>
      </div>
    </div>
  </div>
</template>
