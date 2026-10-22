import { Module } from "@medusajs/framework/utils"
import WebhookNotificationsService from "./service"
export const WEBHOOK_NOTIFICATIONS_MODULE = "webhookNotifications"
export default Module(WEBHOOK_NOTIFICATIONS_MODULE, { service: WebhookNotificationsService })
