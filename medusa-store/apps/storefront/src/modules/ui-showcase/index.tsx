"use client"

import { useState, type ReactNode, type CSSProperties } from "react"
import { Alert, Button as AntButton, Checkbox, Collapse, Empty, Form, Input, Menu, Pagination, Progress, Radio, Result, Select, Skeleton, Space, Spin, Statistic, Steps, Table, Tag, Timeline, Upload, theme } from "antd"
import { MagnifyingGlass, CheckCircleSolid } from "@medusajs/icons"
import { Button, IconButton, Input as AccountInput, Modal as AccountModal } from "@modules/account/components/ui"
import * as Shared from "@modules/common/components/ui"
import FloatingInput from "@modules/common/components/input"
import NativeSelect from "@modules/common/components/native-select"
import CountrySelect from "@modules/checkout/components/country-select"
import CheckboxWithLabel from "@modules/common/components/checkbox"
import FilterRadioGroup from "@modules/common/components/filter-radio-group"
import SelectionRow from "@modules/common/components/selection-row"
import Rating, { RatingInput } from "@modules/common/components/rating"
import GradeBadge from "@modules/common/components/grade-badge"
import Breadcrumbs from "@modules/common/components/breadcrumbs"
import InteractiveLink from "@modules/common/components/interactive-link"
import Modal from "@modules/common/components/modal"
import CartTotals from "@modules/common/components/cart-totals"
import Thumbnail from "@modules/products/components/thumbnail"
import SkeletonProductPreview from "@modules/skeletons/components/skeleton-product-preview"
import { getUiCopy } from "./i18n"
import styles from "./styles.module.css"

const sections = ["actions", "forms", "shared", "commerce", "feedback", "data", "overlays"] as const

function Section({ id, title, note, children }: { id: typeof sections[number]; title: string; note: string; children: ReactNode }) {
  return <section id={id} className={styles.section} aria-labelledby={`${id}-title`}>
    <header className={styles.sectionHeader}><h2 id={`${id}-title`}>{title}</h2><p>{note}</p></header>
    {children}
  </section>
}

