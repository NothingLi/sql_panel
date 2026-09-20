<script setup lang="ts">
import { ref, onMounted, reactive, watch, computed, onUnmounted } from 'vue'
import axios from 'axios'
import * as XLSX from 'xlsx'

declare global {
  interface Window {
    __ENCRYPT__?: string
  }
}
// 从 lucide-vue-next 导入图标组件
import { Plus, Database, Trash2, Clock, ChevronRight, ChevronDown, ChevronUp, Table2, Columns, Settings, Edit2, LogOut, Share2, Copy, Code2, PlayCircle, Users, Search, History, Upload } from 'lucide-vue-next'
// 导入 SQL 格式化工具
import { format } from 'sql-formatter'
import AuthModal from './components/AuthModal.vue'
import ConnectionModal from './components/ConnectionModal.vue'
import DDLModal from './components/DDLModal.vue'
import ImportModal from './components/ImportModal.vue'
import ResultsPanel from './components/ResultsPanel.vue'
import SettingsModal from './components/SettingsModal.vue'
import ShareModal from './components/ShareModal.vue'
import UserManagementModal from './components/UserManagementModal.vue'
import SqlEditor from './components/SqlEditor.vue'
import ToastNotification from './components/ToastNotification.vue'
import { useSqlMonaco } from './composables/useSqlMonaco'
import { useToast } from './composables/useToast'
import { encrypt, decrypt } from './utils/crypto'

// --- 响应式状态定义 ---
const connections = ref<any[]>([])      // 存储所有的数据库连接列表

interface EditorTab {
  id: string
  title: string
  sql: string
  connectionId: string
}

const tabs = ref<EditorTab[]>([{
  id: '1',
  title: 'Untitled',
  sql: ' ',
  connectionId: '',
}])
const activeTabId = ref('1')

const currentConnection = computed({
  get: () => tabs.value.find(t => t.id === activeTabId.value)?.connectionId || '',
  set: (val: string) => {
    const tab = tabs.value.find(t => t.id === activeTabId.value)
    if (tab) {
      tab.connectionId = val
      tab.title = connections.value.find(c => c.id === val)?.name || 'Untitled'
    }
  }
})

const sql = computed({
  get: () => tabs.value.find(t => t.id === activeTabId.value)?.sql || '',
  set: (val: string) => {
    const tab = tabs.value.find(t => t.id === activeTabId.value)
    if (tab) tab.sql = val
  }
})

const results = ref<any>(null)          // 存储查询结果（包含 columns 和 data）

const queryData = computed(() => {
  if (!results.value) return []
  if (results.value.data) return results.value.data
  if (results.value.rows && results.value.columns) {
    return results.value.rows.map((row: any) => 
      results.value.columns.map((col: string) => row[col])
    )
  }
  return []
})

const queryColumns = computed(() => results.value?.columns || [])
const loading = ref(false)              // 查询是否正在加载中
let queryAbortController: AbortController | null = null  // 用于取消上一次查询请求
const showModal = ref(false)            // 是否显示"添加连接"的弹窗
const showSettings = ref(false)         // 是否显示"设置"弹窗
const showUserManagement = ref(false)  // 是否显示"用户管理"弹窗
const isEditing = ref(false)            // 弹窗当前是否为编辑模式
const editingId = ref<string | null>(null) // 正在编辑的连接 ID
const showShareModal = ref(false)       // 是否显示"分享"弹窗
const shareConnectionId = ref<string | null>(null)
const history = ref<any[]>([])          // SQL 查询历史记录
const exportLogs = ref<any[]>([])       // 导出历史记录
const showExportHistory = ref(false)    // 是否显示导出历史弹窗
const sidebarTab = ref<'connections' | 'history'>('connections') // 侧边栏当前选中的标签页
const transactionMode = ref('auto')     // 事务模式：auto | manual
const transactionActive = ref(false)    // 当前是否有活跃事务

// --- DDL 相关状态 ---
const showDDLModal = ref(false)         // 是否显示 DDL 弹窗
const showImportModal = ref(false)     // 是否显示导入弹窗
const importTableName = ref('')        // 从右键菜单传入的目标表名
const currentDDL = ref('')              // 当前显示的 DDL 语句
const ddlTableName = ref('')            // 当前查看 DDL 的表名

// --- 右键菜单相关状态 ---
const contextMenu = reactive({
  show: false,
  x: 0,
  y: 0,
  type: '' as 'table' | 'column',
  tableName: '',
  columnName: '',
  dbName: ''
})
//const resultViewTab = ref<'table' | 'chart'>('table') // 结果区域当前的显示模式
const resultViewTab = ref<'table' | 'chart'>('table') // 结果区域当前的显示模式
const schema = ref<any[]>([])           // 当前连接的表结构信息
const dbType = ref('')                  // 当前连接的数据库类型（用于函数提示过滤）
const dbVersion = ref('')               // 当前连接的数据库主版本号（用于函数提示过滤）

// --- 表格增强状态 ---
const hiddenColumns = ref<Set<string>>(new Set()) // 被隐藏的列
const sortConfig = reactive({
  column: '',
  order: 'asc' as 'asc' | 'desc' | ''
})

const globalSettings = reactive({
  limit: 1000,                           // 全局查询行数限制
  timeout: 300                           // 查询超时秒数，默认 5 分钟
})
const expandedTables = ref<Set<string>>(new Set()) // 记录展开的表名
const loadingSchema = ref(false)        // 是否正在加载 Schema
const schemaSearch = ref('')            // Schema 搜索关键词
const sidebarWidth = ref(300)           // 侧边栏宽度（px）
const isResizing = ref(false)           // 是否正在拖拽调整宽度

// --- 认证相关状态 ---
const token = ref(localStorage.getItem('token') || '')
const username = ref(localStorage.getItem('username') || '')
const currentUserId = ref(parseInt(localStorage.getItem('userId') || '0'))
const role = ref(localStorage.getItem('role') || '')
const isAuth = computed(() => !!token.value)
const isAdmin = computed(() => role.value === 'admin')
const showAuthModal = ref(!isAuth.value)
const authMode = ref<'login'>('login')
const authForm = reactive({
  username: '',
  password: ''
})
const authError = ref('')
const authSuccess = ref('') // 新增成功提示状态
const { toast, showToast, hideToast, getRequestErrorMessage } = useToast()

// 配置 Axios 拦截器


