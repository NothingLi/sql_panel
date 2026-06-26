<script setup lang="ts">
import { ref, watch, computed } from 'vue'
import axios from 'axios'
import { Check, Share2, X, Trash2, Shield, Crown } from 'lucide-vue-next'

interface ShareInfo {
  userId: number
  username: string
  sharedBy: number
  sharedByUsername: string
  isCreator: boolean
}

interface User {
  id: number
  username: string
}

const props = defineProps<{
  show: boolean
  connectionId: string | null
  isAdmin: boolean
  currentUserId: number
}>()

const emit = defineEmits<{
  close: []
  changed: []
}>()

const shares = ref<ShareInfo[]>([])
const users = ref<User[]>([])
const selectedUsernames = ref<string[]>([])
const loading = ref(false)

watch(() => props.show, async (val) => {
  if (val && props.connectionId) {
    loading.value = true
    selectedUsernames.value = []
    await Promise.all([fetchShares(), fetchUsers()])
    loading.value = false
  }
})

const fetchShares = async () => {
  try {
    const res = await axios.get(`/sqlpanel/api/connections/${props.connectionId}/shares`)
    shares.value = res.data
  } catch {
    shares.value = []
  }
}

const fetchUsers = async () => {
  try {
    const res = await axios.get('/sqlpanel/api/users')
    users.value = res.data
  } catch {
    users.value = []
  }
}

const canRevoke = (share: ShareInfo) => {
  if (share.isCreator) return false
  if (props.isAdmin) return true
  return share.sharedBy === props.currentUserId
}

const revokeShare = async (share: ShareInfo) => {
  try {
    await axios.delete(`/sqlpanel/api/connections/${props.connectionId}/shares/${share.userId}`)
    shares.value = shares.value.filter(s => s.userId !== share.userId)
    emit('changed')
  } catch {
    // handled by interceptor
  }
}

const selectedSet = computed(() => new Set(selectedUsernames.value))

const toggleUser = (username: string) => {
  if (selectedSet.value.has(username)) {
    selectedUsernames.value = selectedUsernames.value.filter(item => item !== username)
  } else {
    selectedUsernames.value = [...selectedUsernames.value, username]
  }
}

const shareConnection = async () => {
  if (!props.connectionId || selectedUsernames.value.length === 0) return
  try {
    await axios.post(`/sqlpanel/api/connections/${props.connectionId}/share`, {
      usernames: selectedUsernames.value
    })
    selectedUsernames.value = []
    await fetchShares()
    emit('changed')
  } catch {
    // handled by interceptor
  }
}

const availableUsers = computed(() => {
  const sharedIds = new Set(shares.value.map(s => s.userId))
  return users.value.filter(u => !sharedIds.has(u.id))
})
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="bg-slate-800 border border-slate-700 rounded-xl shadow-2xl w-full max-w-md overflow-hidden animate-in fade-in zoom-in duration-200">
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50">
        <h3 class="font-bold text-lg flex items-center gap-2">
          <Share2 class="w-5 h-5 text-green-400" />
          Share Connection
        </h3>
        <button @click="$emit('close')" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div v-if="loading" class="p-6 text-center text-sm text-slate-400">
        Loading...
      </div>

      <div v-else class="p-6 space-y-5">
        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-2">Current Access</label>
          <div class="max-h-40 overflow-y-auto rounded border border-slate-700 bg-slate-900">
            <div v-if="shares.length === 0" class="px-3 py-4 text-center text-xs italic text-slate-500">
              No users have access.
            </div>
            <div
              v-for="share in shares"
              :key="share.userId"
              class="flex items-center justify-between px-3 py-2 border-b border-slate-800 last:border-b-0"
            >
              <div class="flex items-center gap-2 min-w-0">
                <Crown v-if="share.isCreator" class="w-3.5 h-3.5 text-yellow-400 shrink-0" />
                <Shield v-else class="w-3.5 h-3.5 text-slate-500 shrink-0" />
                <span class="text-sm truncate" :class="share.isCreator ? 'text-yellow-300' : 'text-slate-300'">
                  {{ share.username }}
                </span>
                <span v-if="!share.isCreator" class="text-[10px] text-slate-500 truncate">
                  by {{ share.sharedByUsername }}
                </span>
              </div>
              <button
                v-if="canRevoke(share)"
                @click="revokeShare(share)"
                class="p-1 text-slate-500 hover:text-red-400 transition shrink-0 ml-2"
                title="Revoke access"
              >
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
        </div>

        <div class="border-t border-slate-700 pt-4">
          <div class="flex items-center justify-between mb-2">
            <label class="block text-xs font-semibold text-slate-400 uppercase">Add Users</label>
            <span class="text-[10px] text-slate-500">{{ selectedUsernames.length }} selected</span>
          </div>

          <div class="max-h-48 overflow-y-auto rounded border border-slate-700 bg-slate-900 p-1">
            <button
              v-for="user in availableUsers"
              :key="user.id"
              type="button"
              @click="toggleUser(user.username)"
              class="w-full flex items-center gap-2 rounded px-2 py-2 text-left text-sm transition hover:bg-slate-800"
              :class="selectedSet.has(user.username) ? 'text-green-300 bg-green-500/10' : 'text-slate-300'"
            >
              <span
                class="flex h-4 w-4 items-center justify-center rounded border"
                :class="selectedSet.has(user.username) ? 'border-green-500 bg-green-600' : 'border-slate-600 bg-slate-950'"
              >
                <Check v-if="selectedSet.has(user.username)" class="h-3 w-3 text-white" />
              </span>
              <span class="truncate">{{ user.username }}</span>
            </button>

            <div v-if="availableUsers.length === 0" class="px-3 py-6 text-center text-xs italic text-slate-500">
              All users already have access.
            </div>
          </div>

          <div class="mt-3 flex gap-3">
            <button type="button" @click="$emit('close')"
                    class="flex-1 py-2 border border-slate-700 hover:bg-slate-700 rounded font-medium transition text-sm">
              Done
            </button>
            <button @click="shareConnection"
                    :disabled="selectedUsernames.length === 0"
                    class="flex-1 py-2 bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed rounded font-medium transition text-sm">
              Share
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>