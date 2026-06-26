<script setup lang="ts">
import { Database, Lock, User } from 'lucide-vue-next'

defineProps<{
  show: boolean
  mode: 'login'
  form: { username: string; password: string }
  error: string
  success: string
}>()

defineEmits<{
  submit: []
}>()
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-[100] flex items-center justify-center p-4 bg-slate-950">
    <div class="bg-slate-800 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-sm overflow-hidden animate-in fade-in zoom-in duration-300">
      <div class="p-8">
        <div class="flex flex-col items-center mb-8">
          <div class="w-16 h-16 bg-blue-600 rounded-2xl flex items-center justify-center mb-4 shadow-lg shadow-blue-500/20 rotate-12">
            <Database class="w-8 h-8 text-white -rotate-12" />
          </div>
          <h2 class="text-2xl font-bold">SQL panel</h2>
          <p class="text-slate-400 text-sm mt-1">Welcome back!</p>
        </div>

        <form @submit.prevent="$emit('submit')" class="space-y-4">
          <div v-if="error" class="p-3 rounded bg-red-900/20 border border-red-500/50 text-red-400 text-xs text-center animate-shake">
            {{ error }}
          </div>
          <div v-if="success" class="p-3 rounded bg-green-900/20 border border-green-500/50 text-green-400 text-xs text-center">
            {{ success }}
          </div>

          <div class="space-y-1">
            <label class="text-[10px] font-bold text-slate-500 uppercase px-1">Username</label>
            <div class="relative">
              <User class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" />
              <input v-model="form.username" required type="text" placeholder="Enter username"
                     class="w-full bg-slate-900 border border-slate-700 rounded-xl py-2.5 pl-10 pr-4 text-sm focus:border-blue-500 focus:outline-none transition" />
            </div>
          </div>

          <div class="space-y-1">
            <label class="text-[10px] font-bold text-slate-500 uppercase px-1">Password</label>
            <div class="relative">
              <Lock class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-slate-500" />
              <input v-model="form.password" required type="password" placeholder="Enter password"
                     class="w-full bg-slate-900 border border-slate-700 rounded-xl py-2.5 pl-10 pr-4 text-sm focus:border-blue-500 focus:outline-none transition" />
            </div>
          </div>

          <button type="submit"
                  class="w-full py-3 bg-blue-600 hover:bg-blue-700 rounded-xl font-bold transition mt-4 shadow-lg shadow-blue-600/20">
            Sign In
          </button>
        </form>

        <!-- <div class="mt-8 text-center">
          <button @click="$emit('toggleMode')"
                  class="text-xs text-slate-400 hover:text-blue-400 transition flex items-center justify-center gap-2 mx-auto">
            <component :is="UserPlus" class="w-3.5 h-3.5" />
            {{ mode === 'login' ? "Don't have an account? Register" : "Already have an account? Login" }}
          </button>
        </div> -->
      </div>
    </div>
  </div>
</template>
