<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { Upload, FileSpreadsheet, FileText, Play, X, ChevronLeft, CheckCircle2, Loader2, AlertCircle } from 'lucide-vue-next'
import axios from 'axios'

const props = defineProps<{
  show: boolean
  connectionId: string
  schema: any[]
  tableName?: string
}>()

const emit = defineEmits<{
  close: []
  imported: []
}>()

// 步骤控制: 'select' | 'preview' | 'importing' | 'done' | 'error'
const step = ref<'select' | 'preview' | 'importing' | 'done' | 'error'>('select')
const errorMsg = ref('')

// 文件选择
const selectedFile = ref<File | null>(null)
const dragOver = ref(false)

// 表选择
const tableMode = ref<'existing' | 'new'>('existing')
const selectedTable = ref('')
const newTableName = ref('')

// 解析结果
const parsedColumns = ref<string[]>([])
const parsedPreview = ref<any[][]>([])
const fileId = ref('')
const parsedTotalRows = ref(0)
const parsedFileName = ref('')
const fileParsing = ref(false)

// 导入结果
const importResult = reactive({
  rowsImported: 0,
  duration: '',
  tableName: ''
})

const tableOptions = computed(() => {
  return props.schema.map((t: any) => t.name)
})

const targetTableName = computed(() => {
  return tableMode.value === 'existing' ? selectedTable.value : newTableName.value
})

// 当从右键菜单打开时，自动预选目标表
watch(() => props.show, (show) => {
  if (show && props.tableName) {
    tableMode.value = 'existing'
    selectedTable.value = props.tableName
  }
})

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files && input.files[0]) {
    selectedFile.value = input.files[0]
    // 自动用文件名作为表名建议
    if (tableMode.value === 'new' && !newTableName.value) {
      const name = selectedFile.value.name.replace(/\.[^.]+$/, '')
      newTableName.value = name.replace(/[^a-zA-Z0-9_]/g, '_')
    }
  }
}

function handleDragOver(e: DragEvent) {
  e.preventDefault()
  dragOver.value = true
}

function handleDragLeave() {
  dragOver.value = false
}

function handleDrop(e: DragEvent) {
  e.preventDefault()
  dragOver.value = false
  if (e.dataTransfer?.files && e.dataTransfer.files[0]) {
    selectedFile.value = e.dataTransfer.files[0]
    if (tableMode.value === 'new' && !newTableName.value) {
      const name = selectedFile.value.name.replace(/\.[^.]+$/, '')
      newTableName.value = name.replace(/[^a-zA-Z0-9_]/g, '_')
    }
  }
}

async function parseFile() {
  if (!selectedFile.value) return

  fileParsing.value = true
  errorMsg.value = ''

  try {
    const formData = new FormData()
    formData.append('file', selectedFile.value)

    const response = await axios.post('/sqlpanel/api/import/parse', formData, {
      headers: { 'Content-Type': 'multipart/form-data' }
    })

    parsedColumns.value = response.data.columns
    parsedPreview.value = response.data.preview
    fileId.value = response.data.fileId
    parsedTotalRows.value = response.data.totalRows
    parsedFileName.value = response.data.fileName
    step.value = 'preview'
  } catch (error: any) {
    errorMsg.value = error.response?.data?.error || 'Failed to parse file'
  } finally {
    fileParsing.value = false
  }
}

async function doImport() {
  step.value = 'importing'

  try {
    const response = await axios.post('/sqlpanel/api/import/data', {
      connectionId: props.connectionId,
      tableName: targetTableName.value,
      fileId: fileId.value,
      createTable: tableMode.value === 'new'
    })

    importResult.rowsImported = response.data.rowsImported
    importResult.duration = response.data.duration
    importResult.tableName = response.data.tableName
    step.value = 'done'
    emit('imported')
  } catch (error: any) {
    errorMsg.value = error.response?.data?.error || 'Import failed'
    step.value = 'error'
  }
}

