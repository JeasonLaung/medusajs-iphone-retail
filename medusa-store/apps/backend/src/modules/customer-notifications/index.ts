import { Module } from "@medusajs/framework/utils"
import CustomerNotificationsService from "./service"

export const CUSTOMER_NOTIFICATIONS_MODULE = "customerNotifications"
export default Module(CUSTOMER_NOTIFICATIONS_MODULE, { service: CustomerNotificationsService })
