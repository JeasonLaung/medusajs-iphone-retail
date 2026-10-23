const fs = require("node:fs")
const path = require("node:path")

const root = path.resolve(__dirname, "..")
const folder = path.join(root, "imports/swappie-buyback-prices")
const manifest = JSON.parse(fs.readFileSync(path.join(folder, "manifest.json"), "utf8"))
const round = (n) => Math.round(n * 100) / 100
const faults = ["BROKEN", "BROKEN_SCREEN", "BATTERY_ISSUE", "BROKEN_FRAME", "BROKEN_BACK_CAMERA"]
const configs = []
let tested = 0, matched = 0
for (const entry of manifest.files) {
  if (entry.http_status !== 200 || entry.error) throw new Error(`Incomplete source: ${entry.model}`)
  const source = JSON.parse(fs.readFileSync(path.join(folder, entry.file), "utf8"))
  for (const name of [...new Set(source.results.map((r) => r.model_name))]) {
    const rows = source.results.filter((r) => r.model_name === name)
    if (rows.length !== 160 || rows.some((r) => r.price.currency !== "EUR")) throw new Error(`Unexpected matrix: ${name}`)
    const matrix = Object.fromEntries(rows.map((r) => [`${r.visual_condition}:${[...r.functional_condition].sort().join(",")}`, { amount: r.price.price, purchasable: r.purchasable }]))
    const bases = Object.fromEntries(rows.filter((r) => !r.functional_condition.length).map((r) => [r.visual_condition, r.price.price]))
    const floor = rows[0].price.limit_price
    // Derive the shared damage fee from unclipped single/pair observations.
    const fees = []
    for (const visual of Object.keys(bases)) {
      for (let i = 0; i < faults.length; i++) for (let j = i + 1; j < faults.length; j++) {
        const a = matrix[`${visual}:${faults[i]}`]?.amount
        const b = matrix[`${visual}:${faults[j]}`]?.amount
        const pair = matrix[`${visual}:${[faults[i], faults[j]].sort().join(",")}`]?.amount
        if (a > floor && b > floor && pair > floor) fees.push(round(bases[visual] - a - b + pair))
      }
    }
    const frequency = new Map()
    fees.forEach((fee) => frequency.set(fee, (frequency.get(fee) || 0) + 1))
    const fee = [...frequency].sort((a, b) => b[1] - a[1])[0]?.[0] ?? null
    const costs = Object.fromEntries(faults.map((fault) => {
      const observations = Object.keys(bases).filter((visual) => matrix[`${visual}:${fault}`].amount > floor)
        .map((visual) => round(bases[visual] - matrix[`${visual}:${fault}`].amount - (fee || 0)))
      return [fault, observations[0] ?? null]
    }))
    let matches = 0
    for (const row of rows) {
      const predicted = row.functional_condition.length ? Math.max(floor, round(bases[row.visual_condition] - (fee || 0) - row.functional_condition.reduce((sum, fault) => sum + (costs[fault] || 0), 0))) : bases[row.visual_condition]
      if (Math.abs(predicted - row.price.price) < 0.011) matches++
    }
    tested += rows.length; matched += matches
    configs.push({ model: source.model, capacity: name.slice(source.model.length).trim().replace("1024GB", "1TB").replace("2048GB", "2TB"), source_file: entry.file, fetched_at: source.fetched_at, matrix, bases, floor, damage_fee: fee, fault_costs: costs, additive_matches: matches, total: rows.length })
  }
}
const output = { source: manifest.source, country: manifest.country, currency: "EUR", fetched_at: manifest.requested_at, model_count: manifest.models.length, row_count: tested, additive_matches: matched, configurations: configs }
const destination = path.join(root, "medusa-store/apps/backend/src/lib/data/swappie-buyback-reference.json")
fs.mkdirSync(path.dirname(destination), { recursive: true })
fs.writeFileSync(destination, JSON.stringify(output) + "\n")
const report = ["# Swappie 回收价格分析", "", `来源：2026-10-22 抓取的 BE / EUR 数据。${manifest.models.length} 个型号，${configs.length} 个型号容量组合，${tested} 条原始报价。`, "", "## 可复核的规律", "", "- 每个型号容量有 5 种成色 × 32 种故障组合 = 160 条报价。SEALED_BOX 是未拆封，不映射到二手问卷。", "- 抓取只请求了 256GB / 512GB / 1024GB / 2048GB；缺少 64GB / 128GB，不能据此当作已抓取价格。", `- 拟合公式：无故障时直接取成色基价；有故障时 max(保底价, 成色基价 − 一次性故障费用 − 各故障维修扣款之和)。该公式复现 ${matched}/${tested} 条（${round(matched / tested * 100)}%）；未解释的组合必须保留精确矩阵。`, "- 容量溢价和机型代际差异不统一，不能套固定每级加价。", "- price.price、fixed_price、limit_price 是不同字段；目录以 price.price 为抓取参考，不把 fixed_price 或 limit_price 当正常收购价。", "", "## 问卷映射（本项目策略，不是从接口证实的 Swappie 官网映射）", "", "- 外观取屏幕、边框、背面中最差的非损坏等级：none → LIKE_NEW；minimal → ALMOST_NEW；some → GOOD；visible → MODERATE。", "- 屏幕显示异常、裂痕、缺口、严重划痕统一为 BROKEN_SCREEN，只扣一次。", "- 边框损坏或背玻璃损坏统一为 BROKEN_FRAME，只扣一次；损坏部件不重复参与外观降级。", "- Face ID/Touch ID、前摄、顶部扬声器任一异常归为 BROKEN，只扣一次。问卷没有后摄问题，因此不会自动添加 BROKEN_BACK_CAMERA。", "- 电池低于后台阈值时为 BATTERY_ISSUE；unknown 不伪装为电池正常，显示需检测确认。", "- 无法正常开机或连接移动网络继续沿用不收购规则。", "", "## 型号容量样本", "", "| 型号 | 容量 | 如新 | 轻微痕迹 | 一些痕迹 | 明显磨损 | 保底价 | 公式匹配 |", "|---|---|---:|---:|---:|---:|---:|---:|"]
configs.forEach((c) => report.push(`| ${c.model} | ${c.capacity} | ${c.bases.LIKE_NEW} | ${c.bases.ALMOST_NEW} | ${c.bases.GOOD} | ${c.bases.MODERATE} | ${c.floor} | ${c.additive_matches}/160 |`))
fs.writeFileSync(path.join(folder, "analysis.md"), report.join("\n") + "\n")
console.log(JSON.stringify({ models: output.model_count, configurations: configs.length, rows: tested, additive_matches: matched, mismatching_configs: configs.filter((c) => c.additive_matches !== c.total).map((c) => ({ model: c.model, capacity: c.capacity, matches: c.additive_matches })), example: { ...configs[0], matrix: undefined } }, null, 2))
