<script setup lang="ts">
import { AlertCircle, CheckCircle2, X } from 'lucide-vue-next'

defineProps<{
  show: boolean
  type: 'error' | 'success'
  message: string
}>()

defineEmits<{
  close: []
}>()
</script>

<template>
  <Transition
    enter-active-class="transition duration-200 ease-out"
    enter-from-class="opacity-0 translate-y-2"
    enter-to-class="opacity-100 translate-y-0"
    leave-active-class="transition duration-150 ease-in"
    leave-from-class="opacity-100 translate-y-0"
    leave-to-class="opacity-0 translate-y-2"
  >
    <div
      v-if="show"
      class="fixed left-1/2 -translate-x-1/2 top-5 z-[300] flex w-[min(360px,calc(100vw-2.5rem))] items-start gap-3 rounded-lg border px-4 py-3 shadow-2xl"
      :class="type === 'error'
        ? 'border-red-500/40 bg-red-950/95 text-red-100 shadow-red-950/30'
        : 'border-green-500/40 bg-green-950/95 text-green-100 shadow-green-950/30'"
    >
      <component :is="type === 'error' ? AlertCircle : CheckCircle2" class="mt-0.5 h-5 w-5 flex-shrink-0" />
      <div class="min-w-0 flex-1 text-sm leading-5">{{ message }}</div>
      <button
        @click="$emit('close')"
        class="rounded p-0.5 opacity-70 transition hover:bg-white/10 hover:opacity-100"
        title="Close"
      >
        <X class="h-4 w-4" />
      </button>
    </div>
  </Transition>
</template>
