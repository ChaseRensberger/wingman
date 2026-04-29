import { useRouterState } from '@tanstack/react-router'
import React from 'react'
import { AppSidebar } from './app-sidebar'

const PAGE_TITLES: Record<string, string> = {
  '/': 'Dashboard',
  '/providers': 'Providers',
  '/sessions': 'Sessions',
  '/settings': 'Settings',
}

export function SidebarShell({ children }: React.PropsWithChildren) {
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const title = PAGE_TITLES[pathname] ?? ''

  return (
    <div>
      {/* Static icon-only sidebar — always visible */}
      <div className="fixed inset-y-0 z-50 flex w-14 flex-col">
        <AppSidebar />
      </div>

      <div className="pl-14">
        {/* Top header */}
        <div className="sticky top-0 z-40 flex h-16 shrink-0 items-center gap-x-4 border-b border-border bg-background px-4 sm:gap-x-6 sm:px-6 lg:px-8 dark:border-border dark:bg-card">
          <div className="flex flex-1 items-center justify-between self-stretch">
            <h1 className="text-base font-semibold text-foreground dark:text-foreground">{title}</h1>
          </div>
        </div>

        <main className="py-10">
          <div className="px-4 sm:px-6 lg:px-8">
            <div className="mx-auto max-w-6xl">{children}</div>
          </div>
        </main>
      </div>
    </div>
  )
}