axios.interceptors.request.use(async config => {
  if (token.value) {
    config.headers.Authorization = `Bearer ${token.value}`
  }
  const encryptionEnabled = (window.__ENCRYPT__ || import.meta.env.VITE_ENCRYPT) === 'true'
  if (encryptionEnabled && config.data && typeof config.data === 'object' && !(config.data instanceof FormData)) {
    const json = JSON.stringify(config.data)
    config.data = await encrypt(json)
    config.headers['Content-Type'] = 'text/plain'
  }
  return config
})

axios.interceptors.response.use(
  async response => {
    const encryptionEnabled = (window.__ENCRYPT__ || import.meta.env.VITE_ENCRYPT) === 'true'
    if (encryptionEnabled && typeof response.data === 'string' && response.data) {
      try {
        const json = await decrypt(response.data)
        response.data = JSON.parse(json)

      } catch {

      }
    }
    return response
  },
  async error => {
    // 解密加密的错误响应，让调用方能读到真实错误信息
    const encryptionEnabled = (window.__ENCRYPT__ || import.meta.env.VITE_ENCRYPT) === 'true'
    if (encryptionEnabled && error.response && typeof error.response.data === 'string' && error.response.data) {
      try {
        const json = await decrypt(error.response.data)
        error.response.data = JSON.parse(json)
      } catch {
        // 解密失败保持原样
      }
    }
    // 401 表示登录状态失效。这里保留原有逻辑：清空登录信息并弹回登录弹窗。
    if (error.response?.status === 401) {
      logout()
    }
    // 请求被主动取消（AbortController）时不弹 toast，也不覆盖结果
    if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') {
      return Promise.reject(error)
    }

    // 所有 axios 请求失败都会走到这里，因此只需要在拦截器里统一弹一次错误提示。
    // 这样 fetchConnections/saveConnection/runQuery 等函数里不需要重复写 showToast。
    showToast(getRequestErrorMessage(error), 'error')

    // 继续 reject，保证调用方自己的 catch/finally 还能正常执行。
    // 例如 runQuery 的 catch 仍然可以把错误写到 Results 区域。
    return Promise.reject(error)
  }
)

const login = async () => {
  try {
    authError.value = ''
    authSuccess.value = ''
    const response = await axios.post('/sqlpanel/api/login', authForm)
    token.value = response.data.token
    username.value = response.data.username
    currentUserId.value = response.data.userId
    role.value = response.data.role || ''
    localStorage.setItem('token', token.value)
    localStorage.setItem('username', username.value)
    localStorage.setItem('userId', currentUserId.value.toString())
    localStorage.setItem('role', role.value)
    showAuthModal.value = false
    fetchConnections()
    loadHistory() // 登录后加载该用户的历史记录
  } catch (error: any) {
    console.log(error)
    authError.value = error.response?.data?.error || 'Login failed'
  }
}

const logout = () => {
  token.value = ''
  username.value = ''
  currentUserId.value = 0
  role.value = ''
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  localStorage.removeItem('userId')
  localStorage.removeItem('role')
  showAuthModal.value = true
  connections.value = []
  results.value = null
  schema.value = []
}

// --- 表单状态 ---
// 使用 reactive 定义新连接的表单对象
const newConnection = reactive({
  name: '',
  type: 'mysql',
  host: 'localhost',
  port: 3306,
  user: '',
  password: '',
  database: ''
})

// --- 业务逻辑方法 ---

// 从后端获取所有连接
const fetchConnections = async () => {
  try {
    const response = await axios.get('/sqlpanel/api/connections')
    connections.value = response.data
    // 如果列表不为空且当前没选中，默认选中第一个
    // if (connections.value.length > 0 && !currentConnection.value) {
    //   currentConnection.value = connections.value[0].id
    // }
  } catch (error) {
    console.error('Failed to fetch connections')
  }
}

// 保存新连接或更新现有连接
const saveConnection = async () => {
  try {
    if (isEditing.value && editingId.value) {
      await axios.put(`/sqlpanel/api/connections/${editingId.value}`, newConnection)
    } else {
      await axios.post('/sqlpanel/api/connections', newConnection)
    }
    await fetchConnections() // 刷新列表
    closeModal()
  } catch (error) {
    console.error('Failed to save connection')
  }
}

// 打开新增弹窗
const openAddModal = () => {
  isEditing.value = false
  editingId.value = null
  Object.assign(newConnection, {
    name: '',
    type: 'mysql',
    host: 'localhost',
    port: 3306,
    user: '',
    password: '',
    database: ''
  })
  showModal.value = true
}

// 打开编辑弹窗
const openEditModal = (conn: any) => {
  isEditing.value = true
  editingId.value = conn.id
  Object.assign(newConnection, { ...conn })
  showModal.value = true
}

// 关闭弹窗并重置
const closeModal = () => {
  showModal.value = false
  showShareModal.value = false
  isEditing.value = false
  editingId.value = null
  shareConnectionId.value = null
}

const openShareModal = (connectionId: string) => {
  shareConnectionId.value = connectionId
  showShareModal.value = true
}

// 删除连接
const deleteConnection = async (id: string) => {
  try {
    await axios.delete(`/sqlpanel/api/connections/${id}`)
    await fetchConnections() // 刷新列表
    // 如果删的是当前选中的，清空选中状态
    if (currentConnection.value === id) {
      currentConnection.value = connections.value[0]?.id || ''
    }
  } catch (error) {
    console.error('Failed to delete connection')
  }
}

/**
 * 计算并排序后的数据
 */
const sortedData = computed(() => {
  if (!results.value || !queryData.value.length) return []
  if (!sortConfig.column) return queryData.value

  const colIndex = queryColumns.value.indexOf(sortConfig.column)
  if (colIndex === -1) return queryData.value

  return [...queryData.value].sort((a, b) => {
    const valA = a[colIndex]
    const valB = b[colIndex]

    if (valA === valB) return 0
    
    // 尝试数字比较
    const numA = Number(valA)
    const numB = Number(valB)
    if (!isNaN(numA) && !isNaN(numB)) {
      return sortConfig.order === 'asc' ? numA - numB : numB - numA
    }

    // 字符串比较
    const strA = String(valA || '')
    const strB = String(valB || '')
    return sortConfig.order === 'asc' 
      ? strA.localeCompare(strB) 
      : strB.localeCompare(strA)
  })
})

/**
 * 切换排序
 */
const toggleSort = (column: string) => {
  if (sortConfig.column === column) {
    if (sortConfig.order === 'asc') sortConfig.order = 'desc'
    else if (sortConfig.order === 'desc') {
      sortConfig.column = ''
      sortConfig.order = ''
    }
  } else {
    sortConfig.column = column
    sortConfig.order = 'asc'
  }
}

/**
 * 切换列显示/隐藏
 */
