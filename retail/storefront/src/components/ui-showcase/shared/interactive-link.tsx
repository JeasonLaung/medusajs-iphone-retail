// Source: medusa-store/apps/storefront/src/modules/common/components/interactive-link/index.tsx
import { ArrowUpRightMini } from "../../products/icons"
import { Text } from "./ui"
import Link from "../../products/localized-link"
import {useAccountContext} from "../../account/context"
const LocalizedClientLink = (props: Omit<React.ComponentProps<typeof Link>, "segment">) => <Link {...props} segment={useAccountContext().segment}/>
type InteractiveLinkProps = {
  href: string
  children?: React.ReactNode
  onClick?: () => void
}

const InteractiveLink = ({
  href,
  children,
  onClick,
  ...props
}: InteractiveLinkProps) => {
  return (
    <LocalizedClientLink
      className="flex gap-x-1 items-center group"
      href={href}
      onClick={onClick}
      {...props}
    >
      <Text className="text-ui-fg-interactive">{children}</Text>
      <ArrowUpRightMini
        className="group-hover:rotate-45 ease-in-out duration-150"
        color="var(--fg-interactive)"
      />
    </LocalizedClientLink>
  )
}

export default InteractiveLink
