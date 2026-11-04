import { useEffect, useState, type FormEvent } from "react"
import { defineRouteConfig } from "@medusajs/admin-sdk"
import {
  Button,
  Container,
  Drawer,
  Heading,
  Input,
  Label,
  Switch,
  Text,
  toast,
} from "@medusajs/ui"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { useTranslation } from "react-i18next"
import { sdk } from "../../../lib/sdk"
import { notifyError } from "../../../lib/notifications"

type Settings = {
  enabled: boolean
  profile_url: string
  business_unit_id: string
  summary_template_id: string
  reviews_template_id: string
  summary_height: number
  reviews_height: number
}
type Response = { settings: Settings }
const queryKey = ["trustpilot-settings"]
const fields = [
  "profile_url",
  "business_unit_id",
  "summary_template_id",
  "reviews_template_id",
  "summary_height",
  "reviews_height",
] as const

const TrustpilotPage = () => {
  const { t } = useTranslation()
  const copy = (key: string) => String(t(`trustpilot.${key}`))
  const client = useQueryClient()
  const [draft, setDraft] = useState<Settings | null>(null)
  const [invalid, setInvalid] = useState(false)
  const query = useQuery({
    queryKey,
    queryFn: () => sdk.client.fetch<Response>("/admin/trustpilot"),
  })
  useEffect(() => {
    if (query.error) notifyError(String(t("trustpilot.requestError")))
  }, [query.error, t])
  const save = useMutation({
    mutationFn: (settings: Settings) =>
      sdk.client.fetch<Response>("/admin/trustpilot", {
        method: "POST",
        body: settings,
      }),
    onSuccess: async (data) => {
      client.setQueryData(queryKey, data)
      setDraft(null)
      toast.success(copy("saved"))
      await client.invalidateQueries({ queryKey })
    },
    onError: () => notifyError(copy("requestError")),
  })
  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (!draft) return
    const ids = [
      draft.business_unit_id,
      draft.summary_template_id,
      draft.reviews_template_id,
    ]
    if (
      ids.some((value) => !/^([a-f\d]{24})?$/i.test(value)) ||
      (draft.profile_url &&
        !/^https:\/\/(?:www\.)?trustpilot\.com\/review\/[a-z\d.-]+\/?$/i.test(
          draft.profile_url,
        )) ||
      (draft.enabled && (!draft.profile_url || ids.some((value) => !value)))
    ) {
      setInvalid(true)
      return
    }
    save.mutate(draft)
  }
  const data = query.data?.settings
  return (
    <Container className="flex flex-col gap-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-2">
          <Heading>{copy("title")}</Heading>
          <Text size="small" className="text-ui-fg-subtle">
            {copy("description")}
          </Text>
        </div>
        <Button
          size="small"
          variant="secondary"
          disabled={!data || save.isPending}
          onClick={() => {
            if (data) {
              setDraft({ ...data })
              setInvalid(false)
            }
          }}
        >
          {copy("edit")}
        </Button>
      </div>
      <div className="max-w-3xl space-y-3">
        <Text size="small">{copy("setup")}</Text>
        <Text size="small" className="text-ui-fg-subtle">
          {copy("scope")}
        </Text>
        <Text size="small">
          <a
            className="text-ui-fg-interactive underline"
            href="https://businessapp.b2b.trustpilot.com/"
            target="_blank"
            rel="noopener noreferrer"
          >
            {copy("business")}
          </a>
        </Text>
      </div>
      {query.isPending && <Text size="small">{copy("loading")}</Text>}
      {query.isError && (
        <Button
          size="small"
          className="self-start"
          variant="secondary"
          onClick={() => query.refetch()}
        >
          {copy("retry")}
        </Button>
      )}
      {data && (
        <dl className="grid gap-6 sm:grid-cols-2">
          <div>
            <dt>
              <Text size="small" className="text-ui-fg-subtle">
                {copy("enabled")}
              </Text>
            </dt>
            <dd>
              <Text size="small">
                {copy(data.enabled ? "active" : "inactive")}
              </Text>
            </dd>
          </div>
          {fields.map((key) => (
            <div key={key} className="min-w-0">
              <dt>
                <Text size="small" className="text-ui-fg-subtle">
                  {copy(key)}
                </Text>
              </dt>
              <dd>
                <Text size="small" className="break-words">
                  {String(data[key] || copy("empty"))}
                </Text>
              </dd>
            </div>
          ))}
        </dl>
      )}
      <Drawer
        open={Boolean(draft)}
        onOpenChange={(open) => {
          if (!open && !save.isPending) setDraft(null)
        }}
      >
        <Drawer.Content>
          <Drawer.Header>
            <Drawer.Title>{copy("title")}</Drawer.Title>
          </Drawer.Header>
          <form onSubmit={submit} className="flex min-h-0 flex-1 flex-col">
            <Drawer.Body className="overflow-y-auto">
              <fieldset disabled={save.isPending} className="space-y-5">
                <Text size="small" className="text-ui-fg-subtle">
                  {copy("help")}
                </Text>
                {draft && (
                  <>
                    <div className="flex items-center gap-3">
                      <Switch
                        id="trustpilot-enabled"
                        checked={draft.enabled}
                        onCheckedChange={(enabled) => {
                          setDraft({ ...draft, enabled })
                          setInvalid(false)
                        }}
                      />
                      <Label htmlFor="trustpilot-enabled">
                        {copy("enabled")}
                      </Label>
                    </div>
                    {fields.map((key) => {
                      const numeric = key.endsWith("height")
                      return (
                        <div key={key} className="space-y-2">
                          <Label htmlFor={key}>{copy(key)}</Label>
                          <Input
                            id={key}
                            type={numeric ? "number" : "text"}
                            min={key === "summary_height" ? 40 : 140}
                            max={key === "summary_height" ? 300 : 1200}
                            step={1}
                            required={numeric || draft.enabled}
                            value={draft[key]}
                            onChange={(event) => {
                              setInvalid(false)
                              setDraft({
                                ...draft,
                                [key]: numeric
                                  ? Number(event.target.value)
                                  : event.target.value.trim(),
                              })
                            }}
                          />
                        </div>
                      )
                    })}
                  </>
                )}
                {invalid && (
                  <Text size="small" role="alert" className="text-ui-fg-error">
                    {copy("validation")}
                  </Text>
                )}
              </fieldset>
            </Drawer.Body>
            <Drawer.Footer>
              <Button
                size="small"
                type="button"
                variant="secondary"
                disabled={save.isPending}
                onClick={() => setDraft(null)}
              >
                {copy("cancel")}
              </Button>
              <Button size="small" type="submit" isLoading={save.isPending}>
                {copy("save")}
              </Button>
            </Drawer.Footer>
          </form>
        </Drawer.Content>
      </Drawer>
    </Container>
  )
}

export const config = defineRouteConfig({
  label: "trustpilot.title",
  translationNs: "translation",
})

export default TrustpilotPage
