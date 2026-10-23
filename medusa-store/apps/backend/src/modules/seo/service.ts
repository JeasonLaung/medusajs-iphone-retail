import { MedusaService } from "@medusajs/framework/utils"
import { SeoSetting, SeoPage, SeoSource, SeoCrawlerVisit, SeoTrafficVisit, SeoProvider, SeoMonitor, SeoRun, SeoEvidence, SeoSubmission } from "./models/entities"

export default class SeoModuleService extends MedusaService({
  SeoSetting, SeoPage, SeoSource, SeoCrawlerVisit, SeoTrafficVisit, SeoProvider, SeoMonitor, SeoRun, SeoEvidence, SeoSubmission,
}) {}
