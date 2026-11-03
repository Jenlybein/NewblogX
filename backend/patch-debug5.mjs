import fs from 'node:fs'

const p = 'internal/article/adapter/http/handler_outline.go'
let t = fs.readFileSync(p, 'utf8')
const needle = '\tif err != nil {\n\t\th.fail(c, err)\n\t\treturn\n\t}\n\temitOutlineAudit(c, kind, ownerID)'
if (!t.includes(needle)) {
  console.error('needle missing')
  process.exit(1)
}
t = t.replace(
  needle,
  '\tif err != nil {\n\t\tbuf := make([]byte, 8192)\n\t\tn := runtime.Stack(buf, false)\n\t\tfmt.Fprintf(os.Stderr, "DBG_ERR %T %v\\n%s\\n", err, err, buf[:n])\n\t\th.fail(c, err)\n\t\treturn\n\t}\n\temitOutlineAudit(c, kind, ownerID)',
)
for (const imp of ['fmt', 'os', 'runtime']) {
  if (!t.includes('"' + imp + '"')) {
    t = t.replace('import (\n', 'import (\n\t"' + imp + '"\n')
  }
}
fs.writeFileSync(p, t, 'utf8')
console.log('patched handler error path')
