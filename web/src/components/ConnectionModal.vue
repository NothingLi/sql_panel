<script setup lang="ts">
import { X } from 'lucide-vue-next'

defineProps<{
  show: boolean
  isEditing: boolean
  connection: any
}>()

defineEmits<{
  close: []
  save: []
}>()
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="bg-slate-800 border border-slate-700 rounded-xl shadow-2xl w-full max-w-md overflow-hidden animate-in fade-in zoom-in duration-200">
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50">
        <h3 class="font-bold text-lg">{{ isEditing ? 'Edit Connection' : 'Add Connection' }}</h3>
        <button @click="$emit('close')" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>
      
      <form @submit.prevent="$emit('save')" class="p-4 space-y-4">
        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Display Name</label>
          <input v-model="connection.name" required type="text" placeholder="e.g. Production MySQL"
                 class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
        </div>
        
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Type</label>
            <select v-model="connection.type" 
                    class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition">
              <option value="mysql">MySQL</option>
              <option value="postgres">PostgreSQL</option>
              <option value="sqlite">SQLite</option>
            </select>
          </div>
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Host / Path</label>
            <input v-model="connection.host" required type="text"
                   class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Port</label>
            <input v-model.number="connection.port" required type="number"
                   class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
          </div>
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Database</label>
            <input v-model="connection.database_name" required type="text"
                   class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
          </div>
        </div>

        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Username</label>
            <input v-model="connection.user" type="text"
                   class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
          </div>
          <div>
            <label class="block text-xs font-semibold text-slate-400 uppercase mb-1">Password</label>
            <input v-model="connection.password" type="password"
                   class="w-full bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
          </div>
        </div>

        <div class="pt-4 flex gap-3">
          <button type="button" @click="$emit('close')"
                  class="flex-1 px-4 py-2 border border-slate-700 hover:bg-slate-700 rounded font-medium transition">
            Cancel
          </button>
          <button type="submit"
                  class="flex-1 px-4 py-2 bg-blue-600 hover:bg-blue-700 rounded font-medium transition">
            {{ isEditing ? 'Update Connection' : 'Save Connection' }}
          </button>
        </div>
      </form>
    </div>
  </div>
</template>