const toggleColumnVisibility = (column: string) => {
  if (hiddenColumns.value.has(column)) {
    hiddenColumns.value.delete(column)
  } else {
    hiddenColumns.value.add(column)
  }
}

/**
 * 获取显示的列名
 */
const visibleColumns = computed(() => {
  if (!queryColumns.value.length) return []
  return queryColumns.value.filter((col: string) => !hiddenColumns.value.has(col))
})

// 执行 SQL 查询
const runQuery = async () => {
  if (!currentConnection.value) return
  
  // 重置排序和隐藏列
  sortConfig.column = ''
  sortConfig.order = ''
  hiddenColumns.value.clear()
  
  // 获取要执行的 SQL：如果有选中的内容则只执行选中部分，否则执行全部
  let sqlToRun = sql.value
  if (editorRef.value) {
    const selection = editorRef.value.getSelection()
    const selectedText = editorRef.value.getModel()?.getValueInRange(selection)
    if (selectedText && selectedText.trim().length > 0) {
      sqlToRun = selectedText
    }
  }

  // 应用全局 Limit 限制 (简单追加逻辑，实际可更复杂)
  const upperSQL = sqlToRun.trim().toUpperCase()
  if (upperSQL.startsWith('SELECT') && !upperSQL.includes('LIMIT')) {
    sqlToRun = `${sqlToRun.trim().replace(/;$/, '')} LIMIT ${globalSettings.limit};`
  }

  loading.value = true // 开始加载动画

  // 取消上一次未完成的请求
  if (queryAbortController) {
    queryAbortController.abort()
  }
  queryAbortController = new AbortController()

  try {
    // 发送请求到后端执行 SQL
    const response = await axios.post('/sqlpanel/api/query', {
      connectionId: currentConnection.value,
      sql: sqlToRun,
      timeout: globalSettings.timeout,
    }, {
      signal: queryAbortController.signal,
    })
    results.value = response.data // 存储返回的结果

    if (response.data.transactionActive !== undefined) {
      transactionActive.value = response.data.transactionActive
    }
    
    // 如果执行成功（没有 Error），加入历史记录
    if (!response.data.error) {
      addToHistory(sqlToRun)
    }
  } catch (error: any) {
    // 请求被主动取消时，保留上一次查询结果
    if (error?.name === 'CanceledError' || error?.code === 'ERR_CANCELED') {
      // 不覆盖结果
    } else {
      // 将错误详情展示在结果区，方便排查
      results.value = { error: getRequestErrorMessage(error), duration: 'Query failed' }
    }
  } finally {
    loading.value = false // 结束加载动画
  }
}

const beginTransaction = async () => {
  if (!currentConnection.value) return
  loading.value = true
  try {
    const response = await axios.post('/sqlpanel/api/query', {
      connectionId: currentConnection.value,
      sql: 'BEGIN',
      timeout: globalSettings.timeout,
    })
    if (response.data.error) {
      results.value = response.data
    } else {
      transactionActive.value = true
      showToast('Transaction started', 'success')
    }
  } catch (error) {
    // 错误由 toast 统一提示，不覆盖结果区
  } finally {
    loading.value = false
  }
}

const commitTransaction = async () => {
  if (!currentConnection.value) return
  loading.value = true
  try {
    const response = await axios.post('/sqlpanel/api/query', {
      connectionId: currentConnection.value,
      sql: 'COMMIT',
      timeout: globalSettings.timeout,
    })
    if (response.data.error) {
      results.value = response.data
    } else {
      transactionActive.value = false
      showToast('Transaction committed', 'success')
    }
  } catch (error) {
    // 错误由 toast 统一提示，不覆盖结果区
  } finally {
    loading.value = false
  }
}

const rollbackTransaction = async () => {
  if (!currentConnection.value) return
  loading.value = true
  try {
    const response = await axios.post('/sqlpanel/api/query', {
      connectionId: currentConnection.value,
      sql: 'ROLLBACK',
      timeout: globalSettings.timeout,
    })
    if (response.data.error) {
      results.value = response.data
    } else {
      transactionActive.value = false
      showToast('Transaction rolled back', 'success')
    }
  } catch (error) {
    // 错误由 toast 统一提示，不覆盖结果区
  } finally {
    loading.value = false
  }
}

// Monaco 编辑器相关逻辑已经拆到 composable。
// App.vue 只负责把当前 schema 和运行查询的方法传进去。
const {
  editorRef,
  MONACO_THEME,
  MONACO_OPTIONS,
  handleBeforeMount,
  handleMount,
} = useSqlMonaco(schema, runQuery, dbType, dbVersion)

// 格式化 SQL 代码
const formatSQL = () => {
  if (!sql.value) return
  try {
    sql.value = format(sql.value, {
      language: 'postgresql', // 使用通用型 SQL 格式化
      keywordCase: 'upper',   // 关键字转大写 (修复 linter 错误)
    })
  } catch (error) {
    console.error('Failed to format SQL:', error)
  }
}

// --- 历史记录与导出逻辑 ---

/**
 * 添加到历史记录
 * @param query SQL 语句
 */
const addToHistory = (query: string) => {
  const item = {
    id: Date.now().toString(),
    sql: query,
    time: new Date().toLocaleTimeString(),
    connectionName: connections.value.find(c => c.id === currentConnection.value)?.name || 'Unknown',
    connectionId: currentConnection.value,
  }
  // 优化：如果历史记录中已存在相同的 SQL，先删掉旧的，再把新的插到最前面
  // 并通过 slice(0, 50) 限制只保留最近 50 条记录
  history.value = [item, ...history.value.filter(h => h.sql !== query)].slice(0, 50)
  // 持久化到本地浏览器存储 (增加用户名作为前缀，实现多用户隔离)
  localStorage.setItem(`sql_history_${username.value}`, JSON.stringify(history.value))
}

/**
 * 删除单条历史记录，并同步更新本地存储
 * @param id 要删除的历史记录 ID
 */
const deleteHistoryItem = (id: string) => {
  history.value = history.value.filter(item => item.id !== id)
  localStorage.setItem(`sql_history_${username.value}`, JSON.stringify(history.value))
}

let tabCounter = 1
const addTab = (connectionId?: string, sqlContent?: string) => {
  tabCounter++
  const id = Date.now().toString()
  const connName = connectionId
    ? connections.value.find(c => c.id === connectionId)?.name || 'Untitled'
    : 'Untitled'
  tabs.value.push({
    id,
    title: connName,
    sql: sqlContent || 'SELECT * FROM ',
    connectionId: connectionId || currentConnection.value || '',
  })
  activeTabId.value = id
}

