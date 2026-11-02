// Original catalog assets copied byte-for-byte into Astro public/swappie.
const catalogPaths = new Set([
 '/swappie/swappie-iphone-11-pro-gold.png',
 '/swappie/swappie-iphone-11-purple.png',
 '/swappie/swappie-iphone-12-black.png',
 '/swappie/swappie-iphone-12-mini-red.png',
 '/swappie/swappie-iphone-12-pro-graphite.png',
 '/swappie/swappie-iphone-12-pro-max-graphite.png',
 '/swappie/swappie-iphone-13-midnight.png',
 '/swappie/swappie-iphone-13-mini-green.png',
 '/swappie/swappie-iphone-13-pro-gold.png',
 '/swappie/swappie-iphone-13-pro-max-sierra-blue.png',
 '/swappie/swappie-iphone-14-midnight.png',
 '/swappie/swappie-iphone-14-plus-midnight.png',
 '/swappie/swappie-iphone-14-pro-gold.png',
 '/swappie/swappie-iphone-14-pro-max-deep-purple.png',
 '/swappie/swappie-iphone-15-black.png',
 '/swappie/swappie-iphone-15-plus-blue.png',
 '/swappie/swappie-iphone-15-pro-max-natural-titanium.png',
 '/swappie/swappie-iphone-15-pro-natural-titanium.png',
 '/swappie/swappie-iphone-16-black.png',
 '/swappie/swappie-iphone-16-plus-ultramarine.png',
 '/swappie/swappie-iphone-16-pro-black-titanium.png',
 '/swappie/swappie-iphone-16-pro-max-desert-titanium.png',
 '/swappie/swappie-iphone-16e-black.png',
 '/swappie/swappie-iphone-17-black.png',
 '/swappie/swappie-iphone-17-pro-deep-blue.png',
 '/swappie/swappie-iphone-17-pro-max-deep-blue.png',
 '/swappie/swappie-iphone-17e-soft-pink.png',
 '/swappie/swappie-iphone-air-space-black.png',
 '/swappie/swappie-iphone-se-2020-black.png',
 '/swappie/swappie-iphone-se-2022-midnight.png',
])

export function publicImageURL(value: string): string {
 try {
  const url = new URL(value)
  if (url.protocol === 'http:' && ['127.0.0.1', 'localhost'].includes(url.hostname) && url.port === '8000' && !url.username && !url.password && !url.search && !url.hash && catalogPaths.has(url.pathname)) return url.pathname
 } catch {}
 return value
}
