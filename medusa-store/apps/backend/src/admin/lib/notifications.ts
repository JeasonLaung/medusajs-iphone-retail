import { toast } from "@medusajs/ui"

export function notifyError(message: string) {
  toast.error(message, {
    position: "top-center",
    duration: 4000,
    dismissable: true,
  })
}
