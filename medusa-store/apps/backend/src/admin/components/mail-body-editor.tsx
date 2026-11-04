import { useEffect, useRef, useState } from "react"
import { Button, Input, Label, Text, Textarea } from "@medusajs/ui"
import {
  AlignCenterOutlined,
  AlignLeftOutlined,
  AlignRightOutlined,
  BoldOutlined,
  ClearOutlined,
  ItalicOutlined,
  LinkOutlined,
  OrderedListOutlined,
  PictureOutlined,
  RedoOutlined,
  StrikethroughOutlined,
  TableOutlined,
  UnderlineOutlined,
  UndoOutlined,
  UnorderedListOutlined,
} from "@ant-design/icons"
import { useTranslation } from "react-i18next"
import { emailDocument } from "../lib/mail"
import {
  editableMailHtml,
  mailHtmlText,
  mailTextHtml,
  safeMailUrl,
} from "../lib/mail-editor"
import "./mail-body-editor.css"

type Body = { text_body: string; html_body: string }
type Props = Body & { disabled?: boolean; onChange: (body: Body) => void }

export default function MailBodyEditor({
  text_body,
  html_body,
  disabled = false,
  onChange,
}: Props) {
  const { t } = useTranslation()
  const [mode, setMode] = useState("rich")
  const [panel, setPanel] = useState<"link" | "image" | "table" | null>(null)
  const [url, setUrl] = useState("")
  const [alt, setAlt] = useState("")
  const [rows, setRows] = useState(3)
  const [columns, setColumns] = useState(3)
  const [active, setActive] = useState<string[]>([])
  const editor = useRef<HTMLDivElement>(null)
  const selection = useRef<Range | null>(null)
  const lastValue = useRef<string | null>(null)
  const value = html_body || mailTextHtml(text_body)

  useEffect(() => {
    if (mode !== "rich" || !editor.current) return
    if (lastValue.current !== value) {
      editor.current.innerHTML = editableMailHtml(value)
      selection.current = null
      lastValue.current = value
    }
  }, [value, mode])

  function remember() {
    const current = window.getSelection()
    if (
      current?.rangeCount &&
      editor.current?.contains(current.anchorNode) &&
      editor.current.contains(current.focusNode)
    ) {
      selection.current = current.getRangeAt(0).cloneRange()
      setActive(
        [
          "bold",
          "italic",
          "underline",
          "strikeThrough",
          "insertUnorderedList",
          "insertOrderedList",
          "justifyLeft",
          "justifyCenter",
          "justifyRight",
        ].filter((command) => document.queryCommandState(command))
      )
    }
  }
  function emit() {
    if (!editor.current) return
    // Chromium omits styles inherited from Admin's reset during insertHTML.
    // Store explicit table styles so the same table renders in email clients.
    const content = editor.current.cloneNode(true) as HTMLElement
    content.querySelectorAll("table").forEach((table) => {
      table.style.borderCollapse ||= "collapse"
      table.querySelectorAll<HTMLElement>("td,th").forEach((cell) => {
        if (cell.style.borderWidth && cell.style.borderWidth !== "0px") {
          cell.style.borderStyle ||= "solid"
        }
      })
    })
    const html = content.innerHTML
    const text = mailHtmlText(html)
    const body =
      !text.trim() && !editor.current.querySelector("img,table,hr") ? "" : html
    lastValue.current = body
    onChange({ html_body: body, text_body: text })
    remember()
  }
  function run(command: string, argument?: string) {
    if (disabled || !editor.current) return
    editor.current.focus()
    const current = window.getSelection()
    if (
      selection.current &&
      editor.current.contains(selection.current.commonAncestorContainer)
    ) {
      current?.removeAllRanges()
      current?.addRange(selection.current)
    }
    document.execCommand(command, false, argument)
    emit()
  }
  const commands = [
    ["undo", "undo", UndoOutlined],
    ["redo", "redo", RedoOutlined],
    ["bold", "bold", BoldOutlined],
    ["italic", "italic", ItalicOutlined],
    ["underline", "underline", UnderlineOutlined],
    ["strikeThrough", "strike", StrikethroughOutlined],
    ["justifyLeft", "alignLeft", AlignLeftOutlined],
    ["justifyCenter", "alignCenter", AlignCenterOutlined],
    ["justifyRight", "alignRight", AlignRightOutlined],
    ["insertUnorderedList", "bulletList", UnorderedListOutlined],
    ["insertOrderedList", "numberList", OrderedListOutlined],
    ["removeFormat", "clearFormat", ClearOutlined],
  ] as const
  function openPanel(next: typeof panel) {
    remember()
    setUrl("")
    setAlt("")
    setPanel(next)
  }
  function insert() {
    if (panel === "table") {
      const cell = `<td style="border:1px solid #D9D9D9;padding:8px"><br></td>`
      run(
        "insertHTML",
        `<table style="border-collapse:collapse;width:100%"><tbody>${Array.from({ length: rows }, () => `<tr>${cell.repeat(columns)}</tr>`).join("")}</tbody></table><p><br></p>`
      )
    } else if (safeMailUrl(url.trim(), panel === "image")) {
      if (panel === "image") {
        const image = document.createElement("img")
        image.src = url.trim()
        image.alt = alt
        image.style.maxWidth = "100%"
        image.style.height = "auto"
        run("insertHTML", image.outerHTML)
      } else if (selection.current && !selection.current.collapsed)
        run("createLink", url.trim())
      else {
        const link = document.createElement("a")
        link.href = url.trim()
        link.textContent = url.trim()
        run("insertHTML", link.outerHTML)
      }
    } else return
    setPanel(null)
  }
  return (
    <div className="mail-body-editor">
      <div
        className="flex flex-wrap gap-2"
        role="group"
        aria-label={t("mail.editor.mode")}
      >
        {["rich", "text", "html", "preview"].map((tab) => (
          <Button
            key={tab}
            type="button"
            size="small"
            disabled={disabled}
            aria-pressed={mode === tab}
            variant={mode === tab ? "primary" : "secondary"}
            onClick={() => {
              setMode(tab)
              setPanel(null)
              lastValue.current = null
            }}
          >
            {t(tab === "rich" ? "mail.editor.rich" : `mail.${tab}`)}
          </Button>
        ))}
      </div>
      {mode === "rich" ? (
        <div className="mail-rich-container" aria-disabled={disabled}>
          <div
            className="mail-rich-toolbar"
            role="group"
            aria-label={t("mail.editor.toolbar")}
          >
            {commands.map(([command, label, Icon]) => (
              <Button
                key={command}
                type="button"
                size="small"
                variant={active.includes(command) ? "secondary" : "transparent"}
                aria-pressed={active.includes(command)}
                title={t(`mail.editor.${label}`)}
                aria-label={t(`mail.editor.${label}`)}
                disabled={disabled}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => run(command)}
              >
                <Icon />
              </Button>
            ))}
            <select
              className="mail-rich-select"
              aria-label={t("mail.editor.paragraph")}
              disabled={disabled}
              defaultValue=""
              onChange={(event) => {
                run("formatBlock", event.target.value)
                event.target.value = ""
              }}
            >
              <option value="" disabled>
                {t("mail.editor.paragraph")}
              </option>
              <option value="p">{t("mail.editor.normal")}</option>
              {[1, 2, 3].map((level) => (
                <option key={level} value={`h${level}`}>
                  {t("mail.editor.heading", { level })}
                </option>
              ))}
              <option value="blockquote">{t("mail.editor.quote")}</option>
            </select>
            <select
              className="mail-rich-select"
              aria-label={t("mail.editor.fontSize")}
              disabled={disabled}
              defaultValue=""
              onChange={(event) => {
                run("fontSize", event.target.value)
                event.target.value = ""
              }}
            >
              <option value="" disabled>
                {t("mail.editor.fontSize")}
              </option>
              {[1, 2, 3, 4, 5, 6, 7].map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
            <label className="mail-rich-color" title={t("mail.editor.color")}>
              <Text size="small">{t("mail.editor.color")}</Text>
              <input
                type="color"
                aria-label={t("mail.editor.color")}
                disabled={disabled}
                defaultValue="#000000"
                onChange={(event) => run("foreColor", event.target.value)}
              />
            </label>
            {(
              [
                ["link", LinkOutlined],
                ["image", PictureOutlined],
                ["table", TableOutlined],
              ] as const
            ).map(([name, Icon]) => (
              <Button
                key={name}
                type="button"
                size="small"
                variant="transparent"
                title={t(`mail.editor.${name}`)}
                aria-label={t(`mail.editor.${name}`)}
                disabled={disabled}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => openPanel(name)}
              >
                <Icon />
              </Button>
            ))}
          </div>
          {panel && (
            <div
              className="mail-rich-insert"
              role="group"
              aria-label={t(`mail.editor.${panel}`)}
            >
              {panel === "table" ? (
                <>
                  {(["rows", "columns"] as const).map((field) => (
                    <label key={field} className="flex items-center gap-2">
                      <Text size="small">{t(`mail.editor.${field}`)}</Text>
                      <Input
                        className="w-20"
                        type="number"
                        min={1}
                        max={20}
                        disabled={disabled}
                        value={field === "rows" ? rows : columns}
                        onChange={(event) => {
                          const size = Math.max(
                            1,
                            Math.min(20, Number(event.target.value) || 1)
                          )
                          if (field === "rows") setRows(size)
                          else setColumns(size)
                        }}
                      />
                    </label>
                  ))}
                </>
              ) : (
                <div className="flex min-w-0 flex-1 flex-col gap-2">
                  <Label htmlFor="mail-editor-url">
                    {t("mail.editor.url")}
                  </Label>
                  <Input
                    id="mail-editor-url"
                    autoFocus
                    value={url}
                    disabled={disabled}
                    onChange={(event) => setUrl(event.target.value)}
                    aria-invalid={
                      !!url && !safeMailUrl(url.trim(), panel === "image")
                    }
                  />
                  <Text size="small" className="text-ui-fg-subtle">
                    {t(
                      panel === "image"
                        ? "mail.editor.imageHint"
                        : "mail.editor.linkHint"
                    )}
                  </Text>
                  {panel === "image" && (
                    <Input
                      aria-label={t("mail.editor.alt")}
                      placeholder={t("mail.editor.alt")}
                      value={alt}
                      disabled={disabled}
                      onChange={(event) => setAlt(event.target.value)}
                    />
                  )}
                </div>
              )}
              <Button
                type="button"
                size="small"
                disabled={
                  disabled ||
                  (panel !== "table" &&
                    !safeMailUrl(url.trim(), panel === "image"))
                }
                onClick={insert}
              >
                {t("mail.editor.insert")}
              </Button>
              <Button
                type="button"
                size="small"
                variant="secondary"
                onClick={() => setPanel(null)}
              >
                {t("mail.cancel")}
              </Button>
            </div>
          )}
          <div
            ref={editor}
            className="mail-rich-surface"
            role="textbox"
            aria-label={t("mail.body")}
            aria-multiline="true"
            aria-readonly={disabled}
            contentEditable={!disabled}
            suppressContentEditableWarning
            onInput={emit}
            onKeyUp={remember}
            onMouseUp={remember}
            onBlur={remember}
            onPaste={(event) => {
              event.preventDefault()
              const html = event.clipboardData.getData("text/html")
              run(
                "insertHTML",
                html
                  ? editableMailHtml(html)
                  : mailTextHtml(event.clipboardData.getData("text/plain"))
              )
            }}
            onDrop={(event) => event.preventDefault()}
          />
        </div>
      ) : mode === "preview" ? (
        <iframe
          className="mail-rich-preview"
          sandbox=""
          title={t("mail.preview")}
          srcDoc={emailDocument(
            html_body ||
              `<div style="white-space:pre-wrap">${mailTextHtml(text_body)}</div>`
          )}
        />
      ) : (
        <>
          {mode === "text" && html_body && (
            <Text size="small" className="text-ui-fg-subtle">
              {t("mail.editor.plainHint")}
            </Text>
          )}
          <Textarea
            className="min-h-80"
            aria-label={t(mode === "html" ? "mail.html" : "mail.body")}
            value={
              mode === "html"
                ? value
                : html_body
                  ? mailHtmlText(html_body)
                  : text_body
            }
            disabled={disabled}
            onChange={(event) =>
              onChange(
                mode === "html"
                  ? {
                      html_body: event.target.value,
                      text_body: mailHtmlText(event.target.value),
                    }
                  : { text_body: event.target.value, html_body: "" }
              )
            }
          />
        </>
      )}
    </div>
  )
}
