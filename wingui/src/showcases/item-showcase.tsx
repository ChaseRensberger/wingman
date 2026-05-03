import { Item, ItemIcon, ItemContent, ItemLabel, ItemDescription, ItemAction } from "@/components/core/item"
import { Badge } from "@/components/core/badge"
import { FileIcon, FolderIcon, ImageIcon } from "@phosphor-icons/react"

export function ItemShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Item</h2>
      <div className="space-y-0.5 max-w-sm rounded-lg border border-border">
        <Item>
          <ItemIcon><FolderIcon /></ItemIcon>
          <ItemContent>
            <ItemLabel>Documents</ItemLabel>
            <ItemDescription>12 files</ItemDescription>
          </ItemContent>
        </Item>
        <Item>
          <ItemIcon><ImageIcon /></ItemIcon>
          <ItemContent>
            <ItemLabel>Photos</ItemLabel>
            <ItemDescription>238 files</ItemDescription>
          </ItemContent>
          <ItemAction>
            <Badge variant="secondary">New</Badge>
          </ItemAction>
        </Item>
        <Item>
          <ItemIcon><FileIcon /></ItemIcon>
          <ItemContent>
            <ItemLabel>README.md</ItemLabel>
            <ItemDescription>Modified 2 days ago</ItemDescription>
          </ItemContent>
        </Item>
      </div>
    </section>
  )
}
