declare module 'footer-oracle:footer' {
  import type {ComponentType} from 'react'
  const Footer: ComponentType<{
    data: import('../../../src/lib/footer-data').FooterData
    segment: string
    year?: number
  }>
  export default Footer
}
