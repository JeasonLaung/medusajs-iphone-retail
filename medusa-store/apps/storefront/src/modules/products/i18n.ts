const en = {
  cartLimit: "Stock limit reached",
  cartLimitMessage: "Your cart already contains all available stock for this configuration. You can continue to checkout from your cart.",
  inventoryError: "There is not enough stock to add another item. Check your cart or choose another configuration.",
  appleLaunchPrice: "Apple launch price",
  demoPriceNote: "Demo price: this configuration uses the model starting price and has not been individually priced.",
  pricePending: "Price pending", configurationPending: "Some existing stock is awaiting complete configuration details.", imageNote: "Model image shown. The selected colour may differ.",
  back: "All iPhones", configure: "Make it yours.", capacity: "Capacity", condition: "Grade", battery: "Battery", colour: "Colour",
  modelColours: "Model colours", catalogueNote: "Model information only. Available purchase options are shown above.",
  unspecified: "Not specified", from: "From", tax: "Tax calculated at checkout", select: "Choose your options", unavailable: "Unavailable combination", out: "Out of stock", add: "Add to cart", adding: "Adding…", added: "Added to cart", error: "Could not add this item. Please try again.",
  stock: "Available to order", summary: "Your selection", details: "The details. All here.", description: "About this iPhone", specs: "Technical specifications", gallery: "Product images", image: "Product image", previous: "Previous image", next: "Next image", noImage: "Product image coming soon", original: "Original price", sku: "SKU", shipping: "Delivery & support", shippingText: "Available shipping methods and delivery costs are shown at checkout.", help: "Visit our help centre", overview: "At a glance", display: "Display", chip: "Chip", camera: "Rear cameras", frontCamera: "Front camera", release: "Release year", resolution: "Resolution", weight: "Weight", dimensions: "Dimensions", connector: "Connector", authentication: "Authentication", modelNumbers: "Model numbers", appleSpecs: "Apple technical specifications", specNote: "Specifications describe the model. Storage and other purchase options depend on the selected variant.", related: "Explore more iPhones", relatedText: "Find your next favourite.", viewCart: "View cart", selected: "Selected", modelInfo: "Model information", purchaseOptions: "Purchase options", availableStorage: "Model capacities", sourceNote: "Colour, condition and battery details have not been recorded for this SKU.",
}

const zh: typeof en = {
  cartLimit: "已达可购数量",
  cartLimitMessage: "购物车中的此配置已达当前可售数量，可前往购物车结账。",
  inventoryError: "剩余库存不足，无法继续添加。请检查购物车或选择其他配置。",
  appleLaunchPrice: "Apple 首发价",
  demoPriceNote: "演示价格：当前配置沿用机型起售价，尚未单独定价。",
  pricePending: "价格待定", configurationPending: "部分原有库存尚待补齐配置详情，暂不可选。", imageNote: "图片为机型示意，颜色以所选配置为准。",
  back: "全部 iPhone", configure: "选出你的心仪配置。", capacity: "容量", condition: "等级", battery: "电池", colour: "颜色",
  modelColours: "机型配色", catalogueNote: "仅供了解机型，可购买的配置以上方选项为准。",
  unspecified: "暂未提供", from: "起价", tax: "税费将在结账时计算", select: "请选择配置", unavailable: "暂无此配置组合", out: "暂时缺货", add: "加入购物车", adding: "正在添加…", added: "已加入购物车", error: "添加失败，请重试。",
  stock: "可下单", summary: "你的选择", details: "了解每一处细节。", description: "关于这款 iPhone", specs: "技术规格", gallery: "商品图片", image: "商品图片", previous: "上一张图片", next: "下一张图片", noImage: "商品图片即将上线", original: "原价", sku: "SKU", shipping: "配送与支持", shippingText: "可用的配送方式及运费会在结账时显示。", help: "前往帮助中心", overview: "一览亮点", display: "显示屏", chip: "芯片", camera: "后置摄像头", frontCamera: "前置摄像头", release: "发布年份", resolution: "分辨率", weight: "重量", dimensions: "尺寸", connector: "接口", authentication: "身份验证", modelNumbers: "机型编号", appleSpecs: "Apple 技术规格", specNote: "规格描述的是机型本身，容量及其他可购买配置以所选商品变体为准。", related: "探索更多 iPhone", relatedText: "寻找下一款心仪之选。", viewCart: "查看购物车", selected: "已选择", modelInfo: "机型资料", purchaseOptions: "可购买配置", availableStorage: "机型容量", sourceNote: "该 SKU 尚未录入颜色、成色和电池详情。",
}

export const getProductCopy = (locale: string) =>
  locale.toLowerCase().startsWith("zh") || locale.endsWith("-zh") ? zh : en

export const optionLabel = (title: string, locale: string) => {
  const copy = getProductCopy(locale)
  const key = title.toLowerCase()
  if (/storage|capacity/.test(key)) return copy.capacity
  if (/colou?r/.test(key)) return copy.colour
  if (/condition/.test(key)) return copy.condition
  if (/battery/.test(key)) return copy.battery
  return title
}

const colourNames: Record<string, string> = {
  Blue: "蓝色", Green: "绿色", Midnight: "午夜色", Pink: "粉色", Red: "红色", Starlight: "星光色", Purple: "紫色", Yellow: "黄色", Black: "黑色", White: "白色", Silver: "银色", Gold: "金色",
  "Black Titanium": "黑色钛金属", "Blue Titanium": "蓝色钛金属", "Natural Titanium": "原色钛金属", "White Titanium": "白色钛金属", "Desert Titanium": "沙漠色钛金属",
  "Deep Purple": "暗紫色", "Space Black": "深空黑色", "Space Gray": "深空灰色", "Space Grey": "深空灰色", "Sierra Blue": "远峰蓝色", "Alpine Green": "苍岭绿色", Graphite: "石墨色", "Midnight Green": "暗夜绿色", "Rose Gold": "玫瑰金色", "Pacific Blue": "海蓝色", Teal: "深青色", Ultramarine: "群青色", "Cosmic Orange": "星宇橙色", "Deep Blue": "深蓝色", Lavender: "薰衣草紫色", "Sky Blue": "天蓝色", "Light Gold": "浅金色", "Cloud White": "云白色", Sage: "鼠尾草绿色", "Mist Blue": "雾蓝色", "Soft Pink": "浅粉色",
}

export const optionValueLabel = (value: string, locale: string) => {
  const isChinese = locale.startsWith("zh") || locale.endsWith("-zh")
  if (value === "S" || value === "A") return isChinese ? `${value} 级` : `Grade ${value}`
  if (!isChinese) return value
  return colourNames[value] ?? value
}
