import { Module } from "@medusajs/framework/utils"
import MailModuleService from "./service"

export const MAIL_MODULE = "mail"
export default Module(MAIL_MODULE, { service: MailModuleService })
