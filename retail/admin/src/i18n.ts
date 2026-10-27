export type Language = 'zh-CN' | 'en'
const zh = {
  imageUnavailable:'图片无法加载',
  unassignedOptions:'以下选项值不属于当前商品的有效配置',
  backProducts:'返回商品列表',loadingProduct:'正在加载商品',productNotFound:'商品不存在或已删除',subtitle:'副标题',images:'商品图片',noImages:'暂无图片',options:'选项',noOptions:'暂无选项',variantDetails:'商品变体',noVariants:'暂无变体',configuration:'配置',complete:'配置完整',incomplete:'配置待补全',manageInventory:'管理库存',allowBackorder:'允许缺货订购',yes:'是',no:'否',
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
  imageUnavailable:'Image unavailable',
  unassignedOptions:'These option values are not assigned to this product',
  backProducts:'Back to products',loadingProduct:'Loading product',productNotFound:'Product not found or deleted',subtitle:'Subtitle',images:'Product images',noImages:'No images',options:'Options',noOptions:'No options',variantDetails:'Product variants',noVariants:'No variants',configuration:'Configuration',complete:'Complete',incomplete:'Incomplete',manageInventory:'Manage inventory',allowBackorder:'Allow backorders',yes:'Yes',no:'No',
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
