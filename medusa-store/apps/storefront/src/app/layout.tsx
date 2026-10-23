import { getBaseURL } from "@lib/util/env"
import { Metadata } from "next"
import { AntdRegistry } from "@ant-design/nextjs-registry"
import "styles/globals.css"
import { withSEO } from "@lib/data/seo"
import SEOData from "@modules/common/components/seo-data"

const defaultMetadata: Metadata = {
  metadataBase: new URL(getBaseURL()),
  title: { default: "EuroRefurb", template: "%s | EuroRefurb" },
  applicationName: "EuroRefurb",
  icons: { icon: "/favicon.ico", apple: "/apple-touch-icon.png" },
}
export async function generateMetadata(): Promise<Metadata> { return withSEO(defaultMetadata) }

export default function RootLayout(props: { children: React.ReactNode }) {
  return (
    <html lang="en" data-mode="light">
      <body>
        <SEOData />
        <AntdRegistry>
          <main className="relative">{props.children}</main>
        </AntdRegistry>
      </body>
    </html>
  )
}
