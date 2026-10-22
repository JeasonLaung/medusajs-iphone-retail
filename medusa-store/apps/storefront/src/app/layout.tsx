import { getBaseURL } from "@lib/util/env"
import { Metadata } from "next"
import { AntdRegistry } from "@ant-design/nextjs-registry"
import "styles/globals.css"

export const metadata: Metadata = {
  metadataBase: new URL(getBaseURL()),
  title: { default: "EuroRefurb", template: "%s | EuroRefurb" },
  applicationName: "EuroRefurb",
  icons: { icon: "/favicon.ico", apple: "/apple-touch-icon.png" },
}

export default function RootLayout(props: { children: React.ReactNode }) {
  return (
    <html lang="en" data-mode="light">
      <body>
        <AntdRegistry>
          <main className="relative">{props.children}</main>
        </AntdRegistry>
      </body>
    </html>
  )
}
