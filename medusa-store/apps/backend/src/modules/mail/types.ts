// Medusa's JSON property is typed as an object, while PostgreSQL JSONB also supports arrays.
export const jsonArray = (value: unknown[]) =>
  value as unknown as Record<string, unknown>
export const arrayValue = <T>(value: unknown) =>
  Array.isArray(value) ? (value as T[]) : []
export type MailAttachment = {
  id?: string
  filename: string
  content?: string
  size?: number
}
