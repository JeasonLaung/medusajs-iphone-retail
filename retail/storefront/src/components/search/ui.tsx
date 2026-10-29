import clsx from "clsx"
import {forwardRef,type HTMLAttributes} from "react"
export {clsx as clx}
// Text Component
type TextProps = HTMLAttributes<HTMLParagraphElement> & {
  as?: "p" | "span" | "div"
}

export const Text = forwardRef<HTMLParagraphElement, TextProps>(
  ({ className, as: Component = "p", children, ...props }, ref) => {
    return (
      <Component ref={ref} className={clsx("txt-medium", className)} {...props}>
        {children}
      </Component>
    )
  }
)
Text.displayName = "Text"
