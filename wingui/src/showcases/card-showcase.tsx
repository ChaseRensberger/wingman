import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/core/card'
import { Button } from '@/components/core/button'

export function CardShowcase() {
  return (
    <section className="py-4 space-y-8">
      <h2 className="text-2xl font-semibold">Card</h2>
      <div className="flex flex-wrap gap-4">
        <Card className="w-80">
          <CardHeader>
            <CardTitle>Team Subscription</CardTitle>
            <CardDescription>Manage your team plan and billing.</CardDescription>
            <CardAction>
              <Button variant="outline" size="sm">Manage</Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Your team is on the <strong className="text-foreground">Pro plan</strong>. Next billing date is June 1, 2026.
            </p>
          </CardContent>
          <CardFooter className="gap-2">
            <Button variant="default" size="sm">Upgrade</Button>
            <Button variant="ghost" size="sm">Cancel</Button>
          </CardFooter>
        </Card>

        <Card className="w-80" size="sm">
          <CardHeader>
            <CardTitle>Quick Stats</CardTitle>
            <CardDescription>Last 30 days</CardDescription>
          </CardHeader>
          <CardContent>
            <p className="text-3xl font-bold">1,284</p>
            <p className="text-sm text-muted-foreground">Total requests</p>
          </CardContent>
        </Card>
      </div>
    </section>
  )
}
