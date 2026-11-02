import {publicImageURL} from '../../storefront/src/lib/public-image-url'

// Vite packages the same public catalog bytes for both development and release.
// Keep persisted URLs untouched: they are also part of image command receipts.
const assets = import.meta.glob<string>('../../storefront/public/swappie/*.png', {
 eager: true, query: '?url', import: 'default',
})

export function catalogImageURL(value: string): string {
 const path = publicImageURL(value)
 if (path.startsWith('/swappie/') && !path.includes('?') && !path.includes('#')) {
  return assets['../../storefront/public' + path] ?? value
 }
 return value
}
