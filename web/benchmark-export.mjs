import * as XLSX from 'xlsx'

const COLUMNS = ['id', 'name', 'email', 'city', 'amount', 'status', 'created_at', 'category', 'score', 'description']
const SCALES = [
  { label: '1万条', rows: 10_000 },
  { label: '5万条', rows: 50_000 },
  { label: '10万条', rows: 100_000 },
]

function generateData(count) {
  const data = []
  for (let i = 0; i < count; i++) {
    data.push([
      i + 1,
      `用户_${i}`,
      `user${i}@example.com`,
      ['北京', '上海', '广州', '深圳', '杭州'][i % 5],
      (Math.random() * 10000).toFixed(2),
      ['active', 'inactive', 'pending'][i % 3],
      `2024-${String((i % 12) + 1).padStart(2, '0')}-${String((i % 28) + 1).padStart(2, '0')}`,
      ['技术', '产品', '设计', '运营', '市场'][i % 5],
      Math.floor(Math.random() * 100),
      `这是一段描述文本_${i}`,
    ])
  }
  return data
}

function benchmarkCSV(columns, data) {
  const start = performance.now()
  const headers = columns.join(',')
  const rows = data.map(row =>
    row.map(cell => {
      const str = String(cell ?? '')
      if (str.includes(',') || str.includes('"') || str.includes('\n')) {
        return `"${str.replace(/"/g, '""')}"`
      }
      return str
    }).join(',')
  )
  const csv = '\ufeff' + [headers, ...rows].join('\n')
  const elapsed = performance.now() - start
  const sizeMB = (Buffer.byteLength(csv, 'utf8') / 1024 / 1024).toFixed(2)
  return { elapsed, sizeMB }
}

function benchmarkExcel(columns, data) {
  const start = performance.now()
  const exportData = data.map(row => {
    const obj = {}
    columns.forEach((col, i) => { obj[col] = row[i] })
    return obj
  })
  const ws = XLSX.utils.json_to_sheet(exportData)
  const wb = XLSX.utils.book_new()
  XLSX.utils.book_append_sheet(wb, ws, 'Results')
  const buf = XLSX.write(wb, { type: 'buffer', bookType: 'xlsx' })
  const elapsed = performance.now() - start
  const sizeMB = (buf.length / 1024 / 1024).toFixed(2)
  return { elapsed, sizeMB }
}

function run() {
  console.log('='.repeat(65))
  console.log('  导出性能测试 (CSV vs Excel)')
  console.log('='.repeat(65))
  console.log(`  列数: ${COLUMNS.length}  |  列名: ${COLUMNS.join(', ')}`)
  console.log('-'.repeat(65))

  for (const scale of SCALES) {
    console.log(`\n  📊 ${scale.label} (${scale.rows.toLocaleString()} 行)`)
    const data = generateData(scale.rows)

    const csv = benchmarkCSV(COLUMNS, data)
    console.log(`    CSV   → 耗时: ${csv.elapsed.toFixed(1)}ms  |  文件大小: ${csv.sizeMB} MB`)

    const excel = benchmarkExcel(COLUMNS, data)
    console.log(`    Excel → 耗时: ${excel.elapsed.toFixed(1)}ms  |  文件大小: ${excel.sizeMB} MB`)

    const ratio = (excel.elapsed / csv.elapsed).toFixed(1)
    console.log(`    Excel 比 CSV 慢 ${ratio}x`)
  }

  console.log('\n' + '='.repeat(65))
  console.log('  测试完成')
  console.log('='.repeat(65))
}

run()