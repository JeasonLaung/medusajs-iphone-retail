// Original brand-logo markup; native image keeps its intrinsic 40px geometry.

export default function BrandLogo() {
  return (
    <span className="inline-flex items-center justify-center gap-2 whitespace-nowrap">
      <img src="/brand/eurorefurb-logo.png" alt="" width={40} height={40}
        className="h-10 w-10 shrink-0" />
      <span className="brand-wordmark">EuroRefurb</span>
    </span>
  )
}
