<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Users, X, Trash2, Edit2, UserPlus, Shield, User } from 'lucide-vue-next'
import axios from 'axios'

defineProps<{
  show: boolean
}>()

defineEmits<{
  close: []
}>()

interface UserItem {
  id: number
  username: string
  role: string
}

const users = ref<UserItem[]>([])
const loading = ref(false)
const error = ref('')
const success = ref('')

const editingId = ref<number | null>(null)
const editForm = ref({ username: '', password: '', role: 'user' })

const newForm = ref({ username: '', password: '' })
const creating = ref(false)

const fetchUsers = async () => {
  loading.value = true
  error.value = ''
  try {
    const response = await axios.get('/sqlpanel/api/users/list')
    users.value = response.data
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to load users'
  } finally {
    loading.value = false
  }
}

const startEdit = (user: UserItem) => {
  editingId.value = user.id
  editForm.value = { username: user.username, password: '', role: user.role }
}

const cancelEdit = () => {
  editingId.value = null
  editForm.value = { username: '', password: '', role: 'user' }
}

const saveEdit = async () => {
  if (editingId.value === null) return
  error.value = ''
  success.value = ''
  try {
    await axios.put(`/sqlpanel/api/users/${editingId.value}`, {
      username: editForm.value.username,
      password: editForm.value.password || undefined,
      role: editForm.value.role,
    })
    success.value = 'User updated successfully'
    cancelEdit()
    await fetchUsers()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to update user'
  }
}

const deleteUser = async (user: UserItem) => {
  if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return
  error.value = ''
  success.value = ''
  try {
    await axios.delete(`/sqlpanel/api/users/${user.id}`)
    success.value = `User "${user.username}" deleted`
    await fetchUsers()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to delete user'
  }
}

const createUser = async () => {
  if (!newForm.value.username || !newForm.value.password) {
    error.value = 'Username and password are required'
    return
  }
  creating.value = true
  error.value = ''
  success.value = ''
  try {
    await axios.post('/sqlpanel/api/users', newForm.value)
    success.value = `User "${newForm.value.username}" created`
    newForm.value = { username: '', password: '' }
    await fetchUsers()
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed to create user'
  } finally {
    creating.value = false
  }
}

onMounted(() => {
  fetchUsers()
})
</script>

<template>
  <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
    <div class="bg-slate-800 border border-slate-700 rounded-xl shadow-2xl w-full max-w-2xl overflow-hidden animate-in fade-in zoom-in duration-200">
      <div class="flex items-center justify-between p-4 border-b border-slate-700 bg-slate-800/50">
        <h3 class="font-bold text-lg flex items-center gap-2">
          <Users class="w-5 h-5 text-blue-400" />
          User Management
        </h3>
        <button @click="$emit('close')" class="text-slate-400 hover:text-white transition">
          <X class="w-5 h-5" />
        </button>
      </div>

      <div class="p-6 space-y-4 max-h-[70vh] overflow-y-auto">
        <div v-if="error" class="p-3 rounded bg-red-900/20 border border-red-500/50 text-red-400 text-xs">
          {{ error }}
        </div>
        <div v-if="success" class="p-3 rounded bg-green-900/20 border border-green-500/50 text-green-400 text-xs">
          {{ success }}
        </div>

        <div class="border border-slate-700 rounded-lg p-4 bg-slate-800/50">
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-3">Create New User</label>
          <div class="flex gap-2">
            <input v-model="newForm.username" type="text" placeholder="Username"
                   class="flex-1 min-w-0 bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
            <input v-model="newForm.password" type="password" placeholder="Password"
                   class="flex-1 min-w-0 bg-slate-900 border border-slate-700 rounded p-2 text-sm focus:border-blue-500 focus:outline-none transition" />
            <button @click="createUser" :disabled="creating"
                    class="flex-shrink-0 px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:opacity-50 rounded-lg font-medium transition flex items-center gap-1.5 text-sm">
              <UserPlus class="w-4 h-4" />
              {{ creating ? '...' : 'Create' }}
            </button>
          </div>
        </div>

        <div>
          <label class="block text-xs font-semibold text-slate-400 uppercase mb-2">All Users</label>
          <div v-if="loading" class="text-xs text-slate-500 py-4 text-center">Loading...</div>
          <div v-else-if="users.length === 0" class="text-xs text-slate-500 py-4 text-center italic">No users found.</div>
          <div v-else class="border border-slate-700 rounded-lg overflow-hidden">
            <table class="w-full text-sm">
              <thead>
                <tr class="bg-slate-800/50 text-slate-400 text-xs uppercase">
                  <th class="text-left px-4 py-2 font-semibold">ID</th>
                  <th class="text-left px-4 py-2 font-semibold">Username</th>
                  <th class="text-left px-4 py-2 font-semibold">Role</th>
                  <th class="text-right px-4 py-2 font-semibold">Actions</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-700">
                <tr v-for="user in users" :key="user.id" class="hover:bg-slate-800/30 transition">
                  <td class="px-4 py-2.5 text-slate-500 text-xs">{{ user.id }}</td>
                  <td class="px-4 py-2.5">
                    <template v-if="editingId === user.id">
                      <input v-model="editForm.username"
                             class="w-full bg-slate-900 border border-slate-600 rounded p-1 text-sm focus:border-blue-500 focus:outline-none" />
                    </template>
                    <template v-else>
                      <span class="flex items-center gap-1.5">
                        <Shield v-if="user.role === 'admin'" class="w-3.5 h-3.5 text-yellow-400" />
                        <User v-else class="w-3.5 h-3.5 text-slate-400" />
                        {{ user.username }}
                      </span>
                    </template>
                  </td>
                  <td class="px-4 py-2.5">
                    <template v-if="editingId === user.id">
                      <select v-model="editForm.role"
                              class="bg-slate-900 border border-slate-600 rounded p-1 text-sm focus:border-blue-500 focus:outline-none">
                        <option value="user">user</option>
                        <option value="admin">admin</option>
                      </select>
                    </template>
                    <template v-else>
                      <span :class="user.role === 'admin' ? 'text-yellow-400' : 'text-slate-400'"
                            class="text-xs font-medium uppercase">{{ user.role }}</span>
                    </template>
                  </td>
                  <td class="px-4 py-2.5 text-right">
                    <template v-if="editingId === user.id">
                      <div class="flex items-center justify-end gap-1">
                        <input v-model="editForm.password" type="password" placeholder="new password"
                               class="w-24 bg-slate-900 border border-slate-600 rounded p-1 text-xs focus:border-blue-500 focus:outline-none" />
                        <button @click="saveEdit"
                                class="flex-shrink-0 px-2.5 py-1 bg-green-600 hover:bg-green-700 rounded text-xs font-medium transition">
                          Save
                        </button>
                        <button @click="cancelEdit"
                                class="flex-shrink-0 px-2.5 py-1 bg-slate-600 hover:bg-slate-500 rounded text-xs font-medium transition">
                          Cancel
                        </button>
                      </div>
                    </template>
                    <template v-else>
                      <div class="flex items-center justify-end gap-1">
                        <button @click="startEdit(user)"
                                class="p-1.5 hover:bg-slate-700 rounded transition text-slate-400 hover:text-blue-400"
                                title="Edit">
                          <Edit2 class="w-3.5 h-3.5" />
                        </button>
                        <button @click="deleteUser(user)"
                                class="p-1.5 hover:bg-slate-700 rounded transition text-slate-400 hover:text-red-400"
                                title="Delete">
                          <Trash2 class="w-3.5 h-3.5" />
                        </button>
                      </div>
                    </template>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>