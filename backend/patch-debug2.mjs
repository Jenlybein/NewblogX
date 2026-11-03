import fs from 'node:fs'

const handlers = 'internal/article/adapter/http/handler_outline.go'
let h = fs.readFileSync(handlers, 'utf8')
const needle = `\tdetail, err := h.svc.SaveOutline(c.Request.Context(), app.SaveOutlineCmd{`
if (!h.includes(needle)) {
  console.error('handler needle missing')
  process.exit(1)
}
const after = `\tdetail, err := h.svc.SaveOutline(c.Request.Context(), app.SaveOutlineCmd{`
// insert before the call: dump the parsed nodes
h = h.replace(needle, `\tfor _, n := range nodes {\n\t\tprintln("DBG_NODE", "id="+string(n.ID), "kind="+string(rune(n.Kind+'0')), "article="+string(n.ArticleID), "children="+string(rune(len(n.Children)+'0')))\n\t}\n\tprintln("DBG_CMD", "owner="+string(rune(kind+'0')), "ownerID="+string(rune(0)))\n` + after)
fs.writeFileSync(handlers, h, 'utf8')
console.log('handler patched')
