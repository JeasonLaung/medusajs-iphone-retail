import {PermissionsProvider,RequireAction,useAccess} from './Permissions'
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import { Alert, App as AntApp, Button, ConfigProvider, Descriptions, Drawer, Form, Input, Layout, Menu, Result, Select, Space, Spin, Tag, Typography } from 'antd'
import { MenuOutlined, UserOutlined } from '@ant-design/icons'
import zhCN from 'antd/locale/zh_CN'
import enGB from 'antd/locale/en_GB'
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from 'react-router-dom'
import { APIError, auth } from './api'
import type { User } from './api'
import { copies } from './i18n'
import type { Copy, Language } from './i18n'
const Mailboxes = lazy(() => import('./Mailboxes'))
const Warehouses = lazy(() => import('./Warehouses'))
const Authorization = lazy(() => import('./Authorization'))
const Orders = lazy(() => import('./Orders'))
const OrderDetail = lazy(() => import('./OrderDetail'))
const Products = lazy(() => import('./Products'))
const ProductDetail = lazy(() => import('./ProductDetail'))

type Session = { status: 'loading' } | { status: 'anonymous'; expired?: boolean } | { status: 'authenticated'; user: User } | { status: 'error'; error: unknown }

function ErrorNotice({ error, copy }: { error: unknown; copy: Copy }) {
  const code = error instanceof APIError ? error.code : ''
  let title = copy.unavailable
  if (code === 'auth.invalid_credentials') title = copy.credentials
  else if (code === 'auth.rate_limited') title = copy.limited
  else if (code === 'auth.invalid_origin' || code === 'invalid_origin') title = copy.origin
  else if (code === 'auth.invalid_csrf') title = copy.csrf
  else if (error instanceof APIError && error.status === 401) title = copy.sessionExpired
  return <Alert type="error" showIcon message={title} description={error instanceof APIError && error.requestID ? `${copy.requestID}: ${error.requestID}` : undefined} />
}

function Login({ copy, expired, onLogin }: { copy: Copy; expired?: boolean; onLogin(user: User): void }) {
  const [pending, setPending] = useState(false)
  const pendingRef = useRef(false)
  const [error, setError] = useState<unknown>()
  const [form] = Form.useForm<{ email: string; password: string }>()
  const submit = async (values: { email: string; password: string }) => {
    if (pendingRef.current) return
    pendingRef.current = true; setPending(true); setError(undefined)
    try { onLogin(await auth.login(values.email.trim(), values.password)) }
    catch (error) { setError(error); form.resetFields(['password']) }
    finally { pendingRef.current = false; setPending(false) }
  }
  return <main className="login-main"><section className="login-panel" aria-labelledby="login-title">
    <Typography.Title id="login-title" level={2}>{copy.loginTitle}</Typography.Title>
    <Typography.Paragraph type="secondary">{copy.loginDescription}</Typography.Paragraph>
    {expired && !error && <Alert showIcon type="info" message={copy.sessionExpired} />}
    {error !== undefined && <ErrorNotice error={error} copy={copy} />}
    <Form form={form} layout="vertical" onFinish={submit} requiredMark={false} disabled={pending}>
      <Form.Item name="email" label={copy.email} rules={[{ required: true, message: copy.required }, { type: 'email', message: copy.emailInvalid }]}>
        <Input type="email" autoComplete="username" maxLength={254} autoFocus />
      </Form.Item>
      <Form.Item name="password" label={copy.password} rules={[{ required: true, message: copy.required }]}>
        <Input.Password autoComplete="current-password" />
      </Form.Item>
      <Button type="primary" htmlType="submit" block aria-label={copy.login} disabled={pending} loading={pending}>{copy.login}</Button>
    </Form>
  </section></main>
}

function Workspace(props:{copy:Copy;session:Extract<Session,{status:'authenticated'}>;onSession(value:Session):void}){
 const onExpired=useCallback(()=>props.onSession({status:'anonymous',expired:true}),[props.onSession])
 return <PermissionsProvider onExpired={onExpired}><WorkspaceContent {...props}/></PermissionsProvider>
}

