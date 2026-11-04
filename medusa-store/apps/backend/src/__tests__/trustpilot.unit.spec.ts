import {
  defaultTrustpilot,
  TrustpilotBody,
  trustpilotSettings,
} from "../lib/trustpilot"

const configured = {
  ...defaultTrustpilot,
  enabled: true,
  profile_url: "https://www.trustpilot.com/review/example.com",
  business_unit_id: "507f191e810c19729de860ea",
  summary_template_id: "507f191e810c19729de860eb",
  reviews_template_id: "507f191e810c19729de860ec",
}

describe("Trustpilot display settings", () => {
  it("allows an unconfigured disabled integration but requires complete configuration before enabling", () => {
    expect(TrustpilotBody.safeParse(defaultTrustpilot).success).toBe(true)
    expect(
      TrustpilotBody.safeParse({ ...defaultTrustpilot, enabled: true }).success,
    ).toBe(false)
    expect(TrustpilotBody.safeParse(configured).success).toBe(true)
  })

  it.each([
    "javascript:alert(1)",
    "https://trustpilot.com.attacker.com/review/example.com",
    "https://www.trustpilot.com/review/example.com?redirect=bad",
    "https://user:pass@trustpilot.com/review/example.com",
  ])("rejects unsafe profile URLs: %s", (profile_url) => {
    expect(
      TrustpilotBody.safeParse({ ...configured, profile_url }).success,
    ).toBe(false)
  })

  it("rejects scripts, malformed IDs and widget heights outside supported bounds", () => {
    expect(
      TrustpilotBody.safeParse({
        ...configured,
        reviews_template_id: "<script>",
      }).success,
    ).toBe(false)
    expect(
      TrustpilotBody.safeParse({ ...configured, summary_height: 0 }).success,
    ).toBe(false)
    expect(
      TrustpilotBody.safeParse({ ...configured, reviews_height: 1201 }).success,
    ).toBe(false)
    expect(
      TrustpilotBody.safeParse({ ...configured, script: "alert(1)" }).success,
    ).toBe(false)
  })

  it("fails closed when saved metadata is incomplete and leaves other store metadata intact", async () => {
    const store = {
      id: "store_test",
      metadata: { another_setting: true, trustpilot: { enabled: true } },
    }
    const container = { resolve: () => ({ listStores: async () => [store] }) }
    const result = await trustpilotSettings(container as never)
    expect(result.settings).toEqual(defaultTrustpilot)
    expect(store.metadata.another_setting).toBe(true)
  })
})
