<script setup lang="ts">
import { ref } from 'vue'
import axios from 'axios'
import { Settings, X, Download, Upload } from 'lucide-vue-next'

const props = defineProps<{
  show: boolean
  settings: { limit: number }
  isAdmin: boolean
}>()

const emit = defineEmits<{
  close: []
  imported: []
}>()

const importFileInput = ref<HTMLInputElement | null>(null)

const exportSystemData = async () => {
  try {
    const response = await axios.get('/sqlpanel/api/system/export')
    const blob = new Blob([JSON.stringify(response.data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `sqlpanel-backup-${new Date().toISOString().slice(0, 10)}.json`
    a.click()
    URL.revokeObjectURL(url)
  } catch {
    // error handled by axios interceptor
  }
}

const triggerImport = () => {
  importFileInput.value?.click()
}

const handleImportFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return

  try {
    const text = await file.text()
    const data = JSON.parse(text)
    await axios.post('/sqlpanel/api/system/import', data)
    emit('imported')
  } catch {
    // error handled by axios interceptor
  } finally {
    input.value = ''
  }
}
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="bg-slate-800 border border-slate-700 rounded-xl shadow-2xl w-full max-w-sm overflow-hidden animate-in fade-in zoom-in duration-200">
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50">
        <h3 class="font-bold text-lg flex items-center gap-2">
          <Settings class="w-5 h-5 text-slate-400" />
          Settings
        </h3>
        <button @click="$emit('close')" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>
      
      <div class="p-6 space-y-6">
        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-2">Global Query Limit</label>
          <div class="flex items-center gap-3">
            <input v-model.number="settings.limit" type="number" 
                   class="flex-1 bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
            <span class="text-xs text-slate-500 italic">rows</span>
          </div>
          <p class="mt-2 text-[10px] text-slate-500">
            Automatically appends LIMIT to SELECT queries if not specified.
          </p>
        </div>

        <div v-if="isAdmin" class="border-t border-slate-700 pt-4">
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-3">System Data</label>
          <div class="flex gap-2">
            <button @click="exportSystemData"
                    class="flex-1 flex items-center justify-center gap-2 py-2 bg-slate-700 hover:bg-green-700 rounded font-medium text-sm transition">
              <Download class="w-4 h-4" />
              Export Backup
            </button>
            <button @click="triggerImport"
                    class="flex-1 flex items-center justify-center gap-2 py-2 bg-slate-700 hover:bg-yellow-700 rounded font-medium text-sm transition">
              <Upload class="w-4 h-4" />
              Import Backup
            </button>
            <input ref="importFileInput" type="file" accept=".json" class="hidden" @change="handleImportFile" />
          </div>
          <p class="mt-2 text-[10px] text-slate-500">
            Export/import all system data (users, connections, etc.) for backup and migration.
          </p>
        </div>

        <div class="pt-2">
          <button @click="$emit('close')"
                  class="w-full py-2 bg-slate-700 hover:bg-slate-600 rounded font-medium transition">
            Done
          </button>
        </div>
      </div>
    </div>
  </div>
</template>