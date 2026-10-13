import upstream from "@variablevic/mollie-payments-medusa/providers/mollie"
import { ModuleProvider, Modules } from "@medusajs/framework/utils"
import { createMollieProvider, type MollieProviderConstructor } from "./service"

const providers = upstream as { services: MollieProviderConstructor[] }

export default ModuleProvider(Modules.PAYMENT, {
  services: providers.services.map(createMollieProvider),
})
