export type ServiceContact = {
  id: string
  name: string
  department: "pre_sales" | "after_sales"
  whatsapp: string
  email: string
  timezone: string
  days: number[]
  start: string
  end: string
  availability: "schedule" | "online" | "offline"
}

export function isWithinServiceHours(contact: ServiceContact, now: Date): boolean {
  try {
    const parts = new Intl.DateTimeFormat("en-US", {
      timeZone: contact.timezone, weekday: "short", hour: "2-digit", minute: "2-digit", hourCycle: "h23",
    }).formatToParts(now)
    const value = (type: string) => parts.find(part => part.type === type)?.value || ""
    const day = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"].indexOf(value("weekday"))
    const time = `${value("hour")}:${value("minute")}`
    return contact.days.includes(day) && time >= contact.start && time < contact.end
  } catch {
    return false
  }
}

export function whatsappLink(number: string): string | null {
  return /^\+?[1-9]\d{6,14}$/.test(number) ? `https://wa.me/${number.replace(/^\+/, "")}` : null
}
