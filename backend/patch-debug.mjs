import fs from 'node:fs'

const path = 'internal/article/app/outline.go'
let text = fs.readFileSync(path, 'utf8')
const needle = `\t\t\texisting = byID[in.ID]\n\t\t\tif existing == nil {\n\t\t\t\treturn nil, outline.ErrNodeMissing`
const replacement = `\t\t\texisting = byID[in.ID]\n\t\t\tif existing == nil {\n\t\t\t\tkeys := make([]string, 0, len(byID))\n\t\t\t\tfor k := range byID { keys = append(keys, string(k)) }\n\t\t\t\tprintln("DBG_MISS", "in="+string(in.ID), "kind="+string(rune(in.Kind+'0')), "keys="+fmt.Sprint(keys), "owner="+string(rune(cmd.OwnerKind+'0')), "ownerID="+fmt.Sprint(cmd.OwnerID.Uint64()), "current="+fmt.Sprint(len(current)))\n\t\t\t\treturn nil, outline.ErrNodeMissing`
if (!text.includes(needle)) {
  console.error('needle not found')
  process.exit(1)
}
text = text.replace(needle, replacement)
if (!text.includes('"fmt"')) {
  text = text.replace('import (\n', 'import (\n\t"fmt"\n')
}
fs.writeFileSync(path, text, 'utf8')
console.log('patched')
