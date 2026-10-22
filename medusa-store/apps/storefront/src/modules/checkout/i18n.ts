const en = {
  paymentResultTitle: "Mollie payment result",
  paymentCheckingTitle: "Checking your payment",
  paymentCheckingBody: "We are verifying your payment and preparing your order. Please keep this page open and do not pay again.",
  paymentWaitingTitle: "Waiting for confirmation",
  paymentWaitingBody: "Your payment or order is still being confirmed. This page will update automatically. Please do not pay again.",
  paymentFailedTitle: "Payment was not completed",
  paymentFailedBody: "This payment was canceled, expired or failed. You can return to checkout to try again.",
  paymentInvalidTitle: "We could not match your payment",
  paymentInvalidBody: "Your checkout session may have expired or changed. If you have already paid, contact support before trying again.",
  paymentCheckErrorTitle: "Unable to check your payment",
  paymentCheckErrorBody: "Confirmation is taking longer than expected. Please contact support if you have already paid. Do not start another payment.",
  paymentSuccessTitle: "Payment confirmed",
  paymentSuccessBody: "Your order is ready. Opening your order confirmation…",
  paymentReturnCheckout: "Return to checkout", paymentViewOrder: "View order", paymentContactSupport: "Contact support",
  paymentNeedsReview: "Your payment needs review. Please contact support before trying again.",
  paypalMollie: "PayPal via Mollie", giftCardMollie: "Gift card via Mollie",
  testingOnly: "For testing purposes only.",
  promoInvalid: "This discount code is invalid or does not apply to your order.",
  checkout: "Checkout", back: "Back to cart", details: "Details", delivery: "Delivery", payment: "Payment",
  contact: "Contact details", shipping: "Shipping details", billing: "Billing address", firstName: "First name", lastName: "Last name",
  email: "Email address", confirmEmail: "Confirm email address", emailMismatch: "The email addresses do not match.", phone: "Phone",
  phoneHelp: "Please provide a phone number so we can contact you about your order.", address: "Street address", addressExtra: "Apartment, building, company, etc. (optional)",
  postcode: "Postcode", city: "Town / City", country: "Country", province: "State / Province (optional)", company: "Company (optional)",
  sameBilling: "Billing address is the same as shipping address", savedAddress: "Use a saved address", continueDelivery: "Continue to delivery", continuePayment: "Continue to payment",
  deliveryHelp: "Choose an available delivery method for your address.", homeDelivery: "Home delivery", pickup: "Store pickup", noDelivery: "No delivery methods are available for this address. Edit your details or contact us.",
  deliveryError: "Delivery options could not be loaded. Please refresh checkout.", pricePending: "Price unavailable", selectDelivery: "Please choose a delivery method.",
  paymentMethods: "Payment methods", payNow: "Pay now", paymentMethod: "Payment method", paymentDetails: "Payment details", paymentUnavailable: "Currently unavailable. Please choose another payment method.",
  noPayment: "No payment methods are currently available. Please contact us before placing an order.", paymentSetupError: "Could not set up payment. Please try again.", enterPayment: "Enter payment details", reviewPayment: "Review and pay", review: "Review your order",
  paymentFailed: "Payment was not completed. Please try again or choose another payment method.", paymentPending: "Your payment is still being confirmed. Please do not pay again.", paymentInvalid: "We could not match this payment to your cart. Please reopen checkout.",
  paymentConfirmed: "Payment details confirmed", mollieHelp: "Continue to Mollie to pay securely.", paypalHelp: "Pay securely with PayPal below.", giftCard: "Gift card",
  card: "Debit / credit card", manualPayment: "Manual payment (test)", cardPaypal: "Debit / credit card (PayPal)", cardMollie: "Debit / credit card (Mollie)",
  edit: "Edit", secureCheckout: "Secure checkout", secureHelp: "Choose your payment method in the final step. Your order total is shown below.",
  total: "Total", products: "Items", summary: "Order summary", quantity: "Quantity", subtotal: "Subtotal (excluding tax)", tax: "Taxes", discount: "Discount", shippingCost: "Shipping", notSelected: "Not selected yet",
  promotion: "Discount code", promotionPlaceholder: "Enter your code", apply: "Apply", removePromotion: "Remove discount code", promoError: "Could not update the discount code. Please try again.",
  terms: "By placing your order, you confirm that you have read and accept our Terms of Use, Terms of Sale and Returns Policy, and acknowledge our Privacy Policy.",
  placeOrder: "Place order", selectPayment: "Select a payment method", completeDelivery: "Complete your delivery details", paymentError: "Could not start your payment. Please try again.", paymentLoadError: "Could not load the payment methods.",
}

