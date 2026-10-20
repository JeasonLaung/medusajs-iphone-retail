// Local storage must resolve through the current host, including phone HTTPS links.
export function devicePhotoUrl(url: string) {
  const parsed = new URL(url)
  return ["localhost", "127.0.0.1", "[::1]"].includes(parsed.hostname) && parsed.pathname.startsWith("/static/")
    ? parsed.pathname : url
}
