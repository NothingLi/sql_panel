import type { FunctionDef, DBType } from './types'
import { POSTGRES_FUNCTIONS } from './postgres'
import { MYSQL_FUNCTIONS } from './mysql'
import { SQLITE_FUNCTIONS } from './sqlite'

/** 各数据库的完整函数库。SQLServer 暂未实现，预留空数组。 */
const FUNCTION_REGISTRY: Record<DBType, FunctionDef[]> = {
  postgres: POSTGRES_FUNCTIONS,
  mysql: MYSQL_FUNCTIONS,
  sqlite: SQLITE_FUNCTIONS,
  sqlserver: [],
}

/**
 * 比较两个主版本号字符串。
 * 版本号为空时视为最小（不满足任何 minVersion 要求的反面：满足全部无 minVersion 的函数）。
 * 这里用数字比较：若任一为空返回 -2（视为 0），否则按数字大小比较。
 */
function compareVersion(a: string, b: string): number {
  const na = parseInt(a, 10)
  const nb = parseInt(b, 10)
  const va = isNaN(na) ? 0 : na
  const vb = isNaN(nb) ? 0 : nb
  return va - vb
}

/**
 * 按数据库类型 + 版本过滤函数。
 * version 为空时返回所有无 minVersion 限制的函数（版本未知，只给通用函数）。
 * version 非空时返回 minVersion <= version 的所有函数（含无限制的）。
 */
export function getFunctions(dbType: DBType | string | undefined, version?: string): FunctionDef[] {
  const all = FUNCTION_REGISTRY[(dbType as DBType)] || []
  return all.filter(fn => {
    if (!fn.minVersion) return true
    if (!version) return false
    return compareVersion(version, fn.minVersion) >= 0
  })
}

/** 分类中文标签，用于补全 detail 展示。 */
export const CATEGORY_LABELS: Record<string, string> = {
  string: '字符串函数',
  datetime: '日期时间',
  numeric: '数值函数',
  aggregate: '聚合函数',
  window: '窗口函数',
  json: 'JSON 函数',
  system: '系统函数',
  array: '数组函数',
  conversion: '转换/流程',
}

export type { FunctionDef, DBType, FunctionCategory } from './types'