function WorkspaceContent({ copy, session, onSession }: { copy: Copy; session: Extract<Session, { status: 'authenticated' }>; onSession(value: Session): void }) {
  const navigate = useNavigate()
  const location = useLocation()
  const onExpired = useCallback(() => onSession({ status:'anonymous',expired:true }),[onSession])
  const access=useAccess()
  const orderPage = location.pathname === '/orders' || location.pathname.startsWith('/orders/')
  const authorizationPage = location.pathname === '/authorization'
  const mailboxPage = location.pathname === '/mailboxes'
  const warehousePage = location.pathname === '/warehouses'
  const productPage = location.pathname === '/products' || location.pathname.startsWith('/products/')
  const [drawer, setDrawer] = useState(false)
  const [pending, setPending] = useState(false)
  const busy = useRef(false)
  const [error, setError] = useState<unknown>()
  useEffect(() => {
    let live = true
    const check = () => {
      if (document.visibilityState !== 'visible') return
      auth.me().then(user => { if (live) onSession({ status: 'authenticated', user }) }).catch(error => {
        if (!live) return
        if (error instanceof APIError && error.status === 401) onSession({ status: 'anonymous', expired: true })
        else setError(error)
      })
    }
    window.addEventListener('focus', check)
    document.addEventListener('visibilitychange', check)
    return () => { live = false; window.removeEventListener('focus', check); document.removeEventListener('visibilitychange', check) }
  }, [onSession])
  const logout = async () => {
    if (busy.current) return
    busy.current = true; setPending(true); setError(undefined)
    try { await auth.logout(); onSession({ status: 'anonymous' }); navigate('/login', { replace: true }) }
    catch (error) {
      if (error instanceof APIError && error.status === 401) onSession({ status: 'anonymous', expired: true })
      else setError(error)
    } finally { busy.current = false; setPending(false) }
  }
  const menu = <Menu selectedKeys={[orderPage?'orders':productPage?'products':warehousePage?'warehouses':mailboxPage?'mailboxes':authorizationPage?'authorization':'account']} items={[...(access.can('order.read')?[{key:'orders',label:copy.orders}]:[]),...(access.can('product.read')?[{key:'products',label:copy.products}]:[]),...(access.can('warehouse.read')?[{key:'warehouses',label:copy.warehouses}]:[]),...(access.can('mail.read')?[{key:'mailboxes',label:copy.mailboxes}]:[]),...(access.can('authorization.read')?[{key:'authorization',label:copy.authorization}]:[]),{ key: 'account', icon: <UserOutlined />, label: copy.account }]} onClick={({key}) => { navigate(`/${key}`); setDrawer(false) }} />
  return <Layout className="workspace">
    <Layout.Sider className="desktop-nav" width={208} theme="light"><nav aria-label={copy.admin}>{menu}</nav></Layout.Sider>
    <Drawer open={drawer} onClose={() => setDrawer(false)} title={copy.admin} placement="left" width={256}>{menu}</Drawer>
    <Layout.Content className="workspace-content">
      <div className="page-toolbar"><Button className="mobile-nav" icon={<MenuOutlined />} aria-label={copy.navigation} onClick={() => setDrawer(true)} /><Typography.Title level={2}>{orderPage?copy.orders:productPage?copy.products:warehousePage?copy.warehouses:mailboxPage?copy.mailboxes:authorizationPage?copy.authorization:copy.account}</Typography.Title><Button aria-label={copy.logout} disabled={pending} loading={pending} onClick={logout}>{copy.logout}</Button></div>
      {error !== undefined && <div className="request-error"><ErrorNotice error={error} copy={copy} /></div>}
      <Routes>
        <Route path="/warehouses" element={<Suspense fallback={<Spin/>}><RequireAction action="warehouse.read" copy={copy}><Warehouses copy={copy} onExpired={onExpired}/></RequireAction></Suspense>}/>
        <Route path="/mailboxes" element={<Suspense fallback={<Spin/>}><RequireAction action="mail.read" copy={copy}><Mailboxes copy={copy} onExpired={onExpired}/></RequireAction></Suspense>}/>
        <Route path="/authorization" element={<Suspense fallback={<Spin/>}><RequireAction action="authorization.read" copy={copy}><Authorization copy={copy} onExpired={onExpired}/></RequireAction></Suspense>}/>
        <Route path="/orders" element={<Suspense fallback={<Spin />}><RequireAction action="order.read" copy={copy}><Orders copy={copy} onExpired={onExpired} /></RequireAction></Suspense>} />
        <Route path="/orders/:id" element={<Suspense fallback={<Spin />}><RequireAction action="order.read" copy={copy}><OrderDetail copy={copy} onExpired={onExpired} /></RequireAction></Suspense>} />
        <Route path="/products" element={<Suspense fallback={<Spin />}><RequireAction action="product.read" copy={copy}><Products copy={copy} onExpired={onExpired} /></RequireAction></Suspense>} />
        <Route path="/products/:id" element={<Suspense fallback={<Spin />}><RequireAction action="product.read" copy={copy}><ProductDetail copy={copy} onExpired={onExpired} /></RequireAction></Suspense>} />
        <Route path="/account" element={<section className="account-panel"><Space direction="vertical" size={24} className="full-width">
          <Space><Tag color="success">{copy.signedIn}</Tag><Typography.Text type="secondary">{copy.accountDescription}</Typography.Text></Space>
          <Descriptions column={1} items={[{ key: 'name', label: copy.name, children: session.user.name || '—' }, { key: 'email', label: copy.email, children: session.user.email }, { key: 'role', label: copy.role, children: copy.administrator }]} />
        </Space></section>} />
        <Route path="/" element={<Navigate to="/account" replace />} />
        <Route path="/login" element={<Navigate to="/account" replace />} />
        <Route path="*" element={<Result status="404" title="404" subTitle={copy.notFound} extra={<Button onClick={() => navigate('/account')}>{copy.returnAccount}</Button>} />} />
      </Routes>
    </Layout.Content>
  </Layout>
}

