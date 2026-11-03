import fs from 'node:fs'

const p = 'internal/article/app/outline.go'
let t = fs.readFileSync(p, 'utf8')

const anchor = '\tplanned := make([]*plannedNode, 0, len(cmd.Nodes))'
if (!t.includes(anchor)) {
  console.error('anchor missing')
  process.exit(1)
}
t = t.replace(
  anchor,
  '\tfmt.Fprintf(os.Stderr, "DBG_CUR owner=%d ownerID=%d current=%d byID=%d\\n", int(cmd.OwnerKind), cmd.OwnerID.Uint64(), len(current), len(byID))\n' +
    '\tfor n := range byID {\n\t\tfmt.Fprintf(os.Stderr, "DBG_KEY %s\\n", string(n))\n\t}\n' +
    anchor,
)

const miss = '\t\t\texisting = byID[in.ID]\n\t\t\tif existing == nil {'
if (!t.includes(miss)) {
  console.error('miss anchor missing')
  process.exit(1)
}
t = t.replace(
  miss,
  '\t\t\texisting = byID[in.ID]\n\t\t\tfmt.Fprintf(os.Stderr, "DBG_LOOKUP in=%s found=%v\\n", string(in.ID), existing != nil)\n\t\t\tif existing == nil {',
)

if (!t.includes('"fmt"')) {
  t = t.replace('import (\n\t"context"', 'import (\n\t"context"\n\t"fmt"')
}
if (!t.includes('"os"')) {
  t = t.replace('import (\n\t"context"', 'import (\n\t"context"\n\t"os"')
}
fs.writeFileSync(p, t, 'utf8')
console.log('patched planOutline with fmt+os')
