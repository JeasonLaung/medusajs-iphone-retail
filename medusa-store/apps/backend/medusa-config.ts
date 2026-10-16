import { loadEnv, defineConfig } from "@medusajs/framework/utils"

loadEnv(process.env.NODE_ENV || "development", process.cwd())

const localStoreOrigins = process.env.NODE_ENV === "production"
  ? []
  : ["http://localhost:8000", "http://127.0.0.1:8000"]
const withLocalStoreOrigins = (configured: string | undefined) =>
  [...new Set([...(configured || "").split(",").filter(Boolean), ...localStoreOrigins])].join(",")

module.exports = defineConfig({
  projectConfig: {
    databaseUrl: process.env.DATABASE_URL,
    redisUrl: process.env.REDIS_URL,
    http: {
      storeCors: withLocalStoreOrigins(process.env.STORE_CORS),
      adminCors: process.env.ADMIN_CORS!,
      authCors: withLocalStoreOrigins(process.env.AUTH_CORS),
      jwtSecret: process.env.JWT_SECRET,
      cookieSecret: process.env.COOKIE_SECRET,
    }
  },
  plugins: [
    {
      resolve: "@easypayment/medusa-payment-paypal",
      options: {},
    },
  ],
  modules: [
    { resolve: "./src/modules/mail" },
    { resolve: "./src/modules/order-numbering" },
    { resolve: "./src/modules/after-sales" },
    { resolve: "./src/modules/devices" },
    {
      resolve: "@medusajs/medusa/locking",
      options: { providers: [{ resolve: "@medusajs/medusa/locking-postgres", id: "locking-postgres", is_default: true }] },
    },
    {
      resolve: "@medusajs/medusa/payment",
      options: {
        providers: [
          {
            resolve: "@easypayment/medusa-payment-paypal/providers/paypal",
            id: "paypal",
            options: {},
            dependencies: ["paypal_onboarding"],
          },
          {
            resolve: "@easypayment/medusa-payment-paypal/providers/paypal_card",
            id: "paypal_card",
            options: {},
            dependencies: ["paypal_onboarding"],
          },
          // Mollie validates credentials during startup; enable it when configured.
          ...(process.env.MOLLIE_API_KEY
            ? [
                {
                  resolve: "./src/modules/mollie",
                  id: "mollie",
                  options: {
                    apiKey: process.env.MOLLIE_API_KEY,
                    redirectUrl: process.env.MOLLIE_REDIRECT_URL || `${process.env.STOREFRONT_URL}/api/mollie-return`,
                    medusaUrl: process.env.MEDUSA_BACKEND_URL,
                    autoCapture: true,
                    debug: false,
                  },
                },
              ]
            : []),
        ],
      },
    },
  ],
})
