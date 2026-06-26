<script setup lang="ts">
import { computed } from 'vue'
import { AlignLeft, Play, Plus, X, Database, GitBranch, Check, RotateCcw } from 'lucide-vue-next'
import { VueMonacoEditor } from '@guolao/vue-monaco-editor'

export interface EditorTab {
  id: string
  title: string
  sql: string
  connectionId: string
}

const props = defineProps<{
  modelValue: string
  connectionName: string
  hasConnection: boolean
  loading: boolean
  monacoTheme: string
  monacoOptions: Record<string, any>
  handleBeforeMount: (monacoInstance: any) => void
  handleMount: (editor: any, monacoInstance: any) => void
  tabs: EditorTab[]
  activeTabId: string
  transactionMode: string
  transactionActive: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
  format: []
  run: []
  'switch-tab': [tabId: string]
  'close-tab': [tabId: string]
  'add-tab': []
  'update:transactionMode': [value: string]
  'begin-transaction': []
  'commit-transaction': []
  'rollback-transaction': []
}>()

const sqlValue = computed({
  get: () => props.modelValue,
  set: value => emit('update:modelValue', value),
})
</script>

<template>
  <div class="flex-1 min-h-0 flex flex-col border-b border-slate-700">
    <div class="flex items-center bg-slate-900 border-b border-slate-700 overflow-x-auto">
      <div class="flex items-center flex-1 min-w-0">
        <div
          v-for="tab in tabs"
          :key="tab.id"
          @click="emit('switch-tab', tab.id)"
          class="group flex items-center gap-1.5 px-3 py-1.5 text-xs cursor-pointer border-r border-slate-700 transition min-w-0 max-w-[180px] border-b-2"
          :class="tab.id === activeTabId
            ? 'bg-slate-800 text-slate-200 border-b-blue-500'
            : 'text-slate-500 hover:text-slate-300 hover:bg-slate-800/50 border-b-transparent'"
        >
          <Database class="w-3 h-3 flex-shrink-0" :class="tab.connectionId ? 'text-blue-400' : 'text-slate-600'" />
          <span class="truncate">{{ tab.title || 'Untitled' }}</span>
          <button
            v-if="tabs.length > 1"
            @click.stop="emit('close-tab', tab.id)"
            class="flex-shrink-0 p-0.5 rounded hover:bg-slate-600 opacity-0 group-hover:opacity-100 transition"
          >
            <X class="w-3 h-3" />
          </button>
        </div>
      </div>
      <button
        @click="emit('add-tab')"
        class="flex-shrink-0 p-1.5 mx-1 rounded hover:bg-slate-700 text-slate-500 hover:text-slate-300 transition"
        title="New Tab"
      >
        <Plus class="w-3.5 h-3.5" />
      </button>
    </div>

    <div class="p-2 border-b border-slate-700 flex items-center justify-between bg-slate-800">
      <div class="flex items-center gap-2">
        <span class="text-sm font-medium">Query Editor</span>
        <span v-if="hasConnection" class="text-xs bg-slate-700 px-2 py-0.5 rounded text-slate-300">
          {{ connectionName }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <div class="flex items-center gap-1 bg-slate-700 rounded">
          <button
            @click="emit('update:transactionMode', 'auto')"
            class="px-2 py-1 text-xs rounded transition"
            :class="transactionMode === 'auto' ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-slate-200'"
          >
            Auto
          </button>
          <button
            @click="emit('update:transactionMode', 'manual')"
            class="px-2 py-1 text-xs rounded transition"
            :class="transactionMode === 'manual' ? 'bg-orange-600 text-white' : 'text-slate-400 hover:text-slate-200'"
          >
            Manual Tx
          </button>
        </div>
        <template v-if="transactionMode === 'manual'">
          <button
            v-if="!transactionActive"
            @click="emit('begin-transaction')"
            :disabled="loading || !hasConnection"
            class="flex items-center gap-1 text-xs bg-orange-600 hover:bg-orange-700 disabled:opacity-50 disabled:cursor-not-allowed px-2 py-1 rounded transition"
            title="BEGIN TRANSACTION"
          >
            <GitBranch class="w-3.5 h-3.5" />
            BEGIN
          </button>
          <template v-else>
            <button
              @click="emit('commit-transaction')"
              :disabled="loading"
              class="flex items-center gap-1 text-xs bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed px-2 py-1 rounded transition"
              title="COMMIT"
            >
              <Check class="w-3.5 h-3.5" />
              COMMIT
            </button>
            <button
              @click="emit('rollback-transaction')"
              :disabled="loading"
              class="flex items-center gap-1 text-xs bg-red-600 hover:bg-red-700 disabled:opacity-50 disabled:cursor-not-allowed px-2 py-1 rounded transition"
              title="ROLLBACK"
            >
              <RotateCcw class="w-3.5 h-3.5" />
              ROLLBACK
            </button>
          </template>
        </template>
        <button
          @click="$emit('format')"
          :disabled="!modelValue"
          class="flex items-center gap-2 text-slate-400 hover:text-white hover:bg-slate-700 disabled:opacity-30 px-3 py-1 rounded text-sm font-medium transition"
          title="Format SQL (Keyword Uppercase)"
        >
          <AlignLeft class="w-4 h-4" />
          Format
        </button>
        <button
          @click="$emit('run')"
          :disabled="loading || !hasConnection"
          class="flex items-center gap-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed px-4 py-1 rounded text-sm font-medium transition"
        >
          <Play class="w-4 h-4" />
          Run
        </button>
      </div>
    </div>

    <div class="flex-1 min-h-0 relative overflow-hidden">
      <VueMonacoEditor
        v-model:value="sqlValue"
        language="sql"
        :theme="monacoTheme"
        :options="monacoOptions"
        @before-mount="handleBeforeMount"
        @mount="handleMount"
        class="absolute inset-0"
      />
    </div>
  </div>
</template>