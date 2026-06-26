import { onUnmounted, shallowRef, type Ref } from 'vue'

const SQL_COMPLETION_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'ON',
  'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT', 'OFFSET', 'INSERT INTO', 'VALUES',
  'UPDATE', 'SET', 'DELETE FROM', 'CREATE TABLE', 'ALTER TABLE', 'DROP TABLE',
  'DISTINCT', 'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'AS', 'AND', 'OR', 'NOT',
  'NULL', 'IS NULL', 'IS NOT NULL', 'IN', 'BETWEEN', 'LIKE', 'CASE', 'WHEN',
  'THEN', 'ELSE', 'END', 'UNION', 'UNION ALL'
]

const SQL_COMPLETION_SNIPPETS = [
  {
    label: 'SELECT statement',
    insertText: 'SELECT ${1:*}\nFROM ${2:table_name}\nWHERE ${3:condition};',
    detail: 'snippet',
  },
  {
    label: 'JOIN statement',
    insertText: 'SELECT ${1:*}\nFROM ${2:table_a}\nJOIN ${3:table_b} ON ${4:table_a.id = table_b.id};',
    detail: 'snippet',
  },
  {
    label: 'GROUP BY statement',
    insertText: 'SELECT ${1:column}, COUNT(*)\nFROM ${2:table_name}\nGROUP BY ${1:column};',
    detail: 'snippet',
  },
]

/**
 * Monaco SQL 编辑器相关逻辑。
 *
 * 这个 composable 专门负责 Monaco 这类“第三方编辑器集成”：
 * - 保存 editor 实例
 * - 注册 SQL 主题
 * - 注册 SQL 补全
 * - 注册 Ctrl/Cmd + Enter 快捷键
 *
 * App.vue 只需要把 schema 和 runQuery 传进来。
 */
export function useSqlMonaco(schema: Ref<any[]>, runQuery: () => void) {
  const editorRef = shallowRef<any>()
  const MONACO_THEME = 'sql-dark'
  let completionProviderDisposable: any = null

  const MONACO_OPTIONS = {
    automaticLayout: true,
    minimap: { enabled: false },
    fontSize: 14,
    padding: { top: 16, bottom: 16 },
    scrollBeyondLastLine: false,
    wordWrap: 'on',
    wrappingIndent: 'same',
    scrollbar: {
      horizontal: 'hidden',
      alwaysConsumeMouseWheel: false,
    },
    lineNumbers: 'on',
    renderLineHighlight: 'all',
    fontFamily: "'Fira Code', 'JetBrains Mono', monospace",
    fontLigatures: true,
    quickSuggestions: {
      other: true,
      comments: false,
      strings: false,
    },
    suggestOnTriggerCharacters: true,
    acceptSuggestionOnEnter: 'on',
    tabCompletion: 'on',
    snippetSuggestions: 'top',
  }

  const defineSqlTheme = (monacoInstance: any) => {
    monacoInstance.editor.defineTheme(MONACO_THEME, {
      base: 'vs-dark',
      inherit: true,
      rules: [
        { token: 'keyword.sql', foreground: '60a5fa', fontStyle: 'bold' },
        { token: 'operator.sql', foreground: 'f472b6' },
        { token: 'string.sql', foreground: '34d399' },
        { token: 'number.sql', foreground: 'fbbf24' },
        { token: 'comment.sql', foreground: '64748b', fontStyle: 'italic' },
        { token: 'identifier.sql', foreground: 'e2e8f0' },
        { token: 'type.sql', foreground: 'a78bfa' },
        { token: 'keyword', foreground: '60a5fa', fontStyle: 'bold' },
        { token: 'string', foreground: '34d399' },
        { token: 'number', foreground: 'fbbf24' },
        { token: 'comment', foreground: '64748b', fontStyle: 'italic' },
      ],
      colors: {
        'editor.background': '#020617',
        'editor.foreground': '#e2e8f0',
        'editorLineNumber.foreground': '#475569',
        'editorCursor.foreground': '#38bdf8',
        'editor.selectionBackground': '#1e3a8a88',
        'editor.lineHighlightBackground': '#0f172a',
      },
    })
  }

  const registerSqlCompletionProvider = (monacoInstance: any) => {
    if (completionProviderDisposable) return

    completionProviderDisposable = monacoInstance.languages.registerCompletionItemProvider('sql', {
      triggerCharacters: [' ', '.', ','],
      provideCompletionItems: (model: any, position: any) => {
        const word = model.getWordUntilPosition(position)
        const range = {
          startLineNumber: position.lineNumber,
          endLineNumber: position.lineNumber,
          startColumn: word.startColumn,
          endColumn: word.endColumn,
        }

        const keywordSuggestions = SQL_COMPLETION_KEYWORDS.map(keyword => ({
          label: keyword,
          kind: monacoInstance.languages.CompletionItemKind.Keyword,
          insertText: keyword,
          range,
        }))

        const snippetSuggestions = SQL_COMPLETION_SNIPPETS.map(snippet => ({
          label: snippet.label,
          kind: monacoInstance.languages.CompletionItemKind.Snippet,
          insertText: snippet.insertText,
          insertTextRules: monacoInstance.languages.CompletionItemInsertTextRule.InsertAsSnippet,
          detail: snippet.detail,
          range,
        }))

        const tableSuggestions = schema.value.map(table => ({
          label: table.name,
          kind: monacoInstance.languages.CompletionItemKind.Struct,
          insertText: table.name,
          detail: 'table',
          range,
        }))

        const columnSuggestions = schema.value.flatMap(table => {
          return (table.columns || []).map((column: string) => {
            const columnName = column.split(' (')[0]
            return {
              label: `${columnName} (${table.name})`,
              kind: monacoInstance.languages.CompletionItemKind.Field,
              insertText: columnName,
              detail: column,
              range,
            }
          })
        })

        return {
          suggestions: [
            ...snippetSuggestions,
            ...keywordSuggestions,
            ...tableSuggestions,
            ...columnSuggestions,
          ]
        }
      }
    })
  }

  const handleBeforeMount = (monacoInstance: any) => {
    defineSqlTheme(monacoInstance)
    registerSqlCompletionProvider(monacoInstance)
  }

  const handleMount = (editor: any, monacoInstance: any) => {
    editorRef.value = editor
    monacoInstance.editor.setTheme(MONACO_THEME)
    monacoInstance.editor.setModelLanguage(editor.getModel(), 'sql')
    editor.addCommand(monacoInstance.KeyMod.CtrlCmd | monacoInstance.KeyCode.Enter, () => {
      runQuery()
    })
  }

  onUnmounted(() => {
    completionProviderDisposable?.dispose()
    completionProviderDisposable = null
  })

  return {
    editorRef,
    MONACO_THEME,
    MONACO_OPTIONS,
    handleBeforeMount,
    handleMount,
  }
}
