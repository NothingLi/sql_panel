/**
 * SQL 函数智能提示静态库类型定义。
 *
 * 每个数据库类型维护一份函数清单，补全时按 dbType + version 过滤。
 * version 为主版本号字符串（如 "14"），未探测到时为空字符串，此时只按 minVersion 过滤。
 */

/** 函数分类，用于 Monaco 补全的 detail 字段展示。 */
export type FunctionCategory =
  | 'string'
  | 'datetime'
  | 'numeric'
  | 'aggregate'
  | 'window'
  | 'json'
  | 'system'
  | 'array'
  | 'conversion'

/** 单个 SQL 函数定义。 */
export interface FunctionDef {
  /** 函数名，如 COALESCE */
  name: string
  /** 签名，如 COALESCE(val1, val2, ...) */
  signature: string
  /** 中文说明 */
  description: string
  /** 分类 */
  category: FunctionCategory
  /** 最低主版本号，如 "14" 表示仅 14+ 可用。省略表示全版本通用 */
  minVersion?: string
  /** snippet 插入文本，省略则按签名自动生成 */
  insertText?: string
}

/** 支持的数据库类型（与后端 models.DBType 对应）。 */
export type DBType = 'postgres' | 'mysql' | 'sqlite' | 'sqlserver'
