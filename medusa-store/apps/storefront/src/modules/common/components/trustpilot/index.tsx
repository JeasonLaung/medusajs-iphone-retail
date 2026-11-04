"use client"

import Script from "next/script"
import { useEffect, useRef, useState } from "react"
import type { TrustpilotSettings } from "types/trustpilot"
import { getTrustpilotCopy } from "./i18n"

type TrustpilotWindow = Window & {
  Trustpilot?: {
    loadFromElement: (element: HTMLElement, forceReload: boolean) => void
  }
}

export default function TrustpilotWidget({
  settings,
  locale,
  variant = "reviews",
}: {
  settings: TrustpilotSettings
  locale: string | null
  variant?: "summary" | "reviews"
}) {
  const element = useRef<HTMLDivElement>(null)
  const initialized = useRef<HTMLElement | null>(null)
  const [ready, setReady] = useState(false)
  const language =
    locale?.startsWith("zh") || locale?.endsWith("-zh") ? "zh-CN" : "en-GB"
  const template =
    variant === "summary"
      ? settings.summary_template_id
      : settings.reviews_template_id
  const height =
    variant === "summary" ? settings.summary_height : settings.reviews_height
  const identity = [
    settings.business_unit_id,
    settings.profile_url,
    template,
    height,
    language,
  ].join(":")
  useEffect(() => {
    const api = (window as TrustpilotWindow).Trustpilot
    if (element.current && api && initialized.current !== element.current) {
      api.loadFromElement(element.current, true)
      initialized.current = element.current
    }
  }, [ready, identity])
  if (!settings.enabled) return null
  return (
    <div className="w-full min-w-0" data-testid={`trustpilot-${variant}`}>
      <Script
        id="trustpilot-bootstrap"
        src="https://widget.trustpilot.com/bootstrap/v5/tp.widget.bootstrap.min.js"
        strategy="afterInteractive"
        onReady={() => setReady(true)}
      />
      <div
        key={identity}
        ref={element}
        className="trustpilot-widget"
        data-locale={language}
        data-template-id={template}
        data-businessunit-id={settings.business_unit_id}
        data-style-height={`${height}px`}
        data-style-width="100%"
        data-theme="light"
        style={{ minHeight: height, width: "100%" }}
      >
        <a
          href={settings.profile_url}
          target="_blank"
          rel="noopener noreferrer"
          className="underline"
        >
          {getTrustpilotCopy(locale).link}
        </a>
      </div>
    </div>
  )
}
