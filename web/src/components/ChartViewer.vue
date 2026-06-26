<script setup lang="ts">
import { ref, onMounted, watch, onUnmounted, computed } from 'vue'
import * as echarts from 'echarts'
import { BarChart, LineChart as LineChartIcon, Settings2 } from 'lucide-vue-next'

const props = defineProps<{
  columns: string[]
  data: any[][]
}>()

const chartRef = ref<HTMLElement | null>(null)
let chartInstance: echarts.ECharts | null = null

const xAxisColumn = ref(props.columns[0] || '')
const chartType = ref<'bar' | 'line'>('bar')

watch(() => props.columns, (cols) => {
  if (cols.length > 0 && !cols.includes(xAxisColumn.value)) {
    xAxisColumn.value = cols[0]
  }
})

const countData = computed(() => {
  const colIndex = props.columns.indexOf(xAxisColumn.value)
  if (colIndex === -1) return []

  const counter = new Map<string, number>()
  for (const row of props.data) {
    const val = row[colIndex]
    const key = val === null || val === undefined ? '(null)' : String(val)
    counter.set(key, (counter.get(key) || 0) + 1)
  }

  const entries = Array.from(counter.entries())
  entries.sort((a, b) => b[1] - a[1])
  return entries
})

const initChart = () => {
  if (!chartRef.value) return

  if (!chartInstance) {
    chartInstance = echarts.init(chartRef.value, 'dark')
  }

  const entries = countData.value
  if (entries.length === 0) {
    chartInstance.clear()
    return
  }

  const xData = entries.map(e => e[0])
  const yData = entries.map(e => e[1])

  const option: echarts.EChartsOption = {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: chartType.value === 'bar' ? 'shadow' : 'cross' },
      formatter: (params: any) => {
        const p = Array.isArray(params) ? params[0] : params
        return `<div style="font-size:12px"><strong>${p.name}</strong><br/>Count: <strong>${p.value}</strong></div>`
      }
    },
    grid: {
      top: '8%',
      left: '3%',
      right: '4%',
      bottom: '12%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: xData,
      axisLabel: {
        color: '#94a3b8',
        rotate: xData.length > 10 ? 45 : 0,
        interval: 0,
        overflow: 'truncate',
        width: xData.length > 20 ? 80 : undefined,
      },
      axisTick: { alignWithLabel: true }
    },
    yAxis: {
      type: 'value',
      name: 'Count',
      nameTextStyle: { color: '#64748b', fontSize: 11 },
      axisLabel: { color: '#94a3b8' },
      splitLine: { lineStyle: { color: '#334155' } },
      minInterval: 1,
    },
    series: [
      {
        name: 'Count',
        data: yData,
        type: chartType.value,
        itemStyle: {
          color: chartType.value === 'bar' ? '#3b82f6' : '#10b981'
        },
        smooth: chartType.value === 'line',
        barMaxWidth: 50,
        label: {
          show: entries.length <= 20,
          position: 'top',
          color: '#94a3b8',
          fontSize: 10,
        }
      }
    ]
  }

  chartInstance.setOption(option, true)
}

watch([xAxisColumn, chartType, countData], () => {
  initChart()
})

const handleResize = () => {
  chartInstance?.resize()
}

onMounted(() => {
  initChart()
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  chartInstance?.dispose()
})
</script>

<template>
  <div class="h-full flex flex-col bg-slate-900/50 rounded-lg border border-slate-700 overflow-hidden">
    <div class="p-3 border-b border-slate-700 bg-slate-800/30 flex flex-wrap items-center gap-4">
      <div class="flex items-center gap-2">
        <Settings2 class="w-4 h-4 text-slate-400" />
        <span class="text-xs font-semibold text-slate-400 uppercase tracking-wider">Chart Config</span>
      </div>

      <div class="flex items-center gap-4 ml-auto">
        <div class="flex items-center gap-2">
          <label class="text-[10px] text-slate-500 uppercase">Column:</label>
          <select v-model="xAxisColumn"
                  class="bg-slate-800 border border-slate-600 rounded px-2 py-1 text-xs text-slate-300 focus:outline-none focus:border-blue-500 max-w-[160px] truncate">
            <option v-for="col in columns" :key="col" :value="col">{{ col }}</option>
          </select>
        </div>

        <span class="text-[10px] text-slate-500">
          {{ countData.length }} distinct values
        </span>

        <div class="flex bg-slate-800 p-0.5 rounded border border-slate-600">
          <button @click="chartType = 'bar'"
                  class="p-1 rounded transition"
                  :class="chartType === 'bar' ? 'bg-blue-600 text-white' : 'text-slate-400 hover:text-slate-200'"
                  title="Bar Chart">
            <BarChart class="w-4 h-4" />
          </button>
          <button @click="chartType = 'line'"
                  class="p-1 rounded transition"
                  :class="chartType === 'line' ? 'bg-green-600 text-white' : 'text-slate-400 hover:text-slate-200'"
                  title="Line Chart">
            <LineChartIcon class="w-4 h-4" />
          </button>
        </div>
      </div>
    </div>

    <div class="flex-1 min-h-[300px]" ref="chartRef"></div>
  </div>
</template>