const closeTab = (tabId: string) => {
  if (tabs.value.length <= 1) return
  const idx = tabs.value.findIndex(t => t.id === tabId)
  tabs.value.splice(idx, 1)
  if (activeTabId.value === tabId) {
    activeTabId.value = tabs.value[Math.min(idx, tabs.value.length - 1)].id
  }
}

const switchTab = (tabId: string) => {
  activeTabId.value = tabId
}

const openHistoryTab = (item: any) => {
  let connId = item.connectionId || ''
  if (connId && !connections.value.find((c: any) => c.id === connId)) {
    connId = ''
  }
  if (!connId) {
    const conn = connections.value.find((c: any) => c.name === item.connectionName)
    connId = conn?.id || ''
  }
  addTab(connId, item.sql)
}

/**
 * 从本地存储加载历史记录
 */
const loadHistory = () => {
  const saved = localStorage.getItem(`sql_history_${username.value}`)
  if (saved) {
    history.value = JSON.parse(saved)
  } else {
    history.value = []
  }
}

const saveExportLog = async (fileType: string, fileName: string, rowCount: number) => {
  const connName = connections.value.find(c => c.id === currentConnection.value)?.name || 'Unknown'
  await axios.post('/sqlpanel/api/export-logs', {
    connectionName: connName,
    sqlStatement: sql.value,
    rowCount,
    fileType,
    fileName,
  })
}

const fetchExportLogs = async () => {
  try {
    const response = await axios.get('/sqlpanel/api/export-logs')
    exportLogs.value = response.data
  } catch {
    exportLogs.value = []
  }
  showExportHistory.value = true
}

/**
 * 导出当前查询结果为 CSV 文件
 */
const exportToCSV = async () => {
  if (!results.value || !queryColumns.value.length || !queryData.value.length) return

  const visibleCols = queryColumns.value.filter((col: string) => !hiddenColumns.value.has(col))
  const headers = visibleCols.join(',')

  const rows = sortedData.value.map((row: any[]) => {
    return queryColumns.value
      .map((col: string, index: number) => ({ col, val: row[index] }))
      .filter((item: any) => !hiddenColumns.value.has(item.col))
      .map((item: any) => {
        const str = String(item.val ?? '')
        if (str.includes(',') || str.includes('"') || str.includes('\n')) {
          return `"${str.replace(/"/g, '""')}"`
        }
        return str
      }).join(',')
  })
  
  const csvContent = [headers, ...rows].join('\n')

  const fileName = `query_results_${Date.now()}.csv`

  try {
    await saveExportLog('CSV', fileName, sortedData.value.length)
  } catch {
    showToast('Failed to save export log', 'error')
    return
  }

  const blob = new Blob(['\ufeff' + csvContent], { type: 'text/csv;charset=utf-8;' })
  const link = document.createElement('a')
  const url = URL.createObjectURL(blob)
  link.setAttribute('href', url)
  link.setAttribute('download', fileName)
  link.style.visibility = 'hidden'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
}

const exportToExcel = async () => {
  if (!results.value || !queryColumns.value.length || !queryData.value.length) return

  const visibleCols = queryColumns.value.filter((col: string) => !hiddenColumns.value.has(col))
  const exportData = sortedData.value.map((row: any[]) => {
    const obj: Record<string, any> = {}
    visibleCols.forEach((col: string) => {
      const idx = queryColumns.value.indexOf(col)
      obj[col] = row[idx] ?? ''
    })
    return obj
  })

  const ws = XLSX.utils.json_to_sheet(exportData)
  const wb = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(wb, ws, 'Results')
  const fileName = `query_results_${Date.now()}.xlsx`

  try {
    await saveExportLog('Excel', fileName, sortedData.value.length)
  } catch {
    showToast('Failed to save export log', 'error')
    return
  }

  XLSX.writeFile(wb, fileName)
}

const filteredSchema = computed(() => {
  const term = schemaSearch.value.toLowerCase().trim()
  if (!term) return schema.value

  return schema.value.filter((table: any) => {
    if (table.name.toLowerCase().includes(term)) return true
    return table.columns.some((col: any) => {
      const colName = typeof col === 'string' ? col : col.name
      return colName.toLowerCase().includes(term)
    })
  })
})

watch(schemaSearch, (term) => {
  if (!term.trim()) return
  const t = term.toLowerCase().trim()
  schema.value.forEach((table: any) => {
    const colMatch = table.columns.some((col: any) => {
      const colName = typeof col === 'string' ? col : col.name
      return colName.toLowerCase().includes(t)
    })
    if (colMatch && !table.name.toLowerCase().includes(t)) {
      expandedTables.value.add(table.name)
    }
  })
})

const highlightMatch = (text: string): string => {
  const term = schemaSearch.value.trim()
  if (!term) return text
  const escaped = term.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const regex = new RegExp(`(${escaped})`, 'gi')
  return text.replace(regex, '<mark class="schema-highlight">$1</mark>')
}

/** 规范化列类型显示：统一大写、去除多余空格。 */
const normalizeColumnType = (col: any): string => {
  if (typeof col === 'string') {
    const match = col.match(/^(.+?)\s*\(([^)]+)\)$/)
    return match ? match[2].trim().toUpperCase() : ''
  }
  return (col?.type || '').trim().toUpperCase()
}

const matchIndex = ref(-1)

const matchCount = computed(() => {
  const term = schemaSearch.value.trim()
  if (!term) return 0
  const t = term.toLowerCase()
  let count = 0
  for (const table of filteredSchema.value) {
    const name = table.name.toLowerCase()
    let pos = 0
    while ((pos = name.indexOf(t, pos)) !== -1) { count++; pos++ }
    for (const col of table.columns) {
      const colName = (typeof col === 'string' ? col : col.name).toLowerCase()
      pos = 0
      while ((pos = colName.indexOf(t, pos)) !== -1) { count++; pos++ }
    }
  }
  return count
})

const navigateMatch = (direction: 1 | -1) => {
  const container = document.querySelector('.schema-list-container')
  if (!container) return
  const marks = container.querySelectorAll<HTMLElement>('mark.schema-highlight')
  if (marks.length === 0) {
    matchIndex.value = -1
    return
  }

  marks.forEach(m => m.classList.remove('schema-highlight-active'))

  let next = matchIndex.value + direction
  if (next >= marks.length) next = 0
  if (next < 0) next = marks.length - 1
  matchIndex.value = next

  const active = marks[next]
  active.classList.add('schema-highlight-active')
  active.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
}

