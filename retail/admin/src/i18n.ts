export type Language = 'zh-CN' | 'en'
const zh = {
  products:'商品',search:'搜索',status:'状态',sort:'排序',reset:'重置',draft:'草稿',proposed:'待审核',published:'已发布',rejected:'已拒绝',newest:'创建时间从新到旧',oldest:'创建时间从旧到新',titleAsc:'标题升序',titleDesc:'标题降序',invalidFilters:'筛选参数无效，请重置后重试。',noProducts:'没有符合条件的商品',productTitle:'商品名称',variants:'变体数量',createdAt:'创建时间',
  admin: '管理后台', login: '登录', email: '邮箱', password: '密码', required: '请填写此项', emailInvalid: '请输入有效邮箱',
  loginTitle: '登录管理后台', loginDescription: '使用新系统的员工账号登录。', account: '我的账号', logout: '退出登录',
  retry: '重新连接', checking: '正在确认登录状态', unavailable: '暂时无法连接服务，请稍后重试。',
  credentials: '邮箱或密码不正确。', csrf: '安全校验已失效，请重新登录。', limited: '尝试次数过多，请稍后再试。',
  origin: '当前访问地址未获授权，请从正确的后台地址打开。', requestID: '请求编号',
  sessionExpired: '登录已过期，请重新登录。', notFound: '页面不存在', returnAccount: '返回我的账号',
  name: '姓名', role: '角色', administrator: '管理员', signedIn: '已登录',
  accountDescription: '当前登录账号的信息。', logoutFailure: '退出未完成，请重试；当前页面仍保留登录状态。',
  appError: '页面暂时无法显示，请重新加载。', reload: '重新加载', language: '语言', navigation: '打开导航',
}
export type Copy = typeof zh
const en: Copy = {
  products:'Products',search:'Search',status:'Status',sort:'Sort',reset:'Reset',draft:'Draft',proposed:'Proposed',published:'Published',rejected:'Rejected',newest:'Newest first',oldest:'Oldest first',titleAsc:'Title ascending',titleDesc:'Title descending',invalidFilters:'Invalid filters. Reset and try again.',noProducts:'No matching products',productTitle:'Product',variants:'Variants',createdAt:'Created',
  admin: 'Administration', login: 'Sign in', email: 'Email', password: 'Password', required: 'This field is required', emailInvalid: 'Enter a valid email',
  loginTitle: 'Sign in to administration', loginDescription: 'Use your staff account for the new system.', account: 'My account', logout: 'Sign out',
  retry: 'Reconnect', checking: 'Checking your session', unavailable: 'Unable to connect. Please try again later.',
  credentials: 'The email or password is incorrect.', csrf: 'Security verification expired. Please sign in again.', limited: 'Too many attempts. Please try again later.',
  origin: 'This address is not authorized. Open the configured admin address.', requestID: 'Request ID',
  sessionExpired: 'Your session expired. Please sign in again.', notFound: 'Page not found', returnAccount: 'Return to my account',
  name: 'Name', role: 'Role', administrator: 'Administrator', signedIn: 'Signed in',
  accountDescription: 'Details of the account currently signed in.', logoutFailure: 'Sign out did not complete. Please retry; this page still shows your session.',
  appError: 'This page could not be displayed. Please reload.', reload: 'Reload', language: 'Language', navigation: 'Open navigation',
}
export const copies: Record<Language, Copy> = { 'zh-CN': zh, en }
