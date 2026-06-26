<script setup lang="ts">
import { ref, computed } from 'vue'
import { BarChart3, Download, Filter, LayoutGrid, Play, Clock, Hash } from 'lucide-vue-next'
import { FileSpreadsheet } from 'lucide-vue-next'
import ChartViewer from './ChartViewer.vue'

const props = defineProps<{
  results: any
  loading: boolean
  resultViewTab: 'table' | 'chart'
  hiddenColumns: Set<string>
  sortedData: any[]
  visibleColumns: string[]
  sortColumn: string
  sortOrder: 'asc' | 'desc' | ''
}>()

defineEmits<{
  'update:resultViewTab': [value: 'table' | 'chart']
  exportCsv: []
  exportExcel: []
  toggleColumn: [column: string]
  toggleSort: [column: string]
}>()

const showColumnFilter = ref(false)

const hasResults = computed(() => props.results?.columns && props.sortedData.length > 0)
const isDML = computed(() => props.results?.rowsAffected !== undefined && props.results?.rowsAffected !== null && !hasResults.value)
const hasError = computed(() => !!props.results?.error)
</script>

<template>
  <div class="flex-1 min-h-0 flex flex-col overflow-hidden relative">
    <div class="p-2 border-b border-slate-700 bg-slate-800 flex items-center justify-between shadow-sm gap-2 flex-wrap min-h-[40px]">
      <div class="flex items-center gap-2 flex-shrink-0">
        <span class="text-sm font-medium px-1">Results</span>
        
        <div v-if="results?.columns" class="flex bg-slate-900 p-0.5 rounded border border-slate-700 text-[11px] flex-shrink-0">
          <button
            @click="$emit('update:resultViewTab', 'table')"
            class="px-2.5 py-1 rounded transition flex items-center gap-1"
            :class="resultViewTab === 'table' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'"
          >
            <LayoutGrid class="w-3 h-3 flex-shrink-0" />
            Table
          </button>
          <button
            @click="$emit('update:resultViewTab', 'chart')"
            class="px-2.5 py-1 rounded transition flex items-center gap-1"
            :class="resultViewTab === 'chart' ? 'bg-slate-700 text-white' : 'text-slate-500 hover:text-slate-300'"
          >
            <BarChart3 class="w-3 h-3 flex-shrink-0" />
            Chart
          </button>
        </div>

        <div v-if="results?.columns && resultViewTab === 'table'" class="relative flex-shrink-0">
          <button
            @click="showColumnFilter = !showColumnFilter"
            class="flex items-center gap-1 text-[11px] px-2.5 py-1 rounded border border-slate-700 hover:bg-slate-700 transition"
            :class="hiddenColumns.size > 0 ? 'text-blue-400 border-blue-500/30 bg-blue-500/5' : 'text-slate-400'"
          >
            <Filter class="w-3 h-3 flex-shrink-0" />
            Columns {{ hiddenColumns.size > 0 ? `(${results.columns.length - hiddenColumns.size}/${results.columns.length})` : '' }}
          </button>
          
          <div v-if="showColumnFilter" class="absolute top-full left-0 mt-2 z-[60] bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-2 w-48 animate-in fade-in slide-in-from-top-1 duration-200">
            <div class="px-3 py-1 text-[10px] font-bold text-slate-500 uppercase tracking-wider mb-1">Show/Hide Columns</div>
            <div class="max-h-60 overflow-y-auto px-1">
              <div
                v-for="col in results.columns"
                :key="col"
                @click="$emit('toggleColumn', col)"
                class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-slate-700 cursor-pointer transition group"
              >
                <div
                  class="w-3.5 h-3.5 border border-slate-600 rounded flex items-center justify-center transition"
                  :class="!hiddenColumns.has(col) ? 'bg-blue-600 border-blue-500' : 'bg-slate-900'"
                >
                  <svg v-if="!hiddenColumns.has(col)" class="w-2.5 h-2.5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
                  </svg>
                </div>
                <span class="text-xs truncate flex-1" :class="hiddenColumns.has(col) ? 'text-slate-500 line-through' : 'text-slate-300'">{{ col }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div v-if="results?.columns" class="flex items-center flex-shrink-0">
        <button
          @click="$emit('exportCsv')"
          class="flex items-center gap-1 text-xs bg-slate-700 hover:bg-slate-600 px-2.5 py-1 rounded-l transition text-slate-200 border-r border-slate-600"
          title="Export CSV"
        >
          <Download class="w-3.5 h-3.5 flex-shrink-0" />
          CSV
        </button>
        <button
          @click="$emit('exportExcel')"
          class="flex items-center gap-1 text-xs bg-slate-700 hover:bg-slate-600 px-2.5 py-1 rounded-r transition text-slate-200"
          title="Export Excel"
        >
          <FileSpreadsheet class="w-3.5 h-3.5 flex-shrink-0" />
          Excel
        </button>
      </div>
    </div>

    <div class="flex-1 min-h-0 flex flex-col p-4 bg-slate-950/30 overflow-hidden">
      <div v-if="loading" class="flex items-center justify-center h-full">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500"></div>
      </div>
      
      <div v-else-if="results?.error" class="bg-red-900/20 border border-red-500/50 p-4 rounded text-red-400 font-mono text-sm">
        <div class="flex items-center gap-2 mb-2">
          <Clock class="w-3.5 h-3.5 text-red-400/60" />
          <span class="text-xs text-red-400/60">{{ results.duration }}</span>
        </div>
        {{ results.error }}
      </div>

      <div v-else-if="isDML" class="flex flex-col items-center justify-center h-full text-slate-300 gap-3">
        <div class="flex items-center gap-2 text-lg">
          <Hash class="w-5 h-5 text-green-400" />
          <span class="font-mono text-green-400 font-bold">{{ results.rowsAffected }}</span>
          <span class="text-slate-400">row{{ results.rowsAffected !== 1 ? 's' : '' }} affected</span>
        </div>
        <div class="flex items-center gap-1.5 text-xs text-slate-500">
          <Clock class="w-3 h-3" />
          {{ results.duration }}
        </div>
      </div>

      <div v-else-if="hasResults" class="flex-1 min-h-0 flex flex-col">
        <div class="flex items-center gap-3 mb-2 text-xs text-slate-500 px-1 flex-shrink-0">
          <span class="flex items-center gap-1">
            <Hash class="w-3 h-3" />
            {{ sortedData.length }} row{{ sortedData.length !== 1 ? 's' : '' }} returned
          </span>
          <span class="flex items-center gap-1">
            <Clock class="w-3 h-3" />
            {{ results.duration }}
          </span>
        </div>
        <div class="flex-1 min-h-0 overflow-auto">
          <table v-if="resultViewTab === 'table'" class="w-full text-left border-separate border-spacing-0">
            <thead>
              <tr class="sticky top-0 z-10 bg-slate-900">
                <th 
                  v-for="col in visibleColumns" 
                  :key="col" 
                  class="p-2 text-xs font-semibold uppercase tracking-wider border-b border-slate-700 cursor-pointer hover:bg-slate-800/50 transition select-none"
                  :class="{
                    'text-blue-400': sortColumn === col,
                    'text-slate-400': sortColumn !== col
                  }"
                  @click="$emit('toggleSort', col)"
                >
                  <div class="flex items-center gap-1">
                    <span>{{ col }}</span>
                    <span v-if="sortColumn === col" class="flex-shrink-0">
                      <svg v-if="sortOrder === 'asc'" class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 15l7-7 7 7" />
                      </svg>
                      <svg v-else class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                      </svg>
                    </span>
                  </div>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, i) in sortedData" :key="i" class="border-b border-slate-800 hover:bg-slate-800/50 transition">
                <td v-for="(cell, j) in row" :key="j" class="p-2 text-sm font-mono text-slate-300">
                  {{ cell }}
                </td>
              </tr>
            </tbody>
          </table>

          <ChartViewer v-else :columns="visibleColumns" :data="sortedData" />
        </div>
      </div>

      <div v-else class="flex flex-col items-center justify-center h-full text-slate-500">
        <Play class="w-12 h-12 mb-2 opacity-20" />
        <p>Run a query to see results</p>
      </div>
    </div>
  </div>
</template>