const onSearchKeydown = (e: KeyboardEvent) => {
  if (e.key === 'ArrowDown') {
    e.preventDefault()
    navigateMatch(1)
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    navigateMatch(-1)
  } else if (e.key === 'Escape') {
    schemaSearch.value = ''
    matchIndex.value = -1
  }
}

watch(schemaSearch, () => {
  matchIndex.value = -1
})

// 获取当前连接的 Schema 信息（同时拉取数据库版本用于函数智能提示）
const fetchSchema = async () => {
  if (!currentConnection.value) {
    schema.value = []
    dbType.value = ''
    dbVersion.value = ''
    return
  }
  loadingSchema.value = true
  // 同步当前连接类型，供函数提示立即按 dbType 过滤
  const conn = connections.value.find(c => c.id === currentConnection.value)
  dbType.value = conn?.type || ''
  try {
    const [schemaRes, versionRes] = await Promise.all([
      axios.get(`/sqlpanel/api/tables/${currentConnection.value}`),
      axios.get(`/sqlpanel/api/connections/${currentConnection.value}/version`),
    ])
    // 统一规范化：确保每个列都是 {name, type, comment} 对象格式
    schema.value = (schemaRes.data || []).map((table: any) => ({
      ...table,
      columns: (table.columns || []).map((col: any) => {
        if (typeof col === 'string') {
          const match = col.match(/^(.+?)\s*\(([^)]+)\)$/)
          return {
            name: match ? match[1].trim() : col,
            type: match ? match[2].trim().toUpperCase() : '',
            comment: '',
          }
        }
        return {
          name: col.name,
          type: (col.type || '').trim().toUpperCase(),
          comment: col.comment || '',
        }
      }),
    }))
    // 后端返回 { type, version }，以版本号为准
    dbType.value = versionRes.data?.type || dbType.value
    dbVersion.value = versionRes.data?.version || ''
    expandedTables.value.clear()
  } catch (error) {
    console.error('Failed to fetch schema')
    schema.value = []
    dbVersion.value = ''
  } finally {
    loadingSchema.value = false
  }
}

// 切换表的展开/收起状态
const toggleTable = (tableName: string) => {
  if (expandedTables.value.has(tableName)) {
    expandedTables.value.delete(tableName)
  } else {
    expandedTables.value.add(tableName)
  }
}

// 监听当前连接的变化，自动获取 Schema
watch(currentConnection, () => {
  fetchSchema()
})

// --- 右键菜单与复制逻辑 ---

/**
 * 显示右键菜单
 */
const showContextMenu = (e: MouseEvent, type: 'table' | 'column', tableName: string, colName?: string) => {
  e.preventDefault()
  const conn = connections.value.find(c => c.id === currentConnection.value)
  
  contextMenu.show = true
  contextMenu.x = e.clientX
  contextMenu.y = e.clientY
  contextMenu.type = type
  contextMenu.tableName = tableName
  contextMenu.dbName = conn?.database_name || 'default'
  
  if (type === 'column' && colName) {
    contextMenu.columnName = colName
  } else {
    contextMenu.columnName = ''
  }
}

/**
 * 复制文本到剪贴板
 */
const copyToClipboard = async (text: string) => {
  try {
    await navigator.clipboard.writeText(text)
    showToast('Copied to clipboard', 'success')
    contextMenu.show = false
  } catch (err) {
    showToast('Failed to copy', 'error')
  }
}

/**
  * 在编辑器光标处插入 SQL
  */
const insertSQL = (text: string) => {
  if (editorRef.value) {
    const position = editorRef.value.getPosition()
    // 使用 executeEdits 来支持撤销/重做
    editorRef.value.executeEdits('insert-sql', [
      {
        range: {
          startLineNumber: position.lineNumber,
          startColumn: position.column,
          endLineNumber: position.lineNumber,
          endColumn: position.column
        },
        text: text + '\n',
        forceMoveMarkers: true
      }
    ])
    // 自动聚焦编辑器
    editorRef.value.focus()
  } else {
    // 降级处理：如果没有编辑器实例，直接追加
    sql.value += (sql.value ? '\n' : '') + text
  }
  contextMenu.show = false
}

/**
 * 插入包含所有列名的 SELECT 语句
 */
const insertAllColumnsSQL = (tableName: string) => {
  const table = schema.value.find(t => t.name === tableName)
  if (!table || !table.columns) return

  const columnNames = table.columns.map((col: any) => col.name)
  const columnsStr = columnNames.join(', ')
  
  insertSQL(`SELECT ${columnsStr} FROM ${tableName} LIMIT ${globalSettings.limit};`)
}

/**
 * 右键菜单：生成 INSERT 语句，列出所有字段
 */
const insertInsertSQL = (tableName: string) => {
  const table = schema.value.find((t: any) => t.name === tableName)
  if (!table || !table.columns) return

  const columnNames = table.columns.map((col: any) => col.name)
  const columnsStr = columnNames.join(', ')
  const valuesPlaceholders = columnNames.map(() => "''").join(', ')

  insertSQL(`INSERT INTO ${tableName} (${columnsStr}) VALUES (${valuesPlaceholders});`)
}

/**
 * 点击页面其他地方关闭右键菜单
 */
const handleGlobalClick = () => {
  if (contextMenu.show) {
    contextMenu.show = false
  }
}

/**
 * 获取并显示表的 DDL
 */
const viewTableDDL = async (tableName: string) => {
  if (!currentConnection.value) return
  try {
    const response = await axios.get(`/sqlpanel/api/connections/${currentConnection.value}/schema/${tableName}/ddl`)
    currentDDL.value = response.data.ddl
    ddlTableName.value = tableName
    showDDLModal.value = true
    contextMenu.show = false
  } catch (error: any) {
    console.error('Failed to fetch DDL:', error)
  }
}

const openImportFromMenu = () => {
  importTableName.value = contextMenu.tableName
  showImportModal.value = true
  contextMenu.show = false
}

// 页面挂载时初始化数据
onMounted(() => {
  fetchConnections()
  loadHistory()
  window.addEventListener('click', handleGlobalClick)
})

onUnmounted(() => {
  window.removeEventListener('click', handleGlobalClick)
  document.removeEventListener('mousemove', handleResizeMove)
  document.removeEventListener('mouseup', handleResizeEnd)
})

const startResize = (e: MouseEvent) => {
  e.preventDefault()
  isResizing.value = true
  document.addEventListener('mousemove', handleResizeMove)
  document.addEventListener('mouseup', handleResizeEnd)
}

const handleResizeMove = (e: MouseEvent) => {
  if (!isResizing.value) return
  const newWidth = Math.max(180, Math.min(600, e.clientX))
  sidebarWidth.value = newWidth
}

