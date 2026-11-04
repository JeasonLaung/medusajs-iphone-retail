export const contactCsvTemplate = "\uFEFFemail,name,notes\r\n"

export function parseContactCsv(input: string) {
  const rows: string[][] = []
  let row: string[] = []
  let value = ""
  let quoted = false
  const text = input.replace(/^\uFEFF/, "")
  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (char === '"') {
      if (quoted && text[i + 1] === '"') {
        value += '"'
        i++
      } else if (quoted || !value.trim()) quoted = !quoted
      else throw new Error("invalidCsv")
    } else if (!quoted && (char === "," || char === "\n" || char === "\r")) {
      row.push(value.trim())
      value = ""
      if (char !== ",") {
        if (row.some(Boolean)) rows.push(row)
        row = []
        if (char === "\r" && text[i + 1] === "\n") i++
      }
    } else value += char
  }
  if (quoted) throw new Error("invalidCsv")
  row.push(value.trim())
  if (row.some(Boolean)) rows.push(row)
  const aliases: Record<string, string> = {
    email: "email",
    邮箱: "email",
    name: "name",
    名称: "name",
    姓名: "name",
    notes: "notes",
    备注: "notes",
  }
  const first = rows[0] || []
  const hasHeader = first.some(
    (cell) => aliases[cell.toLowerCase()] === "email"
  )
  const columns = hasHeader
    ? rows.shift()!.map((cell) => aliases[cell.toLowerCase()])
    : ["email", "name", "notes"]
  if (
    hasHeader &&
    (columns.some((column) => !column) ||
      new Set(columns).size !== columns.length)
  )
    throw new Error("invalidCsv")
  if (!rows.length || rows.length > 1000) throw new Error("invalidCsv")
  return rows.map((cells) => {
    if (cells.length > columns.length) throw new Error("invalidCsv")
    const email = cells[columns.indexOf("email")] || ""
    const name = cells[columns.indexOf("name")] || ""
    const notesIndex = columns.indexOf("notes")
    const notes =
      notesIndex >= 0 && notesIndex < cells.length
        ? cells[notesIndex]
        : undefined
    if (
      !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(email) ||
      name.length > 255 ||
      (notes?.length || 0) > 5000
    )
      throw new Error("invalidCsv")
    return { email, name, ...(notes !== undefined ? { notes } : {}) }
  })
}
