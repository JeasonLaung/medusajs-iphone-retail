const en = {
  title: "Contact our team", description: "Choose a pre-sales or after-sales contact. You can reach us through WhatsApp or email.",
  pre_sales: "Pre-sales", after_sales: "After-sales",
  preSalesDescription: "Questions about models, options and purchasing.",
  afterSalesDescription: "Help with orders, deliveries and after-sales requests.",
  empty: "Contact details will be available here once our team has published them.",
  unavailable: "Contact details are temporarily unavailable. Please try again later.",
  hours: "Working hours", scheduleNote: "Scheduled availability follows the working hours shown and does not indicate live presence. You can leave a message outside these hours.",
  inHours: "Within service hours", outOfHours: "Outside service hours", checking: "Checking service hours…",
  online: "Online (set by team)", offline: "Offline (set by team)", email: "Email", whatsapp: "WhatsApp",
  days: ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"],
  whatsappLabel: "Contact {name} on WhatsApp (opens in a new tab)", emailLabel: "Email {name}",
}
const zh: typeof en = {
  title: "联系我们", description: "选择售前或售后客服，通过 WhatsApp 或邮箱与我们联系。",
  pre_sales: "售前咨询", after_sales: "售后服务",
  preSalesDescription: "咨询机型、配置及购买事宜。", afterSalesDescription: "获取订单、配送及售后服务帮助。",
  empty: "客服团队发布联系方式后，将在此展示。", unavailable: "联系方式暂时无法加载，请稍后重试。",
  hours: "工作时间", scheduleNote: "按工作时间显示的服务状态不代表人员实时在线。非工作时间也可留言。",
  inHours: "服务中", outOfHours: "非工作时间", checking: "正在查询服务时间…",
  online: "在线（客服设置）", offline: "离线（客服设置）", email: "邮箱", whatsapp: "WhatsApp",
  days: ["周日", "周一", "周二", "周三", "周四", "周五", "周六"],
  whatsappLabel: "通过 WhatsApp 联系 {name}（在新标签页打开）", emailLabel: "发送邮件给 {name}",
}
export const getServiceCopy = (locale: string | null) => locale?.toLowerCase().startsWith("zh") ? zh : en
