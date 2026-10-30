"use client"

import { useCallback, useEffect, useRef, useState } from "react"
import { Dialog, DialogPanel, DialogTitle } from "@headlessui/react"
import { XMark } from "../layout/icons"
import { customerClient } from "../../lib/customer-client"
import {type CustomerNotice,type NoticePage} from "../../lib/customer-notices"

import { getNotificationCopy } from "./i18n"
import header from "../layout/header.module.css"
import styles from "./styles.module.css"

export default function NotificationBell({ locale,segment }: { locale: string | null;segment:string }) {
  const copy = getNotificationCopy(locale)
  const sequence=useRef(0),active=useRef(true)
  const [api] = useState(()=>customerClient())
  const countryCode=segment
  const readNotifications=async(id?:string)=>{try{await api.readNotifications(id);return true}catch{return false}}
  const [open, setOpen] = useState(false)
  const [data, setData] = useState<NoticePage | null>(null)
  const [offset, setOffset] = useState(0)
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)
  const refresh = useCallback(async () => {
    const run=++sequence.current
    try {
      const result=await api.notifications(offset)
      if(active.current&&run===sequence.current){setData(result);setError("")}
    } catch {if(active.current&&run===sequence.current)setError("load")}
  }, [api,offset])
  useEffect(() => {
    active.current=true
    void refresh()
    const timer=window.setInterval(()=>{if(document.visibilityState==='visible')void refresh()},30000)
    const focus=()=>void refresh()
    window.addEventListener('focus',focus)
    return()=>{active.current=false;sequence.current++;clearInterval(timer);window.removeEventListener('focus',focus)}
  },[refresh])
  async function mark(id?: string, notice?: CustomerNotice) {
    setBusy(true)
    try {
      if (!await readNotifications(id)) { setError("read"); return }
      if(!active.current)return
      await refresh()
      if (notice) {
        setOpen(false)
        window.location.assign(`/${countryCode}/account/${notice.kind === "buyback.updated" ? "buyback" : "orders"}/${encodeURIComponent(notice.resource_id)}`)
      }
    } finally { if(active.current)setBusy(false) }
  }
  const count = data?.unread || 0
  return <>
    <button type="button" className={header.iconButton} aria-label={`${copy.title} (${count})`} aria-haspopup="dialog" onClick={() => { setOpen(true); void refresh() }} data-testid="notification-bell">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" aria-hidden="true"><path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9Z" /><path d="M10 21h4" /></svg>
      {count > 0 && <span className={styles.badge} aria-hidden="true" data-testid="notification-badge">{count > 99 ? "99+" : count}</span>}
    </button>
    <Dialog open={open} onClose={setOpen} className={styles.dialog}>
      <div className={styles.backdrop} aria-hidden="true" />
      <DialogPanel className={styles.panel}>
        <div className={styles.heading}><DialogTitle>{copy.title}</DialogTitle><button type="button" className={styles.close} onClick={() => setOpen(false)} aria-label={copy.close}><XMark aria-hidden="true" /></button></div>
        <div className={styles.toolbar}><span aria-live="polite">{copy.unread} ({count})</span><button disabled={busy || !count} onClick={() => void mark()}>{copy.allRead}</button></div>
        {error && <p role="alert" className={styles.error}>{error === "read" ? copy.readError : copy.error} <button onClick={() => void refresh()}>{copy.retry}</button></p>}
        {!data && !error && <p role="status" className={styles.empty}>{copy.loading}</p>}
        {data?.notifications.length === 0 && <p className={styles.empty}>{copy.empty}</p>}
        <ul className={styles.list}>{data?.notifications.map(notice => <li key={notice.id} className={styles.item} data-unread={!notice.read_at}>
          <div className={styles.title}>{copy.events[notice.kind] || copy.title}{!notice.read_at && <span className={styles.unread}>{copy.unread}</span>}</div>
          {notice.action && <p>{copy.actions[notice.action] || copy.events[notice.kind]}</p>}
          <p className={styles.reference}>{notice.reference}</p>
          <time dateTime={notice.created_at}>{new Intl.DateTimeFormat(locale?.startsWith("zh") ? "zh-CN" : "en-GB", { dateStyle: "medium", timeStyle: "short" }).format(new Date(notice.created_at))}</time>
          <div className={styles.itemActions}><button disabled={busy} onClick={() => void mark(notice.id, notice)}>{copy.open}</button>{!notice.read_at && <button disabled={busy} onClick={() => void mark(notice.id)}>{copy.read}</button>}</div>
        </li>)}</ul>
        {data && data.count > 20 && <div className={styles.pagination}><button disabled={busy || offset === 0} onClick={() => { setData(null); setOffset(offset - 20) }}>{copy.previous}</button><span>{offset / 20 + 1} / {Math.ceil(data.count / 20)}</span><button disabled={busy || offset + 20 >= data.count} onClick={() => { setData(null); setOffset(offset + 20) }}>{copy.next}</button></div>}
      </DialogPanel>
    </Dialog>
  </>
}
