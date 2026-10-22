import { EventEmitter } from "node:events"
import { lookup } from "node:dns/promises"
import { request } from "node:https"
import { deliver } from "../modules/webhook-notifications/transport"

jest.mock("node:dns/promises", () => ({ lookup: jest.fn() }))
jest.mock("node:https", () => ({ request: jest.fn() }))

describe("webhook transport DNS pinning", () => {
  beforeEach(() => jest.resetAllMocks())

  it.each([true, false])("connects with DNS lookup all=%s using only the checked address", async all => {
    const selected = { address: "8.8.8.8", family: 4 }
    jest.mocked(lookup).mockResolvedValue([selected] as any)
    jest.mocked(request).mockImplementation(((_url, options, onResponse) => {
      const req = new EventEmitter() as any
      req.destroy = jest.fn()
      req.end = () => {
        options.lookup("example.com", { all }, (error, address, family) => {
          expect(error).toBeNull()
          if (all) expect(address).toEqual([selected])
          else {
            expect(address).toBe(selected.address)
            expect(family).toBe(4)
          }
        })
        const res = new EventEmitter() as any
        res.statusCode = 200
        onResponse(res)
        res.emit("data", "ok")
        res.emit("end")
        req.emit("close")
      }
      return req
    }) as any)
    await expect(deliver({ provider: "generic", url: "https://example.com/hook", secret: "", token: "" }, "test", "test", "id")).resolves.toBeUndefined()
  })

  it("rejects private DNS answers before opening a connection", async () => {
    jest.mocked(lookup).mockResolvedValue([{ address: "127.0.0.1", family: 4 }] as any)
    await expect(deliver({ provider: "generic", url: "https://example.com/hook", secret: "", token: "" }, "test", "test", "id")).rejects.toThrow("webhooks.errors.url")
    expect(request).not.toHaveBeenCalled()
  })
})
