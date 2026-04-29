import clsx from 'clsx'
import { Link } from './link'

export function Text({ className, ...props }: React.ComponentPropsWithoutRef<'p'>) {
  return (
    <p
      data-slot="text"
      {...props}
      className={clsx(className, 'text-base/6 text-muted-foreground sm:text-sm/6 dark:text-muted-foreground')}
    />
  )
}

export function TextLink({ className, ...props }: React.ComponentPropsWithoutRef<typeof Link>) {
  return (
    <Link
      {...props}
      className={clsx(
        className,
        'text-foreground underline decoration-foreground/50 data-hover:decoration-foreground dark:text-foreground dark:decoration-foreground/50 dark:data-hover:decoration-foreground'
      )}
    />
  )
}

export function Strong({ className, ...props }: React.ComponentPropsWithoutRef<'strong'>) {
  return <strong {...props} className={clsx(className, 'font-medium text-foreground dark:text-foreground')} />
}

export function Code({ className, ...props }: React.ComponentPropsWithoutRef<'code'>) {
  return (
    <code
      {...props}
      className={clsx(
        className,
        'rounded-sm border border-border bg-foreground/2.5 px-0.5 text-sm font-medium text-foreground sm:text-[0.8125rem] dark:border-border dark:bg-background/5 dark:text-foreground'
      )}
    />
  )
}
