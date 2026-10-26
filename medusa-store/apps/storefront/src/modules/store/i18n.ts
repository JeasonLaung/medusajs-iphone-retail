const en = {
  title: "Used and refurbished iPhones",
  highlightsLabel: "Shopping at a glance",
  rating: "Trustpilot",
  warranty: "Warranty",
  returns: "Returns",
  months: "{count} months",
  oneMonth: "1 month",
  days: "{count} days",
  oneDay: "1 day",
  noWarranty: "No warranty",
  noReturns: "No returns",
  policyUnavailable: "Contact us for details",
}

const zh: typeof en = {
  title: "二手和翻新机",
  highlightsLabel: "选购信息",
  rating: "Trustpilot 评分",
  warranty: "保修",
  returns: "退货",
  months: "{count} 个月",
  oneMonth: "1 个月",
  days: "{count} 天",
  oneDay: "1 天",
  noWarranty: "不提供保修",
  noReturns: "不支持退货",
  policyUnavailable: "详情请咨询客服",
}

export const getStoreCopy = (segment: string) =>
  segment?.endsWith("-zh") ? zh : en
