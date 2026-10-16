const en = {
  navigation: "Main navigation", buy: "Buy", sell: "Sell", about: "About us",
  reviews: "Reviews", help: "Help", account: "Account", cart: "Cart",
  menu: "Menu", close: "Close menu", preferences: "Country and language",
  country: "Shipping country", language: "Language", save: "Apply", error: "Unable to save your preferences. Please try again.",
  selectRegion: "Select region and language", closeRegion: "Close region and language selection", noRegions: "No shipping regions are available.",
  focus: "Our focus", shopping: "Shopping", orders: "Account & orders",
  store: "Browse iPhones", back: "Back to the store", upcoming: "Coming soon",
  sellTitle: "Sell your iPhone.",
  sellDescription: "Our iPhone buyback service is being prepared. Online valuations and submissions are not available yet.",
  aboutTitle: "A store for iPhone.",
  aboutDescription: "Explore our iPhone collection by series and find the model that suits you.",
  focusDescription: "Our catalogue focuses on iPhone. Browse model details and available options on each product page.",
  reviewsTitle: "Customer reviews.",
  reviewsDescription: "There are no published customer reviews yet. This page will display reviews when they are available.",
  helpTitle: "How can we help?",
  helpDescription: "Start with the collection, or access your account to view your orders.",
  shoppingDescription: "Browse iPhones by series, open a product to see its details and options, and review your selection in the cart before checkout.",
  ordersDescription: "Sign in to your account to view your orders and manage your profile and addresses.",
}

const zh: typeof en = {
  navigation: "主导航", buy: "购买", sell: "出售", about: "关于我们",
  reviews: "用户评价", help: "帮助", account: "账户", cart: "购物车",
  menu: "菜单", close: "关闭菜单", preferences: "国家与语言",
  country: "配送国家", language: "语言", save: "应用", error: "偏好设置保存失败，请重试。",
  selectRegion: "选择地区与语言", closeRegion: "关闭地区与语言选择", noRegions: "暂无可用配送地区。",
  focus: "我们的专注", shopping: "选购指南", orders: "账户与订单",
  store: "浏览 iPhone", back: "返回商店", upcoming: "即将上线",
  sellTitle: "出售你的 iPhone。",
  sellDescription: "我们的 iPhone 回收服务正在筹备中，线上估价与回收申请暂未开放。",
  aboutTitle: "专注于 iPhone 的商店。",
  aboutDescription: "按系列探索我们的 iPhone 商品，找到适合你的机型。",
  focusDescription: "我们的商品目录专注于 iPhone。你可以在商品页面查看机型详情和可选配置。",
  reviewsTitle: "用户评价。",
  reviewsDescription: "目前暂无已发布的用户评价。有可展示的评价后，我们会在这里更新。",
  helpTitle: "需要什么帮助？",
  helpDescription: "从商品目录开始选购，或登录账户查看你的订单。",
  shoppingDescription: "按系列浏览 iPhone，进入商品页查看详情和可选配置，在结账前通过购物车确认所选商品。",
  ordersDescription: "登录账户即可查看订单，并管理个人资料与地址。",
}

export const getLayoutCopy = (locale: string | null) =>
  locale?.toLowerCase().startsWith("zh") ? zh : en
