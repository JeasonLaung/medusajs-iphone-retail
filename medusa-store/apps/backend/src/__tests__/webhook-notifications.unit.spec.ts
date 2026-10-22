import { createHmac } from "node:crypto"
import { buildRequest, publicAddress, validateUrl } from "../modules/webhook-notifications/transport"

const base = { url: "https://open.feishu.cn/open-apis/bot/v2/hook/test", secret: "secret", token: "" }
describe("webhook notifications", () => {
  it("signs Feishu requests in seconds with the timestamp and secret as HMAC key", () => {
    const { body } = buildRequest({ ...base, provider: "feishu" }, "hello", "test", "id", 1700000000123)
    expect(body).toEqual({ msg_type: "text", content: { text: "hello" }, timestamp: "1700000000", sign: createHmac("sha256", "1700000000\nsecret").update("").digest("base64") })
  })
  it("signs DingTalk requests in milliseconds and URL-encodes the signature", () => {
    const { url, body } = buildRequest({ ...base, provider: "dingtalk", url: "https://oapi.dingtalk.com/robot/send?access_token=test" }, "hello", "test", "id", 1700000000123)
    expect(url.searchParams.get("timestamp")).toBe("1700000000123")
    expect(url.searchParams.get("sign")).toBe(createHmac("sha256", "secret").update("1700000000123\nsecret").digest("base64"))
    expect(url.searchParams.get("access_token")).toBe("test")
    expect(body).toEqual({ msgtype: "text", text: { content: "hello" } })
  })
  it("uses WeCom text format and Slack/Mattermost text format", () => {
    expect(buildRequest({ ...base, provider: "wecom", url: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test" }, "hello", "test", "id").body).toEqual({ msgtype: "text", text: { content: "hello" } })
    for (const provider of ["slack", "mattermost"] as const) expect(buildRequest({ ...base, provider, url: provider === "slack" ? "https://hooks.slack.com/services/test" : "https://chat.example.com/hooks/test" }, "hello", "test", "id").body).toEqual({ text: "hello" })
  })
  it("provides a stable generic JSON envelope", () => {
    expect(buildRequest({ ...base, provider: "generic", url: "https://automation.example.com/webhook/test" }, "hello", "order.placed", "order_123", 0).body).toEqual({ event: "order.placed", resource_id: "order_123", text: "hello", timestamp: "1970-01-01T00:00:00.000Z" })
  })
  it.each(["http://example.com", "https://localhost", "https://127.0.0.1", "https://10.0.0.1", "https://[::1]", "https://[::ffff:127.0.0.1]", "https://user:pass@example.com", "https://example.com:9001", "https://example.com/#fragment"])("rejects unsafe destination %s", url => {
    expect(() => validateUrl(url, "generic")).toThrow("webhooks.errors.url")
  })
  it("restricts hosted providers to their own API hosts", () => {
    expect(() => validateUrl("https://example.com/hook", "feishu")).toThrow()
    expect(() => validateUrl("https://open.feishu.cn.evil.example/hook", "feishu")).toThrow()
    expect(validateUrl("https://open.larksuite.com/open-apis/bot/v2/hook/test", "feishu").hostname).toBe("open.larksuite.com")
  })
  it.each(["10.0.0.2", "172.16.0.2", "192.168.0.1", "169.254.169.254", "100.100.100.200", "0.0.0.0", "224.0.0.1", "fc00::1", "fe80::1", "::ffff:8.8.8.8"])("blocks nonpublic resolved addresses %s", address => expect(publicAddress(address)).toBe(false))
  it("accepts public IPv4 and IPv6 destinations", () => {
    expect(publicAddress("8.8.8.8")).toBe(true)
    expect(publicAddress("2606:4700:4700::1111")).toBe(true)
  })
})
