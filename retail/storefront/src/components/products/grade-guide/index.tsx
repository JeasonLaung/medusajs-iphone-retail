import { Button, Modal, theme } from "antd"
import Image from "../product-image"
import { useState, type CSSProperties } from "react"
import StorefrontTheme from "../storefront-theme"
import { getProductCopy, optionValueLabel } from "../i18n"
import styles from "./styles.module.css"

export default function GradeGuide({ locale, selectedGrade }: { locale: string; selectedGrade?: string }) {
  return <StorefrontTheme locale={locale.startsWith("zh") || locale.endsWith("-zh") ? "zh-CN" : "en"}>
    <Guide locale={locale} selectedGrade={selectedGrade} />
  </StorefrontTheme>
}

function Guide({ locale, selectedGrade }: { locale: string; selectedGrade?: string }) {
  const copy = getProductCopy(locale)
  const { token } = theme.useToken()
  const [open, setOpen] = useState(false)
  const [grade, setGrade] = useState("S")
  const [view, setView] = useState<"body" | "screen">("body")
  const [zoom, setZoom] = useState<{ x: number; y: number } | null>(null)
  const image = `/images/grade-guide/${grade.toLowerCase()}-${view}.png`
  const viewLabel = view === "body" ? copy.gradeGuideBody : copy.gradeGuideScreen
  const variables = {
    "--guide-primary": token.colorPrimary,
    "--guide-action": token.colorLink,
    "--guide-selected": token.colorPrimaryBg,
    "--guide-text": token.colorText,
    "--guide-secondary": token.colorTextSecondary,
    "--guide-surface": token.colorBgLayout,
    "--guide-border": token.colorBorder,
  } as CSSProperties

  return <>
    <Button type="text" shape="circle" size="small" aria-label={copy.gradeGuide} aria-haspopup="dialog" onClick={() => {
      setGrade(selectedGrade === "A" ? "A" : "S")
      setView("body")
      setZoom(null)
      setOpen(true)
    }} className={styles.trigger} style={variables} data-testid="grade-guide-trigger">
      <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" aria-hidden="true"><circle cx="12" cy="12" r="9" /><path d="M9.5 9a2.5 2.5 0 0 1 5 .5c0 1.5-2.5 1.5-2.5 3.5M12 16v1" /></svg>
    </Button>
    <Modal open={open} onCancel={() => setOpen(false)} footer={null} width={1000} title={copy.gradeGuideTitle} closable={{ "aria-label": copy.gradeGuideClose }} centered styles={{ body: { maxHeight: "calc(100dvh - 160px)", overflowY: "auto" } }} modalRender={content => <div onKeyDown={event => {
      if (event.key !== "Tab") return
      const buttons = event.currentTarget.querySelectorAll<HTMLButtonElement>("button:not([disabled])")
      const first = buttons[0]
      const last = buttons[buttons.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault()
        last?.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault()
        first?.focus()
      }
    }}>{content}</div>}>
      <div className={styles.guide} style={variables}>
        <div className={styles.copy}>
          <p className={styles.intro}>{copy.gradeGuideIntro}</p>
          <div className={styles.grades} role="group" aria-label={copy.condition}>
            {["S", "A"].map(value => <button type="button" key={value} aria-pressed={grade === value} onClick={() => { setGrade(value); setView("body"); setZoom(null) }}>{optionValueLabel(value, locale)}</button>)}
          </div>
          <h3>{grade === "S" ? copy.gradeGuideS : copy.gradeGuideA}</h3>
          <ul>{(grade === "S" ? copy.gradeGuideSPoints : copy.gradeGuideAPoints).map(point => <li key={point}>{point}</li>)}</ul>
        </div>
        <div className={styles.visual}>
          <div className={styles.image} onPointerMove={event => {
            if (event.pointerType !== "mouse") return
            const rect = event.currentTarget.getBoundingClientRect()
            setZoom({ x: (event.clientX - rect.left) / rect.width * 100, y: (event.clientY - rect.top) / rect.height * 100 })
          }} onPointerLeave={() => setZoom(null)}>
            {/* Local reference image: this illustrates grade, not the selected model. */}
            <Image src={image} width={640} height={640} sizes="(max-width: 700px) 90vw, 400px" draggable={false} alt={copy.gradeGuideImage.replace("{grade}", optionValueLabel(grade, locale)).replace("{view}", viewLabel)} />
            {zoom && <span className={styles.lens} aria-hidden="true" style={{ left: `${zoom.x}%`, top: `${zoom.y}%`, backgroundImage: `url(${image})`, backgroundPosition: `${zoom.x}% ${zoom.y}%` }} />}
          </div>
          <div className={styles.views} role="group" aria-label={copy.gallery}>
            {(["body", "screen"] as const).map(value => <button type="button" key={value} aria-pressed={view === value} onClick={() => { setView(value); setZoom(null) }}>{value === "body" ? copy.gradeGuideBody : copy.gradeGuideScreen}</button>)}
          </div>
          <p className={styles.zoomHint}>{copy.gradeGuideZoom}</p>
          <p className={styles.note}>{copy.gradeGuideImageNote}</p>
        </div>
      </div>
    </Modal>
  </>
}
