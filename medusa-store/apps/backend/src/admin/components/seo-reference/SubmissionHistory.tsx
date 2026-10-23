import tr from "./translate"
import { useEffect, useState } from "react"
import { Alert, App, Button, Modal, Space, Tag } from "antd"
import { Table } from "./ResizableTable"
import { sdk } from "../../lib/sdk"

export default function SubmissionHistory() {
  const { message } = App.useApp(), [data, setData] = useState<{ list: any[]; total: number }>({ list: [], total: 0 }), [page, setPage] = useState(1), [loading, setLoading] = useState(false), [selected, setSelected] = useState<any>(null)
  async function load() {
    setLoading(true)
    try { setData(await sdk.client.fetch("/admin/seo/submissions", { query: { page, size: 20 } })) }
    catch { message.error(tr("读取提交记录失败")) }
    finally { setLoading(false) }
  }
  useEffect(() => { void load() }, [page])
  return <><Space style={{ marginBottom: 16 }}><Button onClick={() => void load()}>{tr("刷新")}</Button></Space><Table rowKey="id" loading={loading} dataSource={data.list} columns={[
    { title: tr("时间"), dataIndex: "submitted_at", render: value => new Date(value).toLocaleString() },
    { title: tr("状态"), dataIndex: "success", render: value => <Tag color={value ? "success" : "error"}>{value ? tr("成功") : tr("失败")}</Tag> },
    { title: tr("URL 数量"), dataIndex: "submitted_count" }, { title: "HTTP", dataIndex: "http_status" },
    { title: tr("操作"), render: (_, row) => <Button size="small" onClick={() => setSelected(row)}>{tr("查看详情")}</Button> },
  ]} pagination={{ current: page, pageSize: 20, total: data.total, onChange: setPage, showSizeChanger: false }} />
    <Modal rootClassName="seo-reference" title={tr("IndexNow 接口响应")} width={760} open={!!selected} onCancel={() => setSelected(null)} footer={null}>
      {selected && <><Alert type={selected.success ? "success" : "error"} message={`HTTP ${selected.http_status}`} description={selected.error} /><pre style={{ maxHeight: 480, overflow: "auto", whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>{JSON.stringify({ urls: selected.urls?.list, headers: selected.response_headers, body: selected.response_body }, null, 2)}</pre></>}
    </Modal></>
}
