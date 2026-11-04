const en = {
  title: "Customer reviews on Trustpilot",
  description:
    "Independent reviews of our shopping experience, delivery and service.",
  link: "Read our reviews on Trustpilot",
}
const zh: typeof en = {
  title: "Trustpilot 用户评价",
  description: "来自 Trustpilot 的购物体验、配送及服务评价。",
  link: "在 Trustpilot 查看我们的评价",
}
export const getTrustpilotCopy = (locale: string | null) =>
  locale?.startsWith("zh") || locale?.endsWith("-zh") ? zh : en
