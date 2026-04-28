import * as Headless from '@headlessui/react'
import { Link as RouterLink } from '@tanstack/react-router'
import React, { forwardRef } from 'react'

export const Link = forwardRef(function Link(
  { href, ...props }: { href: string } & React.ComponentPropsWithoutRef<'a'>,
  ref: React.ForwardedRef<HTMLAnchorElement>
) {
  return (
    <Headless.DataInteractive>
      <RouterLink to={href} {...props} ref={ref} />
    </Headless.DataInteractive>
  )
})
