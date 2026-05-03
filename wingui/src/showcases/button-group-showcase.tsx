import { ButtonGroup } from "@/components/core/button-group"
import { Button } from "@/components/core/button"
import { TextAlignLeftIcon, TextAlignCenterIcon, TextAlignRightIcon } from "@phosphor-icons/react"

export function ButtonGroupShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Button Group</h2>
      <div className="space-y-4">
        <ButtonGroup>
          <Button variant="outline">First</Button>
          <Button variant="outline">Second</Button>
          <Button variant="outline">Third</Button>
        </ButtonGroup>
        <ButtonGroup>
          <Button variant="outline" size="icon">
            <TextAlignLeftIcon />
          </Button>
          <Button variant="outline" size="icon">
            <TextAlignCenterIcon />
          </Button>
          <Button variant="outline" size="icon">
            <TextAlignRightIcon />
          </Button>
        </ButtonGroup>
      </div>
    </section>
  )
}
