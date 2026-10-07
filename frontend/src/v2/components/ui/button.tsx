import type { LucideIcon } from 'lucide-react'
import type { ReactNode } from 'react'

import { Button as KitButton } from '@/kit/ui/Button'

/** The kit button, with the yt call sites' `primary` / `danger` flags. */
export function Button({
  children,
  onClick,
  primary,
  danger,
  icon,
  disabled,
  className,
  type = 'button',
  form,
}: {
  children: ReactNode
  onClick?: () => void
  primary?: boolean
  danger?: boolean
  icon?: LucideIcon
  disabled?: boolean
  className?: string
  type?: 'button' | 'submit'
  form?: string
}) {
  return (
    <KitButton
      type={type}
      form={form}
      onClick={onClick}
      disabled={disabled}
      icon={icon}
      className={className}
      variant={danger ? 'danger' : primary ? 'primary' : 'secondary'}
    >
      {children}
    </KitButton>
  )
}