const zh: typeof en = {
  paymentResultTitle: "Mollie 支付结果",
  paymentCheckingTitle: "正在核验付款",
  paymentCheckingBody: "我们正在核验付款并处理订单，请保留此页面，暂勿重复付款。",
  paymentWaitingTitle: "等待确认结果",
  paymentWaitingBody: "付款或订单仍在确认中，页面会自动更新，请勿重复付款。",
  paymentFailedTitle: "支付未完成",
  paymentFailedBody: "此次支付已取消、过期或失败，您可以返回结账页面重试。",
  paymentInvalidTitle: "无法匹配付款信息",
  paymentInvalidBody: "结账会话可能已过期或发生变化。如果您已付款，请先联系客服，暂勿重复付款。",
  paymentCheckErrorTitle: "暂时无法核验付款",
  paymentCheckErrorBody: "确认时间超出预期。如果您已经付款，请联系客服，暂勿重复付款。",
  paymentSuccessTitle: "付款已确认",
  paymentSuccessBody: "订单已生成，正在打开订单确认页…",
  paymentReturnCheckout: "返回结账", paymentViewOrder: "查看订单", paymentContactSupport: "联系客服",
  paymentNeedsReview: "您的付款需要核实，请先联系客服，暂勿重复付款。",
  paypalMollie: "PayPal（通过 Mollie）", giftCardMollie: "礼品卡（通过 Mollie）",
  testingOnly: "仅供测试使用。",
  promoInvalid: "此优惠码无效或不适用于当前订单。",
  checkout: "结账", back: "返回购物车", details: "联系信息", delivery: "配送", payment: "支付",
  contact: "联系信息", shipping: "配送信息", billing: "账单地址", firstName: "名", lastName: "姓",
  email: "电子邮箱", confirmEmail: "确认电子邮箱", emailMismatch: "两次填写的邮箱不一致。", phone: "联系电话",
  phoneHelp: "请提供联系电话，以便我们就订单与您联系。", address: "街道地址", addressExtra: "公寓、楼栋、公司等（选填）",
  postcode: "邮政编码", city: "城市", country: "国家或地区", province: "州 / 省（选填）", company: "公司（选填）",
  sameBilling: "账单地址与收货地址相同", savedAddress: "使用已保存的地址", continueDelivery: "继续选择配送", continuePayment: "继续支付",
  deliveryHelp: "请选择适用于收货地址的配送方式。", homeDelivery: "送货上门", pickup: "门店自提", noDelivery: "此地址暂无可用的配送方式，请修改地址或联系我们。",
  deliveryError: "配送方式加载失败，请刷新结账页面。", pricePending: "价格暂不可用", selectDelivery: "请选择配送方式。",
  paymentMethods: "支付方式", payNow: "立即付款", paymentMethod: "支付方式", paymentDetails: "支付信息", paymentUnavailable: "暂不可用，请选择其他支付方式。",
  noPayment: "暂无可用的支付方式，请在下单前联系我们。", paymentSetupError: "支付初始化失败，请重试。", enterPayment: "填写支付信息", reviewPayment: "确认并支付", review: "确认订单",
  paymentFailed: "支付未完成，请重试或选择其他支付方式。", paymentPending: "支付结果仍在确认中，请勿重复付款。", paymentInvalid: "无法将此支付匹配到当前购物车，请重新打开结账页面。",
  paymentConfirmed: "支付信息已确认", mollieHelp: "继续前往 Mollie 安全支付。", paypalHelp: "在下方使用 PayPal 安全支付。", giftCard: "礼品卡",
  card: "借记卡 / 信用卡", manualPayment: "手动支付（测试）", cardPaypal: "借记卡 / 信用卡（PayPal）", cardMollie: "借记卡 / 信用卡（Mollie）",
  edit: "修改", secureCheckout: "安全结账", secureHelp: "在最后一步选择支付方式，订单总额如下所示。",
  total: "合计", products: "商品件数", summary: "订单摘要", quantity: "数量", subtotal: "商品小计（不含税）", tax: "税费", discount: "优惠", shippingCost: "运费", notSelected: "尚未选择",
  promotion: "优惠码", promotionPlaceholder: "输入优惠码", apply: "应用", removePromotion: "移除优惠码", promoError: "优惠码更新失败，请重试。",
  terms: "提交订单即表示您已阅读并接受我们的使用条款、销售条款和退货政策，并已了解隐私政策。",
  placeOrder: "提交订单", selectPayment: "请选择支付方式", completeDelivery: "请先完善配送信息", paymentError: "无法发起支付，请重试。", paymentLoadError: "支付方式加载失败。",
}

export const getCheckoutCopy = (locale: string) => locale.startsWith("zh") || locale.endsWith("-zh") ? zh : en
export function paymentTitle(id: string, locale: string, fallback?: string) {
  const copy = getCheckoutCopy(locale)
  if (id === "pp_system_default") return copy.manualPayment
  if (id === "pp_paypal_card_paypal_card") return copy.cardPaypal
  if (id === "pp_mollie-card_mollie") return copy.cardMollie
  if (id === "pp_mollie-paypal_mollie") return copy.paypalMollie
  if (id === "pp_mollie-giftcard_mollie") return copy.giftCardMollie
  if (id === "pp_stripe_stripe" || id === "pp_medusa-payments_default") return copy.card
  return fallback ?? id
}
