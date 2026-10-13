import { listCartShippingMethods } from "@lib/data/fulfillment"
import { getCheckoutPaymentOptions } from "@lib/data/payment"
import { HttpTypes } from "@medusajs/types"
import Addresses from "@modules/checkout/components/addresses"
import Payment from "@modules/checkout/components/payment"
import Review from "@modules/checkout/components/review"
import Shipping from "@modules/checkout/components/shipping"

export default async function CheckoutForm({
  cart,
  customer,
}: {
  cart: HttpTypes.StoreCart | null
  customer: HttpTypes.StoreCustomer | null
}) {
  if (!cart) {
    return null
  }

  const shippingMethods = await listCartShippingMethods(cart.id)
  const paymentMethods = await getCheckoutPaymentOptions(cart)

  return (
    <div className="w-full grid grid-cols-1 gap-y-8">
      <Addresses cart={cart} customer={customer} />

      {!shippingMethods && (
        <p role="alert" className="text-sm text-ui-fg-error">
          Delivery options could not be loaded. Please refresh checkout before
          continuing.
        </p>
      )}
      <Shipping cart={cart} availableShippingMethods={shippingMethods ?? []} />

      <Payment
        cart={cart}
        availablePaymentMethods={paymentMethods.available}
        unavailablePaymentMethods={paymentMethods.unavailable}
      />

      <Review cart={cart} />
    </div>
  )
}
