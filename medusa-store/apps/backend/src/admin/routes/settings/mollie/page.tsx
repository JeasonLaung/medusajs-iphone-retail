import { useEffect, useState } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import { Button, Container, Heading, Input, Label, Select, Text, toast } from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { sdk } from "../../../lib/sdk"

type Config = { environment: "test" | "production"; redirect_url: string; enabled: boolean; has_api_key: boolean; api_key_hint: string }

const MollieSettings = () => {
  const queryClient = useQueryClient()
  const [environment, setEnvironment] = useState<Config["environment"]>("test")
  const [apiKey, setApiKey] = useState("")
  const [redirectUrl, setRedirectUrl] = useState("")
  const [enabled, setEnabled] = useState(true)
  const { data, isLoading } = useQuery({
    queryKey: ["mollie-settings"],
    queryFn: async () => (await sdk.client.fetch<{ config: Config }>("/admin/mollie")).config,
  })
  useEffect(() => {
    if (!data) return
    setEnvironment(data.environment)
    setRedirectUrl(data.redirect_url)
    setEnabled(data.enabled)
  }, [data])
  const save = useMutation({
    mutationFn: () => sdk.client.fetch("/admin/mollie", { method: "POST", body: { environment, api_key: apiKey || undefined, redirect_url: redirectUrl, enabled } }),
    onSuccess: () => { setApiKey(""); queryClient.invalidateQueries({ queryKey: ["mollie-settings"] }); toast.success("Mollie settings saved") },
    onError: (error: Error) => toast.error(error.message || "Unable to save Mollie settings"),
  })
  return (
    <Container className="flex max-w-3xl flex-col gap-6 p-6">
      <div>
        <Heading>Mollie payments</Heading>
        <Text size="small" className="text-ui-fg-subtle">Configure Mollie without exposing the API key in the storefront.</Text>
      </div>
      {isLoading ? <Text>Loading…</Text> : <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-2"><Label>Mode</Label><Select value={environment} onValueChange={(value) => setEnvironment(value as Config["environment"])}><Select.Trigger><Select.Value /></Select.Trigger><Select.Content><Select.Item value="test">Test</Select.Item><Select.Item value="production">Production</Select.Item></Select.Content></Select></div>
        <div className="flex flex-col gap-2"><Label>{data?.has_api_key ? `API key (${data.api_key_hint})` : "API key"}</Label><Input type="password" value={apiKey} onChange={(event) => setApiKey(event.target.value)} placeholder={data?.has_api_key ? "Leave blank to keep current key" : environment === "test" ? "test_…" : "live_…"} /></div>
        <div className="flex flex-col gap-2"><Label>Payment return URL</Label><Input value={redirectUrl} onChange={(event) => setRedirectUrl(event.target.value)} /></div>
        <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /> Enable Mollie payments</label>
        <Button size="small" className="self-start" onClick={() => save.mutate()} isLoading={save.isPending} disabled={save.isPending}>Save settings</Button>
      </div>}
    </Container>
  )
}

export const config = defineRouteConfig({ label: "Mollie" })
export default MollieSettings
