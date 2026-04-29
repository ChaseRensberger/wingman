import clsx from 'clsx'

export function Text({ className, ...props }: React.ComponentPropsWithoutRef<'p'>) {
  return (
    <p
      data-slot="text"
      {...props}
      className={clsx(className, 'text-base/6 text-muted-foreground sm:text-sm/6')}
    />
  )
}
