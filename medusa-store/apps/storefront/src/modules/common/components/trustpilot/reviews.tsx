import type { TrustpilotSettings } from "types/trustpilot"
import TrustpilotWidget from "."
import { getTrustpilotCopy } from "./i18n"

export default function TrustpilotReviews({
  settings,
  locale,
}: {
  settings: TrustpilotSettings
  locale: string | null
}) {
  const copy = getTrustpilotCopy(locale)
  return (
    <section
      className="my-12 space-y-6"
      aria-labelledby="trustpilot-reviews-title"
    >
      <header className="space-y-2">
        <h2 id="trustpilot-reviews-title" className="text-2xl-semi">
          {copy.title}
        </h2>
        <p>{copy.description}</p>
      </header>
      <TrustpilotWidget settings={settings} locale={locale} />
      <a
        className="inline-block underline"
        href={settings.profile_url}
        target="_blank"
        rel="noopener noreferrer"
      >
        {copy.link}
      </a>
    </section>
  )
}