export default function UiShowcase({ locale }: { locale: string | null }) {
  const c = getUiCopy(locale)
  const { token } = theme.useToken()
  const [form] = Form.useForm()
  const [rating, setRating] = useState(4)
  const [checked, setChecked] = useState(true)
  const [sort, setSort] = useState("newest")
  const [selection, setSelection] = useState({ storage: "512 GB", grade: "S", battery: "85%" })
  const [extras, setExtras] = useState<string[]>(["cable"])
  const [modal, setModal] = useState<"account" | "shared" | null>(null)
  const [page, setPage] = useState(1)
  const [menu, setMenu] = useState("list")
  const [notice, setNotice] = useState(false)
  const [saved, setSaved] = useState(false)
  const models = ["iPhone 15", "iPhone 14", "iPhone 13"].map(value => ({ value, label: value }))
  const capacities = ["128 GB", "256 GB", "512 GB"].map(value => ({ value, label: value }))
  const rows = Array.from({ length: 6 }, (_, index) => ({
    key: index, product: models[index % 3].label, storage: capacities[index % 3].label,
    status: index % 2 === 0 ? c.available : c.pending, price: index % 2 === 0 ? 499 : 399,
  }))
  const money = new Intl.NumberFormat(locale || "en", { style: "currency", currency: "EUR" })

  return <main className={styles.page} style={{ background: token.colorBgLayout, color: token.colorText, "--ui-surface": token.colorBgContainer, "--ui-muted": token.colorTextSecondary, "--ui-canvas": token.colorBgLayout } as CSSProperties}>
    <div className={styles.content}>
      <Breadcrumbs locale={locale || "en"} items={[{ label: c.home, href: "/" }, { label: "UI" }]} />
      <header className={styles.heading}><h1>{c.title}</h1><p style={{ color: token.colorTextSecondary }}>{c.intro}</p></header>
      <nav className={styles.sectionNav} aria-label={c.navigation}>
        {sections.map(id => <a key={id} href={`#${id}`} style={{ color: token.colorLink }}>{c[id]}</a>)}
      </nav>

      <Section id="actions" title={c.actions} note={c.actionsNote}>
        <Space wrap>
          <Button onClick={() => setNotice(true)}>{c.primary}</Button>
          <Button variant="secondary" onClick={() => setNotice(true)}>{c.secondary}</Button>
          <Button variant="transparent" onClick={() => setNotice(true)}>{c.text}</Button>
          <Button variant="danger" onClick={() => setNotice(true)}>{c.danger}</Button>
          <Button isLoading>{c.loading}</Button><Button disabled>{c.disabled}</Button>
          <Button size="small" onClick={() => setNotice(true)}>{c.small}</Button>
          <Button size="large" onClick={() => setNotice(true)}>{c.large}</Button>
          <IconButton aria-label={c.icon} onClick={() => setNotice(true)} icon={<MagnifyingGlass aria-hidden="true" />} />
          <AntButton type="primary" icon={<MagnifyingGlass aria-hidden="true" />} onClick={() => setNotice(true)}>{c.search}</AntButton>
          <InteractiveLink href="/store">{c.browse}</InteractiveLink>
        </Space>
        {notice && <Alert className={styles.notice} type="info" showIcon title={c.clicked} closable onClose={() => setNotice(false)} />}
      </Section>

      <Section id="forms" title={c.forms} note={c.formsNote}>
        <div className={styles.grid}>
          <AccountInput label={c.name} placeholder={c.placeholder} />
          <AccountInput label={c.email} type="email" placeholder="demo@example.com" />
          <AccountInput label={c.password} type="password" autoComplete="new-password" />
          <AccountInput label={c.phone} type="phone" />
          <Form.Item label={c.search} layout="vertical"><Input.Search aria-label={c.search} placeholder={c.search} allowClear onSearch={() => setNotice(true)} /></Form.Item>
          <Form.Item label={c.disabled} layout="vertical"><Input aria-label={c.disabled} disabled value={c.disabled} /></Form.Item>
          <Form.Item label={c.model} layout="vertical"><Select aria-label={c.model} className={styles.full} showSearch allowClear placeholder={c.choose} options={models} /></Form.Item>
          <Form.Item label={c.europeanCountry} layout="vertical"><CountrySelect aria-label={c.europeanCountry} placeholder={c.chooseCountry} data-testid="ui-european-country" /></Form.Item>
          <Form.Item label={c.multiple} layout="vertical"><Select aria-label={c.multiple} mode="multiple" className={styles.full} placeholder={c.choose} options={capacities} /></Form.Item>
          <Form.Item label={c.errorField} layout="vertical" validateStatus="error" help={c.errorHelp}><Input aria-label={c.errorField} aria-invalid="true" /></Form.Item>
          <Form.Item label={c.warningField} layout="vertical" validateStatus="warning" help={c.warningHelp}><Input aria-label={c.warningField} /></Form.Item>
          <Form.Item label={c.notes} layout="vertical"><Input.TextArea aria-label={c.notes} placeholder={c.placeholder} autoSize={{ minRows: 3, maxRows: 6 }} showCount maxLength={200} /></Form.Item>
          <div className={styles.stack}>
            <Radio.Group aria-label={c.radio} options={capacities} defaultValue="128 GB" />
            <Checkbox>{c.consent}</Checkbox>
            <Checkbox disabled>{c.disabled}</Checkbox>
            <Upload accept="image/*" multiple beforeUpload={() => false} listType="picture" maxCount={3}><AntButton>{c.upload}</AntButton></Upload>
            <p className={styles.muted}>{c.uploadHelp}</p>
          </div>
        </div>
      </Section>

      <Section id="shared" title={c.shared} note={c.sharedNote}>
        <div className={styles.grid} data-testid="selection-row-demo">
          <div className={styles.stack}>
            <h3>{c.selectionSingle}</h3>
            <p className={styles.muted}>{c.selectionHelp}</p>
            {([
              { key: "storage", title: c.storage, items: [{ value: "256 GB", amount: 0 }, { value: "512 GB", amount: 50 }] },
              { key: "grade", title: c.selectionGrade, items: [{ value: "S", amount: 30 }, { value: "A", amount: 0 }] },
              { key: "battery", title: c.selectionBattery, items: [{ value: "85%", amount: 0 }, { value: "100%", amount: 30 }] },
            ] as const).map(group => <fieldset key={group.key} className={styles.selectionGroup}>
              <legend>{group.title}</legend>
              <div className={styles.selectionRows}>{group.items.map(item => {
                const selected = selection[group.key] === item.value
                const difference = item.amount - (group.items.find(option => option.value === selection[group.key])?.amount ?? 0)
                return <SelectionRow key={item.value} name={`ui-selection-${group.key}`} value={item.value} checked={selected} onChange={() => setSelection(current => ({ ...current, [group.key]: item.value }))} trailing={!selected && difference !== 0 ? `${difference > 0 ? "+" : "−"} ${money.format(Math.abs(difference))}` : undefined}>
                  {group.key === "grade" ? <GradeBadge grade={item.value} locale={locale || "en"} /> : item.value}
                </SelectionRow>
              })}</div>
            </fieldset>)}
          </div>
          <div className={styles.stack}>
            <h3>{c.selectionMultiple}</h3>
            <p className={styles.muted}>{c.selectionMultipleHelp}</p>
            <fieldset className={styles.selectionGroup}>
              <legend>{c.selectionExtras}</legend>
              <div className={styles.selectionRows}>{[
                { value: "cable", label: c.selectionCable, amount: 10 },
                { value: "charger", label: c.selectionCharger, amount: 20 },
                { value: "case", label: c.selectionCase, amount: 15 },
              ].map(item => <SelectionRow key={item.value} type="checkbox" name="ui-selection-extras" value={item.value} checked={extras.includes(item.value)} onChange={event => setExtras(current => event.target.checked ? [...current, item.value] : current.filter(value => value !== item.value))} trailing={`+ ${money.format(item.amount)}`}>
                {item.label}
              </SelectionRow>)}
                <SelectionRow type="checkbox" name="ui-selection-extras" value="disabled" disabled trailing={c.unavailable}>{c.disabled}</SelectionRow>
              </div>
            </fieldset>
            <fieldset className={styles.selectionGroup} disabled>
              <legend>{c.disabled}</legend>
              <div className={styles.selectionRows}>
                <SelectionRow name="ui-selection-disabled" value="selected" checked readOnly>{c.selectionSelected}</SelectionRow>
                <SelectionRow name="ui-selection-disabled" value="unavailable" checked={false} readOnly>{c.unavailable}</SelectionRow>
              </div>
            </fieldset>
          </div>
        </div>
        <div className={styles.grid}>
          <div className={styles.stack}><h3>{c.floating}</h3><FloatingInput name="ui-floating" label={c.name} /><FloatingInput name="ui-floating-disabled" label={c.disabled} disabled /></div>
          <div className={styles.stack}><h3>{c.basic}</h3><Shared.Label htmlFor="ui-basic">{c.name}</Shared.Label><Shared.Input id="ui-basic" placeholder={c.placeholder} /></div>
          <div className={styles.stack}><h3>{c.native}</h3><NativeSelect aria-label={c.native} defaultValue="" placeholder={c.choose}>{models.map(item => <option key={item.value} value={item.value}>{item.label}</option>)}</NativeSelect></div>
          <div className={styles.stack}><h3>{c.checkbox}</h3><CheckboxWithLabel checked={checked} onChange={() => setChecked(value => !value)} label={c.checked} name="ui-address" /><Shared.Checkbox id="ui-shared-disabled" disabled label={c.disabled} /></div>
          <div className={styles.filter}><FilterRadioGroup title={c.filters} value={sort} handleChange={setSort} items={[{ value: "newest", label: c.newest }, { value: "price", label: c.price }]} /></div>
          <div className={styles.stack}><h3>RadioGroup</h3><Shared.RadioGroup role="radiogroup" aria-label={c.radio}>{capacities.map(item => <Shared.RadioGroup.Item key={item.value} id={`ui-${item.value}`} name="ui-capacity" value={item.value} label={item.label} defaultChecked={item.value === "128 GB"} />)}</Shared.RadioGroup></div>
          <div className={styles.stack}><h3>{c.badge}</h3><Space wrap>{(["green", "red", "blue", "orange", "grey"] as const).map((color, i) => <Shared.Badge color={color} key={color}>{[c.success, c.error, c.info, c.warning, c.disabled][i]}</Shared.Badge>)}<Shared.IconBadge><CheckCircleSolid aria-hidden="true" /></Shared.IconBadge></Space></div>
          <div className={styles.stack}><h3>{c.grade}</h3><Space wrap>{["S", "A", "B", "C"].map(grade => <GradeBadge key={grade} grade={grade} locale={locale || "en"} />)}</Space></div>
          <div className={styles.stack}><h3>{c.readRating}</h3><Rating value={4.5} label={c.readRating} /></div>
          <div className={styles.stack}><h3>{c.inputRating}</h3><RatingInput value={rating} onChange={setRating} disabled={false} label={c.rating} starLabels={c.stars} /></div>
          <div className={styles.stack}><h3>Button / IconButton</h3><Space wrap><Shared.Button onClick={() => setNotice(true)}>{c.primary}</Shared.Button><Shared.Button variant="secondary" onClick={() => setNotice(true)}>{c.secondary}</Shared.Button><Shared.Button variant="transparent" onClick={() => setNotice(true)}>{c.text}</Shared.Button><Shared.Button disabled>{c.disabled}</Shared.Button><Shared.IconButton aria-label={c.icon} onClick={() => setNotice(true)}><MagnifyingGlass aria-hidden="true" /></Shared.IconButton></Space></div>
          <div className={styles.stack}><h3>{c.typography}</h3><Shared.Container className={styles.sampleSurface}><Shared.Heading level="h3">{c.title}</Shared.Heading><Shared.Text>{c.textSample}</Shared.Text></Shared.Container></div>
        </div>
      </Section>

      <Section id="commerce" title={c.commerce} note={c.commerceNote}>
        <div className={styles.grid}>
          <div className={styles.stack}><h3>{c.thumbnail}</h3><div className={styles.productSample}><Thumbnail size="square" /></div></div>
          <div className={styles.stack}><h3>{c.totals}</h3><CartTotals totals={{ currency_code: "eur", item_subtotal: 499, shipping_subtotal: 10, discount_subtotal: 20, tax_total: 0, total: 489 }} /></div>
          <div className={styles.stack}><h3>{c.productSkeleton}</h3><SkeletonProductPreview /></div>
        </div>
      </Section>

      <Section id="feedback" title={c.feedback} note={c.feedbackNote}>
        <div className={styles.grid}>
          <Alert type="success" showIcon title={c.success} description={c.successMessage} />
          <Alert type="info" showIcon title={c.info} description={c.infoMessage} />
          <Alert type="warning" showIcon title={c.warning} description={c.warningMessage} />
          <Alert type="error" showIcon title={c.error} description={c.errorMessage} />
          <Space wrap><Tag color="success">{c.success}</Tag><Tag color="processing">{c.pending}</Tag><Tag color="warning">{c.warning}</Tag><Tag color="error">{c.error}</Tag><Tag>{c.disabled}</Tag></Space>
          <Space><Spin size="small" /><span>{c.loading}</span></Space>
          <Skeleton active title paragraph={{ rows: 2 }} />
          <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description={c.empty} />
        </div>
        <Result status="success" title={c.result} subTitle={c.resultNote} />
      </Section>

      <Section id="data" title={c.data} note={c.dataNote}>
        <div className={styles.grid}>
          <Statistic title={c.total} value={rows.length} />
          <div><h3>{c.progress}</h3><Progress percent={66} /><Progress percent={100} status="success" /><Progress percent={30} status="exception" /></div>
        </div>
        <div className={styles.stack}>
          <Steps current={1} items={c.steps.map(title => ({ title }))} />
          <Menu mode="horizontal" selectedKeys={[menu]} onClick={event => setMenu(event.key)} items={[{ key: "list", label: c.list }, { key: "details", label: c.details }]} />
          {menu === "list" ? <>
            <Table size="small" scroll={{ x: 480 }} pagination={false} dataSource={rows.slice((page - 1) * 3, page * 3)} columns={[
              { title: c.product, dataIndex: "product" }, { title: c.storage, dataIndex: "storage" },
              { title: c.status, dataIndex: "status", render: value => <Tag color={value === c.available ? "success" : "processing"}>{value}</Tag> },
              { title: c.amount, dataIndex: "price", render: value => money.format(value) },
            ]} />
            <Pagination current={page} onChange={setPage} total={rows.length} pageSize={3} showSizeChanger={false} />
          </> : <Timeline items={c.timeline.map(content => ({ content }))} />}
          <Collapse items={[{ key: "faq", label: c.faq, children: c.faqAnswer }]} />
        </div>
      </Section>

      <Section id="overlays" title={c.overlays} note={c.overlaysNote}>
        <Space wrap><Button onClick={() => setModal("account")}>{c.openAccount}</Button><Button variant="secondary" onClick={() => setModal("shared")}>{c.openShared}</Button></Space>
        <div className={styles.demoForm}>
          <p className={styles.muted}>{c.tryForm}</p>
          <Form form={form} layout="vertical" requiredMark onFinish={() => setSaved(true)} onValuesChange={() => setSaved(false)}>
            <Form.Item label={c.name} name="demoName" rules={[{ required: true, whitespace: true, message: c.required }]}><Input autoComplete="off" /></Form.Item>
            <Form.Item label={c.email} name="demoEmail" rules={[{ required: true, message: c.required }, { type: "email", message: c.invalidEmail }]}><Input autoComplete="off" /></Form.Item>
            <Space wrap><Button type="submit">{c.submit}</Button><Button variant="secondary" onClick={() => { form.resetFields(); setSaved(false) }}>{c.reset}</Button></Space>
          </Form>
          {saved && <Alert className={styles.notice} type="success" showIcon title={c.saved} role="status" />}
        </div>
      </Section>
      <AccountModal isOpen={modal === "account"} close={() => setModal(null)}>
        <AccountModal.Title><h2>{c.modalTitle}</h2></AccountModal.Title>
        <AccountModal.Body><p>{c.modalBody}</p></AccountModal.Body>
        <AccountModal.Footer><Button onClick={() => setModal(null)}>{c.close}</Button></AccountModal.Footer>
      </AccountModal>
      <Modal isOpen={modal === "shared"} close={() => setModal(null)} size="small">
        <Modal.Title closeLabel={c.close}>{c.modalTitle}</Modal.Title>
        <Modal.Description>{c.modalBody}</Modal.Description>
        <Modal.Footer><Button onClick={() => setModal(null)}>{c.close}</Button></Modal.Footer>
      </Modal>
    </div>
  </main>
}
