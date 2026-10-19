"use client"

import { CheckCircle } from "@medusajs/icons"
import { resetOnboardingState } from "@lib/data/onboarding"
import { Button, Container, Text } from "@modules/common/components/ui"
import { useAccountCopy } from "@modules/account/use-account-copy"

const OnboardingCta = ({ orderId }: { orderId: string }) => {
  const { t } = useAccountCopy()
  return (
    <Container className="max-w-4xl h-full bg-ui-bg-subtle w-full">
      <div className="flex flex-col gap-y-4 center p-4 md:items-center">
        <Text className="text-ui-fg-base text-xl">
          {t("testOrderCreated")} <CheckCircle aria-hidden="true" width={24} height={24} className="inline-block align-middle shrink-0" />
        </Text>
        <Text className="text-ui-fg-subtle text-small-regular">
          {t("completeStoreSetupDescription")}
        </Text>
        <Button
          className="w-fit"
          size="large"
          onClick={() => resetOnboardingState(orderId)}
        >
          {t("completeStoreSetup")}
        </Button>
      </div>
    </Container>
  )
}

export default OnboardingCta
