import tr from "./translate"
import { App as AntdApp } from "antd"
import { useState } from "react"
import { Button, Image, Input, Modal, Space, Upload } from "antd"
import { UploadOutlined } from "@ant-design/icons"
import { sdk } from "../../lib/sdk"

// Preserve the reference page's image picker, using Medusa's file upload provider.
export default function FilePicker({ value = "", onChange, accept = "image/*", maxSize = 5 }: { value?: string; onChange?: (url: string) => void; accept?: string; maxSize?: number }) {
  const { message } = AntdApp.useApp()
  const [open, setOpen] = useState(false), [url, setURL] = useState(value), [uploading, setUploading] = useState(false)
  return <Space wrap><Input value={value} onChange={event => onChange?.(event.target.value)} placeholder="https://…" /><Button onClick={() => { setURL(value); setOpen(true) }}>{tr("选择图片")}</Button>
    <Modal rootClassName="seo-reference" open={open} title={tr("选择图片")} onCancel={() => setOpen(false)} onOk={() => { onChange?.(url); setOpen(false) }} okButtonProps={{ disabled: uploading }}>
      <Space direction="vertical" style={{ width: "100%" }}><Input value={url} onChange={event => setURL(event.target.value)} placeholder={tr("图片 URL")} />
        <Upload accept={accept} showUploadList={false} beforeUpload={file => { if (maxSize && file.size > maxSize * 1024 * 1024) { message.error(tr(`图片不能超过 ${maxSize} MB`)); return Upload.LIST_IGNORE } return true }} customRequest={async ({ file, onSuccess, onError }) => {
          setUploading(true)
          try { const body = new FormData(); body.append("files", file as File); const result = await sdk.client.fetch<{ files: { url: string }[] }>("/admin/uploads", { method: "POST", body, headers: { "Content-Type": null } }); setURL(result.files[0].url); onSuccess?.(result) }
          catch (error) { const failure = error instanceof Error ? error : new Error(tr("上传失败")); message.error(failure.message); onError?.(failure) }
          finally { setUploading(false) }
        }}><Button icon={<UploadOutlined />} loading={uploading}>{tr("上传图片")}</Button></Upload>
        {url && <Image src={url} alt="OG image" style={{ maxHeight: 240, objectFit: "contain" }} />}
      </Space>
    </Modal>
  </Space>
}
