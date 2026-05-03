import { Empty, EmptyIcon, EmptyTitle, EmptyDescription, EmptyActions } from "@/components/core/empty"
import { Button } from "@/components/core/button"
import { FolderOpenIcon, MagnifyingGlassIcon } from "@phosphor-icons/react"

export function EmptyShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Empty</h2>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="rounded-lg border border-dashed border-border">
          <Empty>
            <EmptyIcon>
              <FolderOpenIcon />
            </EmptyIcon>
            <EmptyTitle>No files found</EmptyTitle>
            <EmptyDescription>
              You haven't uploaded any files yet. Get started by uploading your first file.
            </EmptyDescription>
            <EmptyActions>
              <Button size="sm">Upload file</Button>
            </EmptyActions>
          </Empty>
        </div>
        <div className="rounded-lg border border-dashed border-border">
          <Empty>
            <EmptyIcon>
              <MagnifyingGlassIcon />
            </EmptyIcon>
            <EmptyTitle>No results found</EmptyTitle>
            <EmptyDescription>
              We couldn't find anything matching your search. Try different terms.
            </EmptyDescription>
            <EmptyActions>
              <Button variant="outline" size="sm">Clear search</Button>
            </EmptyActions>
          </Empty>
        </div>
      </div>
    </section>
  )
}
