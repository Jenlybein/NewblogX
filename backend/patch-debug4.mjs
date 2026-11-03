import fs from 'node:fs'

const p = 'internal/article/adapter/mysql/outline_repo.go'
let t = fs.readFileSync(p, 'utf8')
const needle = '\tif int(res.RowsAffected) != len(ids) {\n\t\treturn outline.ErrNodeMissing\n\t}'
if (!t.includes(needle)) {
  console.error('needle missing')
  process.exit(1)
}
t = t.replace(
  needle,
  '\tif int(res.RowsAffected) != len(ids) {\n\t\tfmt.Fprintf(os.Stderr, "DBG_SOFTDELETE want=%d got=%d ids=%v sql=%s\\n", len(ids), res.RowsAffected, raw, res.Statement.SQL.String())\n\t\treturn outline.ErrNodeMissing\n\t}',
)
if (!t.includes('"fmt"')) {
  t = t.replace('import (\n', 'import (\n\t"fmt"\n')
}
if (!t.includes('"os"')) {
  t = t.replace('import (\n', 'import (\n\t"os"\n')
}
fs.writeFileSync(p, t, 'utf8')
console.log('patched SoftDelete')