function resetAndClose() {
  step.value = 'select'
  selectedFile.value = null
  selectedTable.value = ''
  newTableName.value = ''
  tableMode.value = 'existing'
  parsedColumns.value = []
  parsedPreview.value = []
  fileId.value = ''
  parsedTotalRows.value = 0
  errorMsg.value = ''
  emit('close')
}
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-[120] flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" @click.self="resetAndClose">
    <div class="bg-slate-800 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-2xl max-h-[85vh] overflow-hidden flex flex-col animate-in fade-in zoom-in duration-200">
      <!-- 头部 -->
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50 shrink-0">
        <div class="flex items-center gap-3">
          <button v-if="step === 'preview'" @click="step = 'select'" class="p-1 hover:bg-slate-700 rounded transition text-slate-400 hover:text-slate-200">
            <ChevronLeft class="w-5 h-5" />
          </button>
          <h3 class="font-bold text-lg flex items-center gap-2">
            <Upload class="w-5 h-5 text-blue-400" />
            Import Data
          </h3>
        </div>
        <button @click="resetAndClose" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>

      <!-- 步骤1：选择文件和表 -->
      <div v-if="step === 'select'" class="flex-1 overflow-y-auto p-6 space-y-5">
        <!-- 文件上传区 -->
        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-2">1. Select File (CSV or XLSX)</label>
          <div
            @dragover="handleDragOver"
            @dragleave="handleDragLeave"
            @drop="handleDrop"
            class="border-2 border-dashed rounded-xl p-8 text-center transition cursor-pointer"
            :class="dragOver ? 'border-blue-400 bg-blue-500/10' : selectedFile ? 'border-green-500/50 bg-green-500/5' : 'border-slate-600 hover:border-slate-500 bg-slate-900/30'"
          >
            <input type="file" accept=".csv,.xlsx" @change="handleFileSelect" class="hidden" ref="fileInput" />
            
            <div v-if="!selectedFile" @click="($refs.fileInput as HTMLInputElement).click()" class="space-y-3">
              <div class="flex justify-center gap-3">
                <FileSpreadsheet class="w-10 h-10 text-green-400/60" />
                <FileText class="w-10 h-10 text-blue-400/60" />
              </div>
              <p class="text-sm text-slate-400">Drop CSV or XLSX file here, or click to browse</p>
              <p class="text-xs text-slate-600">Maximum 100 rows preview shown</p>
            </div>

            <div v-else class="space-y-2">
              <div class="flex items-center justify-center gap-2">
                <component :is="selectedFile.name.endsWith('.xlsx') ? FileSpreadsheet : FileText" class="w-6 h-6" 
                           :class="selectedFile.name.endsWith('.xlsx') ? 'text-green-400' : 'text-blue-400'" />
                <span class="text-sm font-medium">{{ selectedFile.name }}</span>
                <span class="text-xs text-slate-500">({{ (selectedFile.size / 1024).toFixed(1) }} KB)</span>
              </div>
              <button @click="selectedFile = null" class="text-xs text-red-400 hover:text-red-300 underline">Remove</button>
            </div>
          </div>
        </div>

        <!-- 目标表选择 -->
        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-2">2. Target Table</label>
          
          <div class="flex bg-slate-900 p-1 rounded-lg text-xs mb-3">
            <button @click="tableMode = 'existing'" 
                    class="flex-1 py-1.5 rounded-md transition"
                    :class="tableMode === 'existing' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'">
              Existing Table
            </button>
            <button @click="tableMode = 'new'" 
                    class="flex-1 py-1.5 rounded-md transition"
                    :class="tableMode === 'new' ? 'bg-slate-700 text-white' : 'text-slate-400 hover:text-slate-200'">
              New Table
            </button>
          </div>

          <div v-if="tableMode === 'existing'">
            <select v-model="selectedTable"
                    class="w-full bg-slate-900 border border-slate-700 rounded-lg p-2.5 text-sm focus:border-blue-500 focus:outline-none transition">
              <option value="" disabled>Select a table...</option>
              <option v-for="t in tableOptions" :key="t" :value="t">{{ t }}</option>
            </select>
          </div>

          <div v-else>
            <input v-model="newTableName" type="text" placeholder="Enter new table name..."
                   class="w-full bg-slate-900 border border-slate-700 rounded-lg p-2.5 text-sm focus:border-blue-500 focus:outline-none transition" />
            <p class="text-[10px] text-slate-500 mt-1">A new table will be created with all columns as TEXT type.</p>
          </div>
        </div>
      </div>

      <!-- 步骤2：预览 -->
      <div v-if="step === 'preview'" class="flex-1 min-h-0 flex flex-col overflow-hidden">
        <div class="px-6 py-3 bg-slate-900/50 border-b border-slate-700 shrink-0">
          <div class="flex items-center justify-between text-sm">
            <span class="text-slate-400">Preview: <strong class="text-slate-200">{{ parsedFileName }}</strong></span>
            <span class="text-xs text-slate-500">{{ parsedTotalRows }} rows total (showing {{ parsedPreview.length }})</span>
          </div>
        </div>
        
        <div class="flex-1 overflow-auto p-4">
          <table class="w-full text-left border-separate border-spacing-0 text-xs">
            <thead>
              <tr class="sticky top-0 z-10 bg-slate-900">
                <th class="p-2 text-slate-400 font-semibold uppercase border-b border-slate-700 w-10 text-center">#</th>
                <th v-for="col in parsedColumns" :key="col" class="p-2 text-slate-400 font-semibold uppercase border-b border-slate-700 whitespace-nowrap">
                  {{ col }}
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(row, i) in parsedPreview" :key="i" class="border-b border-slate-800 hover:bg-slate-800/30 transition">
                <td class="p-2 text-slate-600 text-center">{{ i + 1 }}</td>
                <td v-for="(cell, j) in row" :key="j" class="p-2 font-mono text-slate-300 max-w-[200px] truncate">
                  {{ cell }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 导入中 -->
      <div v-if="step === 'importing'" class="flex-1 flex flex-col items-center justify-center p-8 gap-4">
        <Loader2 class="w-10 h-10 text-blue-400 animate-spin" />
        <p class="text-slate-300 font-medium">Importing data to <strong class="text-blue-400">{{ targetTableName }}</strong>...</p>
        <p class="text-xs text-slate-500">{{ parsedTotalRows }} rows</p>
      </div>

      <!-- 导入成功 -->
      <div v-if="step === 'done'" class="flex-1 flex flex-col items-center justify-center p-8 gap-3">
        <CheckCircle2 class="w-12 h-12 text-green-400" />
        <p class="text-lg font-bold text-green-400">{{ importResult.rowsImported }} rows imported</p>
        <p class="text-sm text-slate-400">to <strong class="text-slate-200">{{ importResult.tableName }}</strong></p>
        <p class="text-xs text-slate-500">Duration: {{ importResult.duration }}</p>
      </div>

      <!-- 导入失败 -->
      <div v-if="step === 'error'" class="flex-1 flex flex-col items-center justify-center p-8 gap-3">
        <AlertCircle class="w-12 h-12 text-red-400" />
        <p class="text-red-400 font-medium">Import failed</p>
        <p class="text-sm text-slate-400 text-center max-w-md">{{ errorMsg }}</p>
      </div>

      <!-- 错误提示条 -->
      <div v-if="errorMsg && (step === 'select' || step === 'preview')" class="px-6 py-2 bg-red-900/30 border-t border-red-500/30 shrink-0">
        <p class="text-xs text-red-400 flex items-center gap-1.5">
          <AlertCircle class="w-3.5 h-3.5" />
          {{ errorMsg }}
        </p>
      </div>

      <!-- 底部按钮 -->
      <div v-if="step !== 'importing'" class="p-4 border-t border-slate-700 bg-slate-800/50 flex gap-3 shrink-0">
        <button @click="resetAndClose" class="flex-1 px-4 py-2.5 border border-slate-700 hover:bg-slate-700 rounded-lg font-medium transition text-sm">
          {{ step === 'done' ? 'Close' : 'Cancel' }}
        </button>
        
        <button v-if="step === 'select'" @click="parseFile" 
                :disabled="!selectedFile || !targetTableName || fileParsing"
                class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-800 disabled:text-slate-400 disabled:cursor-not-allowed rounded-lg font-medium transition text-sm flex items-center justify-center gap-2">
          <Loader2 v-if="fileParsing" class="w-4 h-4 animate-spin" />
          <Play v-else class="w-4 h-4" />
          {{ fileParsing ? 'Parsing...' : 'Parse & Preview' }}
        </button>

        <button v-if="step === 'preview'" @click="doImport"
                class="flex-1 px-4 py-2.5 bg-green-600 hover:bg-green-700 rounded-lg font-medium transition text-sm flex items-center justify-center gap-2">
          <Upload class="w-4 h-4" />
          Import {{ parsedTotalRows }} Rows
        </button>

        <button v-if="step === 'done'" @click="resetAndClose"
                class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-700 rounded-lg font-medium transition text-sm">
          Done
        </button>

        <button v-if="step === 'error'" @click="step = 'select'; errorMsg = ''"
                class="flex-1 px-4 py-2.5 bg-blue-600 hover:bg-blue-700 rounded-lg font-medium transition text-sm">
          Try Again
        </button>
      </div>
    </div>
  </div>
</template>