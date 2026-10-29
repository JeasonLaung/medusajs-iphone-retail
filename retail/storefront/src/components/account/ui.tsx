import {Alert,Button as AntButton,Form,Input as AntInput,type InputProps,type ButtonProps} from "antd"
import {useId} from "react"
import {useFormStatus} from "react-dom"
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

export function ErrorMessage({
  error,
  ...props
}: {
  error?: string | null
  "data-testid"?: string
}) {
  return error ? (
    <Alert {...props} type="error" showIcon message={error} className="my-3" />
  ) : null
}