const handleResizeEnd = () => {
  isResizing.value = false
  document.removeEventListener('mousemove', handleResizeMove)
  document.removeEventListener('mouseup', handleResizeEnd)
}
</script>

<template>
  <!-- 主容器：使用 Flex 布局占据全屏 -->
  <div class="fixed inset-0 flex w-full bg-slate-900 text-slate-100 overflow-hidden" :class="{ 'select-none': isResizing }">
    
    <!-- 左侧边栏：可拖拽调整宽度 -->
    <div :style="{ width: sidebarWidth + 'px' }" class="flex-shrink-0 border-r border-slate-700 flex flex-col relative">
      <!-- 拖拽手柄 -->
      <div
        @mousedown="startResize"
        class="absolute top-0 -right-1 w-3 h-full cursor-col-resize group z-10"
      >
        <div class="absolute top-0 right-1 w-0.5 h-full transition-colors"
          :class="isResizing ? 'bg-blue-500' : 'bg-transparent group-hover:bg-blue-500/50'"
        ></div>
      </div>
      <!-- 头部：标题和添加按钮 -->
      <div class="p-4 border-b border-slate-700">
        <div class="flex items-center justify-between mb-4">
          <h1 class="font-bold text-lg flex items-center gap-2">
            <Database class="w-5 h-5 text-blue-400" />
            SQL panel
          </h1>
          <div class="flex items-center gap-1">
            <button @click="showSettings = true" class="p-1 hover:bg-slate-700 rounded transition text-slate-400 hover:text-slate-200" title="Settings">
              <Settings class="w-5 h-5" />
            </button>
            <button @click="fetchExportLogs" class="p-1 hover:bg-slate-700 rounded transition text-slate-400 hover:text-slate-200" title="Export History">
              <History class="w-5 h-5" />
            </button>
            <button v-if="isAdmin" @click="showUserManagement = true" class="p-1 hover:bg-slate-700 rounded transition text-slate-400 hover:text-blue-400" title="Manage Users">
              <Users class="w-5 h-5" />
            </button>
            <button @click="openAddModal" class="p-1 hover:bg-slate-700 rounded transition" title="Add Connection">
              <Plus class="w-5 h-5" />
            </button>
          </div>
        </div>
        
        <!-- 侧边栏 Tab 切换 -->
        <div class="flex bg-slate-800 p-1 rounded-lg text-xs mb-4">
          <button @click="sidebarTab = 'connections'" 
                  class="flex-1 py-1.5 rounded-md transition flex items-center justify-center gap-1.5"
                  :class="sidebarTab === 'connections' ? 'bg-slate-700 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'">
            <Database class="w-3.5 h-3.5" />
            Connections
          </button>
          <button @click="sidebarTab = 'history'" 
                  class="flex-1 py-1.5 rounded-md transition flex items-center justify-center gap-1.5"
                  :class="sidebarTab === 'history' ? 'bg-slate-700 text-white shadow-sm' : 'text-slate-400 hover:text-slate-200'">
            <Clock class="w-3.5 h-3.5" />
            History
          </button>
        </div>

        <!-- 用户信息与退出 -->
        <div class="px-2 py-3 bg-slate-800/50 rounded-lg border border-slate-700 flex items-center justify-between">
          <div class="flex items-center gap-2 overflow-hidden">
            <div class="w-7 h-7 bg-blue-600 rounded-full flex items-center justify-center text-xs font-bold uppercase">
              {{ username[0] }}
            </div>
            <span class="text-xs font-medium truncate">{{ username }}</span>
          </div>
          <button @click="logout" class="p-1.5 hover:bg-slate-700 rounded-md text-slate-400 hover:text-red-400 transition" title="Logout">
            <LogOut class="w-4 h-4" />
          </button>
        </div>
      </div>
      
      <!-- 连接列表区域 -->
      <div v-if="sidebarTab === 'connections'" class="flex-1 min-h-0 flex flex-col">
        <!-- 连接列表和 Schema 拆成两个滚动区域，避免 Schema 表太多时把连接列表一起卷走 -->
        <div class="shrink-0 max-h-56 overflow-y-auto p-2 pb-3 border-b border-slate-700">
          <div class="text-xs font-semibold text-slate-500 uppercase px-2 mb-2 tracking-wider">Connections</div>
          <!-- 循环渲染连接 -->
          <div v-for="conn in connections" :key="conn.id" 
               @click="currentConnection = conn.id"
               class="group flex items-center justify-between p-2 rounded cursor-pointer mb-1 transition"
               :class="currentConnection === conn.id ? 'bg-blue-600 text-white' : 'hover:bg-slate-800'">
            <div class="flex items-center gap-2 overflow-hidden">
              <Database class="w-4 h-4 flex-shrink-0" />
              <span class="truncate">{{ conn.name }}</span>
            </div>
            <!-- 操作按钮：默认隐藏，鼠标悬停时显示 -->
            <div class="opacity-0 group-hover:opacity-100 flex items-center gap-1 transition">
              <!-- 分享按钮：仅创建者可见 -->
              <button v-if="conn.creatorId === currentUserId || isAdmin" 
                      @click.stop="openShareModal(conn.id)" 
                      class="p-1 hover:text-green-400 transition" title="Share Connection">
                <Share2 class="w-3.5 h-3.5" />
              </button>
              <button v-if="conn.creatorId === currentUserId" @click.stop="openEditModal(conn)" 
                      class="p-1 hover:text-blue-400 transition">
                <Edit2 class="w-3.5 h-3.5" />
              </button>
              <button v-if="conn.creatorId === currentUserId" @click.stop="deleteConnection(conn.id)" 
                      class="p-1 hover:text-red-400 transition">
                <Trash2 class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>
          <!-- 列表为空时的提示 -->
          <div v-if="connections.length === 0" class="p-4 text-center text-slate-500 text-sm italic">
            No connections yet. Click + to add.
          </div>
        </div>

        <!-- Schema 浏览器部分 -->
        <div v-if="currentConnection" class="flex-1 min-h-0 flex flex-col">
          <div class="shrink-0 bg-slate-900 px-4 py-2 text-xs font-semibold text-slate-500 uppercase tracking-wider flex items-center justify-between border-b border-slate-800">
            <span>Schema</span>
            <button @click="fetchSchema" class="hover:text-slate-300 transition" :class="{ 'animate-spin': loadingSchema }">
              <svg class="w-3 h-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
              </svg>
            </button>
          </div>
          <div class="shrink-0 px-3 py-2">
            <div class="relative">
              <Search class="absolute left-2 top-1/2 -translate-y-1/2 w-3 h-3 text-slate-500" />
              <input v-model="schemaSearch" type="text" placeholder="Filter tables or columns..."
                     @keydown="onSearchKeydown"
                     class="w-full bg-slate-800 border border-slate-700 rounded pl-7 pr-16 py-1.5 text-xs text-slate-300 placeholder-slate-500 focus:outline-none focus:border-blue-500 transition" />
              <div v-if="schemaSearch.trim()" class="absolute right-1 top-1/2 -translate-y-1/2 flex items-center gap-0.5">
                <span class="text-[10px] text-slate-500 mr-0.5">{{ matchIndex >= 0 ? matchIndex + 1 : 0 }}/{{ matchCount }}</span>
                <button @click="navigateMatch(-1)" class="p-0.5 rounded hover:bg-slate-700 text-slate-400 hover:text-slate-200 transition" title="Previous match (↑)">
                  <ChevronUp class="w-3 h-3" />
                </button>
                <button @click="navigateMatch(1)" class="p-0.5 rounded hover:bg-slate-700 text-slate-400 hover:text-slate-200 transition" title="Next match (↓)">
                  <ChevronDown class="w-3 h-3" />
                </button>
              </div>
            </div>
          </div>
          
          <div class="flex-1 min-h-0 overflow-y-auto px-2 py-2 schema-list-container">
            <div v-if="loadingSchema" class="px-4 py-2 text-xs text-slate-500">Loading schema...</div>
            <div v-else-if="filteredSchema.length === 0 && schema.length > 0" class="px-4 py-2 text-xs text-slate-500 italic">No matching tables.</div>
            <div v-else-if="schema.length === 0" class="px-4 py-2 text-xs text-slate-500 italic">No tables found.</div>
            <div v-else class="space-y-1">
              <div v-for="table in filteredSchema" :key="table.name" class="px-1">
                <!-- 表名行：增加右键监听 -->
                <div @click="toggleTable(table.name)" 
                     @contextmenu="showContextMenu($event, 'table', table.name)"
                     class="flex items-center gap-1.5 p-1.5 rounded hover:bg-slate-800 cursor-pointer text-sm text-slate-300 transition group">
                  <component :is="expandedTables.has(table.name) ? ChevronDown : ChevronRight" class="w-3.5 h-3.5 flex-shrink-0 text-slate-500" />
                  <Table2 class="w-4 h-4 flex-shrink-0 text-blue-500/70" />
                  <span class="truncate" v-html="highlightMatch(table.name)"></span>
                </div>
                
                <!-- 列信息（展开后显示）：增加右键监听 -->
                <div v-if="expandedTables.has(table.name)" class="ml-6 space-y-0.5 mt-0.5">
                  <div v-for="col in table.columns" :key="col.name" 
                       @contextmenu="showContextMenu($event, 'column', table.name, col.name)"
                       class="flex items-center gap-1.5 p-1 text-[11px] text-slate-500 hover:text-slate-300 transition cursor-default group">
                    <Columns class="w-3 h-3 flex-shrink-0 opacity-50" />
                    <span class="truncate" v-html="highlightMatch(col.name)"></span>
                    <span class="text-slate-600 flex-shrink-0">({{ normalizeColumnType(col) }})</span>
                    <span v-if="col.comment" class="text-slate-600 truncate opacity-0 group-hover:opacity-100 transition-opacity ml-1 italic">-- {{ col.comment }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 历史记录区域 -->
      <div v-else class="flex-1 overflow-y-auto p-2">
        <div class="text-xs font-semibold text-slate-500 uppercase px-2 mb-2 tracking-wider">Recent Queries</div>
        <div v-for="item in history" :key="item.id" 
             @click="openHistoryTab(item)"
             class="p-2 rounded cursor-pointer mb-1 hover:bg-slate-800 transition group border border-transparent hover:border-slate-700">
          <div class="flex items-start gap-2 mb-1">
            <div class="text-xs font-mono text-blue-400 truncate flex-1">{{ item.sql }}</div>
            <button
              @click.stop="deleteHistoryItem(item.id)"
              class="opacity-0 group-hover:opacity-100 p-0.5 rounded text-slate-500 hover:text-red-400 hover:bg-red-500/10 transition"
              title="Delete history"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
          <div class="flex items-center justify-between text-[10px] text-slate-500">
            <span>{{ item.connectionName }}</span>
            <span>{{ item.time }}</span>
          </div>
        </div>
        <div v-if="history.length === 0" class="p-4 text-center text-slate-500 text-sm italic">
          No query history yet.
        </div>
      </div>
    </div>

    <!-- 右侧主内容区：上下分割 -->
    <div class="flex-1 flex flex-col overflow-hidden">
      
      <SqlEditor
        v-model="sql"
        :connection-name="connections.find(c => c.id === currentConnection)?.name || ''"
        :has-connection="!!currentConnection"
        :loading="loading"
        :monaco-theme="MONACO_THEME"
        :monaco-options="MONACO_OPTIONS"
        :handle-before-mount="handleBeforeMount"
        :handle-mount="handleMount"
        :tabs="tabs"
        :active-tab-id="activeTabId"
        :transaction-mode="transactionMode"
        :transaction-active="transactionActive"
        @format="formatSQL"
        @run="runQuery"
        @switch-tab="switchTab"
        @close-tab="closeTab"
        @add-tab="addTab()"
        @update:transaction-mode="transactionMode = $event"
        @begin-transaction="beginTransaction"
        @commit-transaction="commitTransaction"
        @rollback-transaction="rollbackTransaction"
      />

      <ResultsPanel
        v-model:result-view-tab="resultViewTab"
        :results="results"
        :loading="loading"
        :hidden-columns="hiddenColumns"
        :sorted-data="sortedData"
        :visible-columns="visibleColumns"
        :sort-column="sortConfig.column"
        :sort-order="sortConfig.order"
        @export-csv="exportToCSV"
        @export-excel="exportToExcel"
        @toggle-column="toggleColumnVisibility"
        @toggle-sort="toggleSort"
      />
    </div>
  </div>

  <ToastNotification
    :show="toast.show"
    :type="toast.type"
    :message="toast.message"
    @close="hideToast"
  />

  <ConnectionModal
    :show="showModal"
    :is-editing="isEditing"
    :connection="newConnection"
    @close="closeModal"
    @save="saveConnection"
  />

  <ImportModal
    :show="showImportModal"
    :connection-id="currentConnection"
    :schema="schema"
    :table-name="importTableName"
    @close="showImportModal = false"
    @imported="fetchConnections"
  />

  <div v-if="showExportHistory" class="fixed inset-0 z-[110] flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm" @click.self="showExportHistory = false">
    <div class="bg-slate-800 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-2xl max-h-[80vh] overflow-hidden flex flex-col">
      <div class="flex items-center justify-between p-4 border-b border-slate-700">
        <h3 class="font-bold text-lg flex items-center gap-2">
          <History class="w-5 h-5 text-blue-400" />
          Export History
        </h3>
        <button @click="showExportHistory = false" class="text-slate-400 hover:text-white transition">
          <svg class="w-5 h-5" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" /></svg>
        </button>
      </div>
      <div class="flex-1 overflow-y-auto p-4">
        <div v-if="exportLogs.length === 0" class="text-center text-slate-500 py-8 text-sm italic">No export records yet.</div>
        <div v-else class="space-y-2">
          <div v-for="log in exportLogs" :key="log.id" class="bg-slate-900 rounded-lg p-3 border border-slate-700/50">
            <div class="flex items-center justify-between mb-2">
              <div class="flex items-center gap-2">
                <span class="text-[10px] px-1.5 py-0.5 rounded font-bold uppercase"
                      :class="log.fileType === 'Excel' ? 'bg-green-900/50 text-green-400' : 'bg-blue-900/50 text-blue-400'">
                  {{ log.fileType }}
                </span>
                <span class="text-xs text-slate-400">{{ log.createdAt }}</span>
              </div>
              <div class="flex items-center gap-3 text-xs text-slate-500">
                <span>{{ log.rowCount }} rows</span>
                <span>{{ log.username }}</span>
              </div>
            </div>
            <div class="text-xs text-slate-500 mb-1">{{ log.connectionName }}</div>
            <code class="text-xs text-slate-300 bg-slate-950 block p-2 rounded border border-slate-700/50 max-h-20 overflow-y-auto whitespace-pre-wrap">{{ log.sqlStatement }}</code>
            <div class="text-[10px] text-slate-600 mt-1 truncate">{{ log.fileName }}</div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Custom Context Menu -->
  <div v-if="contextMenu.show" 
       :style="{ left: contextMenu.x + 'px', top: contextMenu.y + 'px' }"
       class="fixed z-[200] bg-slate-800 border border-slate-700 rounded-lg shadow-2xl py-1 w-48 animate-in fade-in zoom-in duration-100"
       @click.stop>
    
    <!-- Table Context Items -->
    <template v-if="contextMenu.type === 'table'">
      <button @click="insertSQL('SELECT * FROM ' + contextMenu.tableName + ';')" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-green-400">
        <PlayCircle class="w-3.5 h-3.5" /> Select All (SELECT *)
      </button>
      <button @click="insertAllColumnsSQL(contextMenu.tableName)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-green-400">
        <PlayCircle class="w-3.5 h-3.5" /> Select All Columns
      </button>
      <button @click="insertInsertSQL(contextMenu.tableName)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-green-400">
        <Plus class="w-3.5 h-3.5" /> Insert Statement
      </button>
      <div class="h-px bg-slate-700 my-1"></div>
      <button @click="copyToClipboard(contextMenu.tableName)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition">
        <Copy class="w-3.5 h-3.5" /> Copy Table Name
      </button>
      <button @click="copyToClipboard(`${contextMenu.dbName}.${contextMenu.tableName}`)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition">
        <Copy class="w-3.5 h-3.5" /> Copy Full Name (DB.Table)
      </button>
      <div class="h-px bg-slate-700 my-1"></div>
      <button @click="viewTableDDL(contextMenu.tableName)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-blue-400">
        <Code2 class="w-3.5 h-3.5" /> View Create Statement
      </button>
      <div class="h-px bg-slate-700 my-1"></div>
      <button @click="openImportFromMenu"
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-yellow-400">
        <Upload class="w-3.5 h-3.5" /> Import Data
      </button>
    </template>

    <!-- Column Context Items -->
    <template v-if="contextMenu.type === 'column'">
      <button @click="insertSQL('SELECT ' + contextMenu.columnName + ' FROM ' + contextMenu.tableName + ' LIMIT ' + globalSettings.limit + ';')" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition text-green-400">
        <PlayCircle class="w-3.5 h-3.5" /> Select Column
      </button>
      <div class="h-px bg-slate-700 my-1"></div>
      <button @click="copyToClipboard(contextMenu.columnName)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition">
        <Copy class="w-3.5 h-3.5" /> Copy Column Name
      </button>
      <button @click="copyToClipboard(`${contextMenu.tableName}.${contextMenu.columnName}`)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition">
        <Copy class="w-3.5 h-3.5" /> Copy Table.Column
      </button>
      <button @click="copyToClipboard(`${contextMenu.dbName}.${contextMenu.tableName}.${contextMenu.columnName}`)" 
              class="w-full text-left px-3 py-2 text-xs hover:bg-blue-600 flex items-center gap-2 transition">
        <Copy class="w-3.5 h-3.5" /> Copy Full Name (DB.T.C)
      </button>
    </template>
  </div>

  <AuthModal
    :show="showAuthModal"
    :mode="authMode"
    :form="authForm"
    :error="authError"
    :success="authSuccess"
    @submit="login()"
  />

  <SettingsModal
    :show="showSettings"
    :settings="globalSettings"
    :isAdmin="isAdmin"
    @close="showSettings = false"
    @imported="fetchConnections"
  />

  <UserManagementModal
    :show="showUserManagement"
    @close="showUserManagement = false"
  />

  <ShareModal
    :show="showShareModal"
    :connectionId="shareConnectionId"
    :isAdmin="isAdmin"
    :currentUserId="currentUserId"
    @close="closeModal"
    @changed="fetchConnections"
  />

  <DDLModal
    v-model:ddl="currentDDL"
    :show="showDDLModal"
    :table-name="ddlTableName"
    :monaco-theme="MONACO_THEME"
    :monaco-options="MONACO_OPTIONS"
    :handle-before-mount="handleBeforeMount"
    @close="showDDLModal = false"
    @copy="copyToClipboard(currentDDL)"
  />
</template>

<style scoped>
:deep(.schema-highlight) {
  background: rgba(59, 130, 246, 0.35);
  color: #93c5fd;
  border-radius: 2px;
  padding: 0 1px;
}
:deep(.schema-highlight-active) {
  background: rgba(250, 204, 21, 0.5);
  color: #fde68a;
  border-radius: 2px;
  padding: 0 1px;
  outline: 1px solid rgba(250, 204, 21, 0.6);
}
</style>
