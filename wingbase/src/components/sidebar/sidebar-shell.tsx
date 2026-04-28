import { Dialog, DialogBackdrop, DialogPanel, TransitionChild } from '@headlessui/react'
import { List, X } from '@phosphor-icons/react'
import { useRouterState } from '@tanstack/react-router'
import React from 'react'
import { useSidebar } from './sidebar-provider'

const PAGE_TITLES: Record<string, string> = {
  '/': 'Dashboard',
  '/providers': 'Providers',
  '/sessions': 'Sessions',
  '/settings': 'Settings',
}

export function SidebarShell({
  sidebar,
  children,
}: React.PropsWithChildren<{ sidebar: React.ReactNode }>) {
  const { openMobile, setOpenMobile } = useSidebar()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const title = PAGE_TITLES[pathname] ?? ''

  return (
    <div>
      {/* Mobile sidebar */}
      <Dialog open={openMobile} onClose={setOpenMobile} className="relative z-50 lg:hidden">
        <DialogBackdrop
          transition
          className="fixed inset-0 bg-zinc-900/80 transition-opacity duration-300 ease-linear data-closed:opacity-0"
        />

        <div className="fixed inset-0 flex">
          <DialogPanel
            transition
            className="relative mr-16 flex w-14 flex-1 transform transition duration-300 ease-in-out data-closed:-translate-x-full"
          >
            <TransitionChild>
              <div className="absolute top-0 left-full flex w-16 justify-center pt-5 duration-300 ease-in-out data-closed:opacity-0">
                <button type="button" onClick={() => setOpenMobile(false)} className="-m-2.5 p-2.5">
                  <span className="sr-only">Close sidebar</span>
                  <X aria-hidden="true" className="size-6 text-white" />
                </button>
              </div>
            </TransitionChild>

            {sidebar}
          </DialogPanel>
        </div>
      </Dialog>

      {/* Static sidebar for desktop — icon-only w-14 rail */}
      <div className="hidden lg:fixed lg:inset-y-0 lg:z-50 lg:flex lg:w-14 lg:flex-col">
        {sidebar}
      </div>

      <div className="lg:pl-14">
        {/* Top header — same bg as sidebar so separator lines up cleanly */}
        <div className="sticky top-0 z-40 flex h-16 shrink-0 items-center gap-x-4 border-b border-zinc-200 bg-white px-4 sm:gap-x-6 sm:px-6 lg:px-8 dark:border-white/10 dark:bg-zinc-900">
          <button
            type="button"
            onClick={() => setOpenMobile(true)}
            className="-m-2.5 p-2.5 text-zinc-700 hover:text-zinc-900 lg:hidden dark:text-zinc-400 dark:hover:text-white"
          >
            <span className="sr-only">Open sidebar</span>
            <List aria-hidden="true" className="size-6" />
          </button>

          {/* Separator */}
          <div aria-hidden="true" className="h-6 w-px bg-zinc-200 lg:hidden dark:bg-white/10" />

          <div className="flex flex-1 items-center justify-between self-stretch lg:gap-x-6">
            <h1 className="text-base font-semibold text-zinc-950 dark:text-white">{title}</h1>
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
