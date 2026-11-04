import {
  contactCsvTemplate,
  parseContactCsv,
} from "../admin/lib/contact-import"
import { MailCommand } from "../api/admin/mail/validators"

describe("contact CSV import", () => {
  it("round-trips the template with Chinese names, quotes, commas and multiline notes", () => {
    const csv =
      contactCsvTemplate +
      'client@example.com,"采购, 王","客户说""下周联系""\n需要报价"\r\n'
    expect(parseContactCsv(csv)).toEqual([
      {
        email: "client@example.com",
        name: "采购, 王",
        notes: '客户说"下周联系"\n需要报价',
      },
    ])
  })
  it("accepts reordered Chinese headers and legacy rows without inventing notes", () => {
    expect(
      parseContactCsv("备注,邮箱,名称\n法国客户,client@example.com,张三")[0]
    ).toEqual({ email: "client@example.com", name: "张三", notes: "法国客户" })
    expect(
      parseContactCsv("email,name\na@example.com,A")[0]
    ).not.toHaveProperty("notes")
    expect(parseContactCsv("a@example.com,A")[0]).not.toHaveProperty("notes")
  })
  it("rejects empty templates, broken quotes, invalid emails and oversized imports", () => {
    for (const csv of [
      contactCsvTemplate,
      'a@example.com,"unclosed',
      "invalid,A",
      "a@example.com,A,notes,extra",
      "a@example.com,A\n".repeat(1001),
    ]) {
      expect(() => parseContactCsv(csv)).toThrow("invalidCsv")
    }
  })
  it("validates notes and preserves omission for old clients", () => {
    const contact = { email: "a@example.com", name: "A", notes: "客户备注" }
    const command = MailCommand.parse({
      action: "contacts",
      contacts: [contact],
    })
    expect(command.action === "contacts" && command.contacts[0].notes).toBe(
      "客户备注"
    )
    expect(
      MailCommand.safeParse({
        action: "contacts",
        contacts: [{ ...contact, notes: "x".repeat(5001) }],
      }).success
    ).toBe(false)
    const legacy = MailCommand.parse({
      action: "contacts",
      contacts: [{ email: contact.email }],
    })
    expect(
      legacy.action === "contacts" && legacy.contacts[0].notes
    ).toBeUndefined()
  })
})