export default function AdminApp() {
  const [language, setLanguage] = useState<Language>(() => {
    try { return localStorage.getItem('eurorefurb.admin.language') === 'en' ? 'en' : 'zh-CN' } catch { return 'zh-CN' }
  })
  const [session, setSession] = useState<Session>({ status: 'loading' })
  const [attempt, setAttempt] = useState(0)
  const copy = copies[language]
  useEffect(() => {
    document.documentElement.lang = language
    document.title = `Eurorefurb · ${copy.admin}`
    try { localStorage.setItem('eurorefurb.admin.language', language) } catch { /* Preference persistence is optional. */ }
  }, [language, copy.admin])
  useEffect(() => {
    const controller = new AbortController()
    setSession({ status: 'loading' })
    auth.me(controller.signal).then(user => { if (!controller.signal.aborted) setSession({ status: 'authenticated', user }) }).catch(error => {
      if (controller.signal.aborted) return
      setSession(error instanceof APIError && error.status === 401 ? { status: 'anonymous' } : { status: 'error', error })
    })
    return () => controller.abort()
  }, [attempt])
  return <ConfigProvider locale={language === 'en' ? enGB : zhCN} theme={{ token: {
    colorPrimary: '#1677FF', colorInfo: '#1677FF', colorSuccess: '#52C41A', colorWarning: '#FA8C16', colorError: '#F5222D',
    colorBgLayout: '#F8F8F8', colorBgContainer: '#FFFFFF', colorText: '#000000E0', colorTextSecondary: '#000000A6',
    colorBorder: '#D9D9D9', colorSplit: '#0505050F', borderRadius: 6, fontFamily: 'system-ui, -apple-system, "Segoe UI", "Microsoft YaHei", sans-serif',
  } }}><AntApp><BrowserRouter><Layout className="app-layout">
    <Layout.Header className="app-header"><Space size={12}><Typography.Text strong className="brand">EUROREFURB</Typography.Text><Typography.Text type="secondary">{copy.admin}</Typography.Text></Space>
      <Select aria-label={copy.language} virtual={false} value={language} options={[{ value: 'zh-CN', label: '简体中文' }, { value: 'en', label: 'English' }]} onChange={setLanguage} popupMatchSelectWidth={128} />
    </Layout.Header>
    {session.status === 'loading' && <main className="center-state" aria-label={copy.checking}><Spin size="large" /><Typography.Paragraph>{copy.checking}</Typography.Paragraph></main>}
    {session.status === 'error' && <main className="center-state"><ErrorNotice error={session.error} copy={copy} /><Button type="primary" onClick={() => setAttempt(n => n + 1)}>{copy.retry}</Button></main>}
    {session.status === 'anonymous' && <Routes><Route path="/login" element={<Login copy={copy} expired={session.expired} onLogin={user => setSession({ status: 'authenticated', user })} />} /><Route path="*" element={<Navigate to="/login" replace />} /></Routes>}
    {session.status === 'authenticated' && <Workspace copy={copy} session={session} onSession={setSession} />}
  </Layout></BrowserRouter></AntApp></ConfigProvider>
}
