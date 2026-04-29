import * as Headless from '@headlessui/react'
import clsx from 'clsx'
import React, { forwardRef } from 'react'
import { Link } from './link'

const styles = {
  base: [
    // Base
    'relative isolate inline-flex items-baseline justify-center gap-x-2 rounded-lg border text-base/6 font-semibold font-mono',
    // Sizing
    'px-[calc(--spacing(3.5)-1px)] py-[calc(--spacing(2.5)-1px)] sm:px-[calc(--spacing(3)-1px)] sm:py-[calc(--spacing(1.5)-1px)] sm:text-sm/6',
    // Focus
    'focus:not-data-focus:outline-hidden data-focus:outline-2 data-focus:outline-offset-2 data-focus:outline-primary',
    // Disabled
    'data-disabled:opacity-50',
    // Icon
    '*:data-[slot=icon]:-mx-0.5 *:data-[slot=icon]:my-0.5 *:data-[slot=icon]:size-5 *:data-[slot=icon]:shrink-0 *:data-[slot=icon]:self-center *:data-[slot=icon]:text-(--btn-icon) sm:*:data-[slot=icon]:my-1 sm:*:data-[slot=icon]:size-4 forced-colors:[--btn-icon:ButtonText] forced-colors:data-hover:[--btn-icon:ButtonText]',
  ],
  solid: [
    // Optical border, implemented as the button background to avoid corner artifacts
    'border-transparent bg-(--btn-border)',
    // Dark mode: border is rendered on `after` so background is set to button background
    'dark:bg-(--btn-bg)',
    // Button background, implemented as foreground layer to stack on top of pseudo-border layer
    'before:absolute before:inset-0 before:-z-10 before:rounded-[calc(var(--radius-lg)-1px)] before:bg-(--btn-bg)',
    // Drop shadow, applied to the inset `before` layer so it blends with the border in light mode
    'before:shadow-sm',
    // Background color is moved to control and shadow is removed in dark mode so hide `before` pseudo
    'dark:before:hidden',
    // Dark mode: Subtle white outline is applied using a border
    'dark:border-white/5',
    // Shim/overlay, inset to match button foreground and used for hover state + highlight shadow
    'after:absolute after:inset-0 after:-z-10 after:rounded-[calc(var(--radius-lg)-1px)]',
    // Inner highlight shadow
    'after:shadow-[inset_0_1px_--theme(--color-white/15%)]',
    // White overlay on hover
    'data-active:after:bg-(--btn-hover-overlay) data-hover:after:bg-(--btn-hover-overlay)',
    // Dark mode: `after` layer expands to cover entire button
    'dark:after:-inset-px dark:after:rounded-lg',
    // Disabled
    'data-disabled:before:shadow-none data-disabled:after:shadow-none',
  ],
  outline: [
    // Base
    'border-border text-foreground data-active:bg-accent/50 data-hover:bg-accent/50',
    // Dark mode
    'dark:border-border dark:text-foreground dark:[--btn-bg:transparent] dark:data-active:bg-accent/50 dark:data-hover:bg-accent/50',
    // Icon
    '[--btn-icon:var(--color-muted-foreground)] data-active:[--btn-icon:var(--color-foreground)] data-hover:[--btn-icon:var(--color-foreground)]',
  ],
  plain: [
    // Base
    'border-transparent text-foreground data-active:bg-accent data-hover:bg-accent',
    // Dark mode
    'dark:text-foreground dark:data-active:bg-accent dark:data-hover:bg-accent',
    // Icon
    '[--btn-icon:var(--color-muted-foreground)] data-active:[--btn-icon:var(--color-foreground)] data-hover:[--btn-icon:var(--color-foreground)]',
  ],
  colors: {
    primary: [
      'text-primary-foreground [--btn-bg:var(--color-primary)] [--btn-border:var(--color-primary)]/90 [--btn-hover-overlay:var(--color-primary-foreground)]/10',
      'dark:text-primary-foreground dark:[--btn-bg:var(--color-primary)] dark:[--btn-hover-overlay:var(--color-primary-foreground)]/5',
      '[--btn-icon:var(--color-primary-foreground)]/60 data-active:[--btn-icon:var(--color-primary-foreground)]/80 data-hover:[--btn-icon:var(--color-primary-foreground)]/80',
    ],
    secondary: [
      'text-secondary-foreground [--btn-bg:var(--color-secondary)] [--btn-border:var(--color-secondary)]/90 [--btn-hover-overlay:var(--color-secondary-foreground)]/10',
      'dark:text-secondary-foreground dark:[--btn-bg:var(--color-secondary)] dark:[--btn-hover-overlay:var(--color-secondary-foreground)]/5',
      '[--btn-icon:var(--color-secondary-foreground)]/60 data-active:[--btn-icon:var(--color-secondary-foreground)]/80 data-hover:[--btn-icon:var(--color-secondary-foreground)]/80',
    ],
    destructive: [
      'text-white [--btn-bg:var(--color-destructive)] [--btn-border:var(--color-destructive)]/90 [--btn-hover-overlay:var(--color-white)]/10',
      'dark:[--btn-bg:var(--color-destructive)] dark:[--btn-hover-overlay:var(--color-white)]/5',
      '[--btn-icon:var(--color-white)]/60 data-active:[--btn-icon:var(--color-white)]/80 data-hover:[--btn-icon:var(--color-white)]/80',
    ],
    zinc: [
      'text-foreground [--btn-bg:var(--color-background)] [--btn-border:var(--color-border)]/90 [--btn-hover-overlay:var(--color-foreground)]/5',
      'dark:text-foreground dark:[--btn-bg:var(--color-card)] dark:[--btn-hover-overlay:var(--color-foreground)]/5',
      '[--btn-icon:var(--color-muted-foreground)] data-active:[--btn-icon:var(--color-foreground)] data-hover:[--btn-icon:var(--color-foreground)]',
    ],
  },
}

type ButtonProps = (
  | { color?: keyof typeof styles.colors; outline?: never; plain?: never }
  | { color?: never; outline: true; plain?: never }
  | { color?: never; outline?: never; plain: true }
) & { className?: string; children: React.ReactNode } & (
    | ({ href?: never } & Omit<Headless.ButtonProps, 'as' | 'className'>)
    | ({ href: string } & Omit<React.ComponentPropsWithoutRef<typeof Link>, 'className'>)
  )

export const Button = forwardRef(function Button(
  { color, outline, plain, className, children, ...props }: ButtonProps,
  ref: React.ForwardedRef<HTMLElement>
) {
  const classes = clsx(
    className,
    styles.base,
    outline ? styles.outline : plain ? styles.plain : clsx(styles.solid, styles.colors[color ?? 'primary'])
  )

  return typeof props.href === 'string' ? (
    <Link {...props} className={classes} ref={ref as React.ForwardedRef<HTMLAnchorElement>}>
      <TouchTarget>{children}</TouchTarget>
    </Link>
  ) : (
    <Headless.Button {...props} className={clsx(classes, 'cursor-default')} ref={ref}>
      <TouchTarget>{children}</TouchTarget>
    </Headless.Button>
  )
})

/**
 * Expand the hit area to at least 44×44px on touch devices
 */
export function TouchTarget({ children }: { children: React.ReactNode }) {
  return (
    <>
      <span
        className="absolute top-1/2 left-1/2 size-[max(100%,2.75rem)] -translate-x-1/2 -translate-y-1/2 pointer-fine:hidden"
        aria-hidden="true"
      />
      {children}
    </>
  )
}
