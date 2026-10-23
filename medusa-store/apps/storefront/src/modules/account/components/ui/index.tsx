"use client"

import {
  Alert,
  Button as AntButton,
  Card,
  Form,
  Input as AntInput,
  Modal as AntModal,
  Select,
  Typography,
  type ButtonProps,
  type InputProps,
} from "antd"
import {
  Children,
  isValidElement,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactElement,
  type ReactNode,
  type HTMLAttributes,
  type ComponentRef,
} from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import { useFormStatus } from "react-dom"
import type { HttpTypes } from "@medusajs/types"
import { countrySelectTranslations } from "@lib/data/country-select-translations"
import { getCountryOptions, normalizeCountrySearch } from "@lib/util/countries"
import { useAccountCopy } from "@modules/account/use-account-copy"

// Keep native form actions and field names while rendering Ant Design controls.
export function Input({
  label,
  topLabel,
  type,
  ...props
}: InputProps & { label?: string; topLabel?: string }) {
  const id = useId()
  const control =
    type === "password" ? (
      <AntInput.Password {...props} id={props.id ?? id} size="large" />
    ) : (
      <AntInput
        {...props}
        id={props.id ?? id}
        type={type === "phone" ? "tel" : type}
        size="large"
      />
    )
  return label || topLabel ? (
    <Form.Item
      label={topLabel ?? label}
      layout="vertical"
      required={props.required}
      style={{ marginBottom: 8 }}
      htmlFor={props.id ?? id}
    >
      {control}
    </Form.Item>
  ) : (
    control
  )
}

type ActionProps = Omit<ButtonProps, "type" | "size" | "variant"> & {
  type?: "button" | "submit" | "reset"
  variant?: "primary" | "secondary" | "transparent" | "danger"
  size?: "small" | "base" | "large"
  isLoading?: boolean
}

export function Button({
  type = "button",
  variant = "primary",
  size,
  isLoading,
  danger,
  ...props
}: ActionProps) {
  return (
    <AntButton
      {...props}
      htmlType={type}
      type={
        variant === "primary"
          ? "primary"
          : variant === "transparent"
          ? "text"
          : "default"
      }
      danger={variant === "danger" || danger}
      size={size === "base" ? "middle" : size}
      loading={isLoading}
    />
  )
}

export function SubmitButton(props: ActionProps) {
  const { pending } = useFormStatus()
  return (
    <Button
      {...props}
      type="submit"
      disabled={pending || props.disabled}
      isLoading={pending}
    />
  )
}

export function IconButton(props: ActionProps) {
  return <Button variant="transparent" {...props} />
}
export function LinkButton({ href, ...props }: ActionProps & { href: string }) {
  const { countryCode } = useParams<{ countryCode: string }>()
  return (
    <Link href={`/${countryCode}${href}`} passHref legacyBehavior>
      <Button {...props} />
    </Link>
  )
}
export function Heading({
  level = "h2",
  ...props
}: HTMLAttributes<HTMLHeadingElement> & { level?: "h1" | "h2" | "h3" | "h4" }) {
  return (
    <Typography.Title
      {...props}
      style={{
        fontSize: 18,
        fontFamily: '"Outfit", "DM Sans", "Microsoft YaHei", sans-serif',
        ...props.style,
      }}
      level={Number(level.slice(1)) as 1 | 2 | 3 | 4}
    />
  )
}
export function Text(
  props: HTMLAttributes<HTMLSpanElement> & { size?: string }
) {
  const { size: _size, ...rest } = props
  return (
    <Typography.Text
      {...rest}
      style={{
        display: rest.className?.split(" ").includes("flex") ? "flex" : "block",
        ...rest.style,
      }}
    />
  )
}
export function Container({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <Card
      variant="borderless"
      className={className}
      styles={{
        body: {
          padding: 0,
          width: "100%",
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
        },
      }}
    >
      {children}
    </Card>
  )
}
export function ErrorMessage({
  error,
  ...props
}: {
  error?: string | null
  "data-testid"?: string
}) {
  return error ? (
    <Alert {...props} type="error" showIcon title={error} className="my-3" />
  ) : null
}

export function CountrySelect({
  region,
  regions,
  name,
  defaultValue,
  required,
  disabled,
  ...props
}: {
  region?: HttpTypes.StoreRegion
  regions?: HttpTypes.StoreRegion[]
  name: string
  defaultValue?: string
  required?: boolean
  disabled?: boolean
  autoComplete?: string
  "data-testid"?: string
}) {
  const { locale, t } = useAccountCopy()
  const [value, setValue] = useState(defaultValue ?? "")
  const [invalid, setInvalid] = useState(false)
  const nativeRef = useRef<HTMLSelectElement>(null)
  const selectRef = useRef<ComponentRef<typeof Select>>(null)
  const id = useId()
  const options = getCountryOptions(regions ?? (region ? [region] : []), locale)
  useEffect(() => {
    const form = nativeRef.current?.form
    const reset = () => {
      setValue(defaultValue ?? "")
      setInvalid(false)
    }
    form?.addEventListener("reset", reset)
    return () => form?.removeEventListener("reset", reset)
  }, [defaultValue])
  return (
    <Form.Item
      label={t("country")}
      layout="vertical"
      required={required}
      validateStatus={invalid ? "error" : undefined}
      help={
        invalid
          ? countrySelectTranslations[locale.startsWith("zh") ? "zh" : "en"]
              .required
          : undefined
      }
      style={{ marginBottom: 8 }}
      htmlFor={id}
    >
      <select
        ref={nativeRef}
        name={name}
        required={required}
        disabled={disabled}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        tabIndex={-1}
        aria-hidden="true"
        className="sr-only"
        onInvalid={(event) => {
          event.preventDefault()
          setInvalid(true)
          selectRef.current?.focus()
        }}
      >
        <option value="" />
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
      <Select
        ref={selectRef}
        id={id}
        aria-label={t("country")}
        aria-required={required}
        aria-invalid={invalid}
        data-testid={props["data-testid"]}
        size="large"
        showSearch
        value={value || undefined}
        disabled={disabled}
        placeholder={t("country")}
        options={options}
        filterOption={(query, option) =>
          !!option?.search.includes(normalizeCountrySearch(query))
        }
        onChange={(code) => {
          setValue(code)
          setInvalid(false)
        }}
      />
    </Form.Item>
  )
}

export function Modal({
  isOpen,
  close,
  children,
  "data-testid": testId,
}: {
  isOpen: boolean
  close: () => void
  children: ReactNode
  "data-testid"?: string
}) {
  const parts = Children.toArray(children)
  const title = parts.find(
    (child) => isValidElement(child) && child.type === Modal.Title
  ) as ReactElement<{ children: ReactNode }> | undefined
  return (
    <AntModal
      title={title?.props.children}
      open={isOpen}
      onCancel={close}
      footer={null}
      destroyOnHidden
      centered
      modalRender={(node) => <div data-testid={testId}>{node}</div>}
    >
      {parts.filter((child) => child !== title)}
    </AntModal>
  )
}
Modal.Title = function Title({
  children,
}: {
  children: ReactNode
  closeLabel?: string
}) {
  return <div className="pr-8 mb-4">{children}</div>
}
Modal.Body = function Body({ children }: { children: ReactNode }) {
  return <div>{children}</div>
}
Modal.Footer = function Footer({ children }: { children: ReactNode }) {
  return <div className="flex justify-end">{children}</div>
}
