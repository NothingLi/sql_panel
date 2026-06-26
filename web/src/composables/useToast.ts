import { onUnmounted, reactive } from 'vue'

export type ToastType = 'error' | 'success'

/**
 * 统一管理应用里的右上角 Toast 提示。
 *
 * composable 的好处：
 * - 状态和操作函数放在一起，App.vue 不需要关心计时器细节。
 * - 以后其他组件也想弹 toast，可以复用这份逻辑。
 */
export function useToast() {
  const toast = reactive({
    show: false,
    type: 'error' as ToastType,
    message: '',
  })

  let toastTimer: ReturnType<typeof setTimeout> | null = null

  const showToast = (message: string, type: ToastType = 'error') => {
    if (toastTimer) {
      clearTimeout(toastTimer)
    }

    toast.message = message
    toast.type = type
    toast.show = true

    toastTimer = setTimeout(() => {
      toast.show = false
      toastTimer = null
    }, 3000)
  }

  const hideToast = () => {
    toast.show = false
    if (toastTimer) {
      clearTimeout(toastTimer)
      toastTimer = null
    }
  }

  const getRequestErrorMessage = (error: any) => {
    if (!error.response) {
      return '网络请求失败，请检查网络连接或后端服务。'
    }

    if (error.response.status === 401) {
      return '登录已过期，请重新登录。'
    }

    return error.response.data?.error
      || error.response.data?.message
      || `请求失败（${error.response.status}）`
  }

  onUnmounted(() => {
    if (toastTimer) {
      clearTimeout(toastTimer)
    }
  })

  return {
    toast,
    showToast,
    hideToast,
    getRequestErrorMessage,
  }
}
