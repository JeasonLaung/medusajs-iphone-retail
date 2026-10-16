const en = {
  updating: "Updating cart…",
  cart: "Cart", items: "item(s)", quantity: "Quantity", remove: "Remove", removing: "Removing…", available: "Available to order", stockChanged: "This quantity is no longer available. Please update your cart.", stockUnknown: "Availability will be checked at checkout.",
  updateError: "Could not update the quantity. Please try again.", removeError: "Could not remove this item. Please try again.", condition: "Condition", battery: "Battery", colour: "Colour", capacity: "Capacity",
  summary: "Order summary", subtotal: "Subtotal", shipping: "Shipping fee", shippingPending: "Calculated at checkout", taxes: "Taxes", discount: "Discount", total: "Total", checkout: "Proceed to checkout", continue: "Continue shopping", checkoutHelp: "Review delivery and payment options at checkout.",
  empty: "Your cart is empty", emptyHelp: "Find an iPhone and choose the configuration that suits you.", signIn: "Sign in", signInHelp: "Already have an account?", demo: "Demo price", unavailable: "Unavailable",
  chosenTitle: "Your chosen configuration", chosenText: "Review the capacity, colour, condition and battery before ordering.",
  totalsTitle: "Clear order totals", totalsText: "See your items and discounts here. Delivery costs are confirmed at checkout.",
  checkoutTitle: "Checkout in three steps", checkoutText: "Add your details, choose delivery and select an available payment method.",
}
const zh: typeof en = {
  updating: "正在更新购物车…",
  cart: "购物车", items: "件商品", quantity: "数量", remove: "移除", removing: "正在移除…", available: "可下单", stockChanged: "当前库存不足以满足此数量，请调整购物车。", stockUnknown: "库存将在结账时核实。",
  updateError: "数量更新失败，请重试。", removeError: "商品移除失败，请重试。", condition: "成色", battery: "电池", colour: "颜色", capacity: "容量",
  summary: "订单摘要", subtotal: "商品小计", shipping: "运费", shippingPending: "结账时计算", taxes: "税费", discount: "优惠", total: "合计", checkout: "前往结账", continue: "继续选购", checkoutHelp: "在结账时确认配送和支付方式。",
  empty: "购物车还是空的", emptyHelp: "挑选一款 iPhone，选择适合你的配置。", signIn: "登录", signInHelp: "已有账户？", demo: "演示价格", unavailable: "暂不可用",
  chosenTitle: "确认心仪配置", chosenText: "下单前核对容量、颜色、成色和电池配置。",
  totalsTitle: "清晰的订单金额", totalsText: "商品和优惠在此显示，配送费用将在结账时确认。",
  checkoutTitle: "三步完成结账", checkoutText: "填写信息、选择配送，再选择可用的支付方式。",
}
export const getCartCopy = (locale: string) => locale.startsWith("zh") || locale.endsWith("-zh") ? zh : en
