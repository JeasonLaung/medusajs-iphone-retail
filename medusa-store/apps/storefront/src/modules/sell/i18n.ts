const en = {
  title: "Your iPhone. A new chapter.",
  intro: "Give your old iPhone a new story. Get started with a few simple questions about its model, battery and condition.",
  start: "Sell your iPhone",
  imageAlt: "Hands holding three iPhones in blue, black and white",
  processTitle: "How selling your iPhone works",
  processIntro: "From telling us about your phone to getting paid, here’s the process step by step.",
  stepLabel: "Step {number}",
  processSteps: [
    { title: "Tell us about your iPhone", description: "Choose your model and storage, then answer a few questions about the battery, functions and appearance." },
    { title: "Prepare and send your phone", description: "Back up your data, turn off Find My and erase your iPhone. Pack it carefully and follow the shipping instructions." },
    { title: "Inspection and final offer", description: "Your phone is inspected to check its condition against your answers. Review the final offer before deciding whether to accept." },
    { title: "Accept and get paid", description: "Once you accept the final offer, payment is arranged using the payment details you provide." },
  ],
  onePhone: "One iPhone. A few simple steps.",
  onePhoneDescription: "Choose your model, check the storage and battery, then tell us how your phone looks and works. Selling more than one? Fill in the details for each phone separately.",
  findModel: "Not sure which model you have?",
  findModelDescription: "On your iPhone, open Settings > General > About and look for Model Name.",
  modelTitle: "iPhone model",
  unavailable: "Models could not be loaded. Please try again.",
  retry: "Try again",
  back: "Back",
  availability: "Device details can be filled in now. Valuations, shipping, inspection and payment are not available yet.",
}

const zh: typeof en = {
  title: "让你的 iPhone，开启下一程。",
  intro: "给闲置的 iPhone 一个新开始。从型号、电池到成色，只需回答几个简单的问题，即可开始。",
  start: "出售你的 iPhone",
  imageAlt: "双手拿着蓝色、黑色和白色的三部 iPhone",
  processTitle: "如何出售你的 iPhone",
  processIntro: "从填写手机信息到收款，整个回收流程分为以下几步。",
  stepLabel: "第 {number} 步",
  processSteps: [
    { title: "填写手机信息", description: "选择型号与容量，再回答几个关于电池健康、功能和外观的问题。" },
    { title: "准备并寄出手机", description: "备份个人数据，关闭「查找」并抹除手机内容，妥善包装后按寄送说明寄出。" },
    { title: "检测并确认报价", description: "收到手机后核对实际状况与填写信息，你可以查看最终报价，再决定是否接受。" },
    { title: "接受报价并收款", description: "接受最终报价后，将按你提供的收款信息安排付款。" },
  ],
  onePhone: "一部 iPhone，几个简单步骤。",
  onePhoneDescription: "选择型号，确认容量与电池健康，再告诉我们手机的外观和使用状况。有多部手机需要出售？请为每部手机分别填写信息。",
  findModel: "不确定自己的手机型号？",
  findModelDescription: "在 iPhone 上打开「设置 > 通用 > 关于本机」，查看「型号名称」。",
  modelTitle: "iPhone 型号",
  unavailable: "暂时无法加载型号，请重试。",
  retry: "重试",
  back: "返回",
  availability: "目前可填写手机信息，估价、寄送、检测与收款服务暂未开放。",
}

export const getSellCopy = (locale: string | null) =>
  locale?.toLowerCase().startsWith("zh") ? zh : en

