import '@ant-design/v5-patch-for-react-19'
import { Component, StrictMode } from 'react'
import type { ReactNode } from 'react'
import { createRoot } from 'react-dom/client'
import { Button, Result } from 'antd'
import AdminApp from './App'
import 'antd/dist/reset.css'
import './style.css'

class ErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false }
  static getDerivedStateFromError() { return { failed: true } }
  render() {
    return this.state.failed ? <Result status="error" title={document.documentElement.lang === 'en' ? 'Unable to display this page' : '页面暂时无法显示'} extra={<Button onClick={() => location.reload()}>{document.documentElement.lang === 'en' ? 'Reload' : '重新加载'}</Button>} /> : this.props.children
  }
}

createRoot(document.getElementById('root')!).render(<StrictMode><ErrorBoundary><AdminApp /></ErrorBoundary></StrictMode>)
