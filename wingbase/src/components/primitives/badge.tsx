import * as Headless from '@headlessui/react'
import clsx from 'clsx'
import React, { forwardRef } from 'react'
import { TouchTarget } from './button'
import { Link } from './link'

const colors = {
  red: 'bg-destructive/10 text-destructive group-data-hover:bg-destructive/20',
  orange: 'bg-orange-500/10 text-orange-500 group-data-hover:bg-orange-500/20',
  amber: 'bg-amber-500/10 text-amber-500 group-data-hover:bg-amber-500/20',
  yellow: 'bg-yellow-500/10 text-yellow-500 group-data-hover:bg-yellow-500/20',
  lime: 'bg-lime-500/10 text-lime-500 group-data-hover:bg-lime-500/20',
  green: 'bg-green-500/10 text-green-500 group-data-hover:bg-green-500/20',
  emerald: 'bg-emerald-500/10 text-emerald-500 group-data-hover:bg-emerald-500/20',
  teal: 'bg-teal-500/10 text-teal-500 group-data-hover:bg-teal-500/20',
  cyan: 'bg-cyan-500/10 text-cyan-500 group-data-hover:bg-cyan-500/20',
  sky: 'bg-sky-500/10 text-sky-500 group-data-hover:bg-sky-500/20',
  blue: 'bg-primary/10 text-primary group-data-hover:bg-primary/20',
  indigo: 'bg-indigo-500/10 text-indigo-500 group-data-hover:bg-indigo-500/20',
  violet: 'bg-violet-500/10 text-violet-500 group-data-hover:bg-violet-500/20',
  purple: 'bg-purple-500/10 text-purple-500 group-data-hover:bg-purple-500/20',
  fuchsia: 'bg-fuchsia-500/10 text-fuchsia-500 group-data-hover:bg-fuchsia-500/20',
  pink: 'bg-pink-500/10 text-pink-500 group-data-hover:bg-pink-500/20',
  rose: 'bg-rose-500/10 text-rose-500 group-data-hover:bg-rose-500/20',
  zinc: 'bg-muted text-foreground group-data-hover:bg-accent',
}

type BadgeProps = { color?: keyof typeof colors }

export function Badge({ color = 'zinc', className, ...props }: BadgeProps & React.ComponentPropsWithoutRef<'span'>) {
  return (
    <span
      {...props}
      className={clsx(
        className,
        'inline-flex items-center gap-x-1.5 rounded-md px-1.5 py-0.5 text-sm/5 font-medium sm:text-xs/5 forced-colors:outline',
        colors[color]
      )}
    />
  )
}

export const BadgeButton = forwardRef(function BadgeButton(
  {
    color = 'zinc',
    className,
    children,
    ...props
  }: BadgeProps & { className?: string; children: React.ReactNode } & (
      | ({ href?: never } & Omit<Headless.ButtonProps, 'as' | 'className'>)
      | ({ href: string } & Omit<React.ComponentPropsWithoutRef<typeof Link>, 'className'>)
    ),
  ref: React.ForwardedRef<HTMLElement>
) {
  const classes = clsx(
    className,
    'group relative inline-flex rounded-md focus:not-data-focus:outline-hidden data-focus:outline-2 data-focus:outline-offset-2 data-focus:outline-primary'
  )

  return typeof props.href === 'string' ? (
    <Link {...props} className={classes} ref={ref as React.ForwardedRef<HTMLAnchorElement>}>
      <TouchTarget>
        <Badge color={color}>{children}</Badge>
      </TouchTarget>
    </Link>
  ) : (
    <Headless.Button {...props} className={classes} ref={ref}>
      <TouchTarget>
        <Badge color={color}>{children}</Badge>
      </TouchTarget>
    </Headless.Button>
  )
})
